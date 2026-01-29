/*
Copyright (c) Gatis Beikerts

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package renewal

import (
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/errors"
	"github.com/unknovs/status-list-go/models"
	"github.com/unknovs/status-list-go/services"
	"github.com/unknovs/status-list-go/services/storage"
)

// RenewalService handles the renewal of status lists
type RenewalService struct {
	config  *config.Config
	storage storage.Storage
}

// NewRenewalService creates a new renewal service
func NewRenewalService(cfg *config.Config, stor storage.Storage) *RenewalService {
	return &RenewalService{
		config:  cfg,
		storage: stor,
	}
}

// RenewLists renews all status lists that haven't expired
func (rs *RenewalService) RenewLists() error {
	log.Println("Starting list renewal process")

	// Check if the status list directory exists
	if _, err := os.Stat(rs.config.StatusListDir); os.IsNotExist(err) {
		log.Printf("Error listing files: status list directory does not exist: %s", rs.config.StatusListDir)
		return fmt.Errorf("status list directory does not exist: %s: %w", rs.config.StatusListDir, err)
	}

	// Create formatter for JWT/CWT generation
	formatter := services.NewStatusListFormatter(rs.config)

	// Use Storage.List to find all full_list.json files
	allFiles, err := rs.storage.List("")
	if err != nil {
		log.Printf("Error listing files: %v", err)
		return nil // Handle gracefully, don't fail the entire renewal
	}

	// Process only full_list.json files
	for _, filePath := range allFiles {
		if filepath.Base(filePath) == "full_list.json" {
			if err := rs.processListFile(filePath, formatter); err != nil {
				log.Printf("Error processing file %s: %v", filePath, err)
				// Continue with other files
			}
		}
	}

	log.Println("List renewal process completed")
	return nil
}

// processListFile processes a single list file
func (rs *RenewalService) processListFile(filePath string, formatter *services.StatusListFormatter) error {
	// Convert absolute path to relative path if needed
	var relativePath string
	if filepath.IsAbs(filePath) {
		if rel, err := filepath.Rel(rs.config.StatusListDir, filePath); err == nil {
			relativePath = rel
		} else {
			// If we can't make it relative, use the original path and let storage handle it
			relativePath = filePath
		}
	} else {
		relativePath = filePath
	}

	dirPath := filepath.Dir(relativePath)

	// Read and parse the list file using Storage interface
	jsonData, err := rs.storage.Read(relativePath)
	if err != nil {
		// File might have been deleted by cleanup service, skip gracefully
		if stdErrors.Is(err, errors.ErrNotFound) {
			log.Printf("File %s no longer exists (may have been cleaned up), skipping", filePath)
			return nil
		}
		log.Printf("Error reading file %s: %v", filePath, err)
		return nil // Continue with other files
	}

	var statusListData models.StatusListData
	if err := json.Unmarshal(jsonData, &statusListData); err != nil {
		log.Printf("Error unmarshaling file %s: %v", filePath, err)
		return nil // Continue with other files
	}

	// Check if required URIs exist
	if statusListData.StatusListURI == "" || statusListData.IdentifierListURI == "" {
		log.Printf("URIs don't exist in file: %s", filePath)
		return nil // Continue with other files
	}

	// Check if expired
	if statusListData.Expires != nil {
		expiresDate, err := time.Parse("2006-01-02", *statusListData.Expires)
		if err != nil {
			log.Printf("Error parsing expiry date in file %s: %v", filePath, err)
			return nil // Continue with other files
		}

		if expiresDate.Before(time.Now()) {
			log.Printf("List %s is expired, skipping renewal", dirPath)
			// Note: Expired lists are cleaned up by the separate cleanup service
			// which runs on a configurable schedule (default: daily at 2:00 AM)
			return nil
		}
	}

	// Determine list type and regenerate files
	if strings.Contains(dirPath, "token_status_list") {
		return rs.renewTokenStatusList(dirPath, &statusListData, formatter)
	} else if strings.Contains(dirPath, "identifier_list") {
		return rs.renewIdentifierList(dirPath, &statusListData, formatter)
	}

	return nil
}

// renewTokenStatusList renews token status list files
func (rs *RenewalService) renewTokenStatusList(dirPath string, statusListData *models.StatusListData, formatter *services.StatusListFormatter) error {
	// Note: Backup functionality removed as it requires filesystem-specific operations
	// In production, backups should be handled at the infrastructure level (S3 versioning, etc.)

	// Regenerate JWT
	rs.generateAndWrite(dirPath, "token_status_list.jwt", func() (string, error) {
		return formatter.GenerateJWT(statusListData.TokenStatusList, statusListData.Country, statusListData.StatusListURI)
	}, "JWT")

	// Regenerate CWT
	rs.generateAndWrite(dirPath, "token_status_list.cwt", func() (string, error) {
		return formatter.GenerateCWT(statusListData.TokenStatusList, statusListData.Country, statusListData.StatusListURI)
	}, "CWT")

	return nil
}

// renewIdentifierList renews identifier list files
func (rs *RenewalService) renewIdentifierList(dirPath string, statusListData *models.StatusListData, formatter *services.StatusListFormatter) error {
	// Note: Backup functionality removed as it requires filesystem-specific operations
	// In production, backups should be handled at the infrastructure level (S3 versioning, etc.)

	// Regenerate JWT
	rs.generateAndWrite(dirPath, "identifier_list.jwt", func() (string, error) {
		return formatter.GenerateIdentifierJWT(statusListData.IdentifierList, statusListData.Country, statusListData.IdentifierListURI)
	}, "identifier JWT")

	// Regenerate CWT
	rs.generateAndWrite(dirPath, "identifier_list.cwt", func() (string, error) {
		return formatter.GenerateIdentifierCWT(statusListData.IdentifierList, statusListData.Country, statusListData.IdentifierListURI)
	}, "identifier CWT")

	return nil
}

// generateAndWrite handles the pattern of generating content and writing it to a file
// with appropriate error handling and logging
func (rs *RenewalService) generateAndWrite(
	dirPath string,
	filename string,
	generateFunc func() (string, error),
	description string,
) {
	content, err := generateFunc()
	if err != nil {
		log.Printf("Failed to generate %s for %s: %v", description, dirPath, err)
		return
	}

	filePath := filepath.Join(dirPath, filename)
	if err := rs.writeOrCreateFile(filePath, []byte(content)); err != nil {
		if stdErrors.Is(err, errors.ErrNotFound) {
			log.Printf("Directory %s no longer exists (cleaned up), skipping %s write", dirPath, description)
		} else {
			log.Printf("Failed to write %s file %s: %v", description, filePath, err)
		}
	}
}

// writeOrCreateFile is a helper that creates a file if it doesn't exist, or writes to it if it does
// It implements retry logic for version conflicts (optimistic locking failures)
func (rs *RenewalService) writeOrCreateFile(path string, content []byte) error {
	exists, err := rs.storage.Exists(path)
	if err != nil {
		return err
	}

	if !exists {
		return rs.storage.Create(path, content)
	}

	// File exists, attempt write with retry logic
	return rs.writeWithRetry(path, content, 3)
}

// writeWithRetry attempts to write with version control, retrying on conflicts
func (rs *RenewalService) writeWithRetry(path string, content []byte, maxRetries int) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		result := rs.performWriteAttempt(path, content, attempt, maxRetries)
		if result.shouldReturn {
			return result.err
		}
	}

	return fmt.Errorf("failed to write after %d retries", maxRetries)
}

// writeAttemptResult contains the result of a single write attempt
type writeAttemptResult struct {
	shouldReturn bool  // true if the caller should return immediately
	err          error // error to return (nil on success)
}

// performWriteAttempt performs a single write attempt and returns whether to continue
func (rs *RenewalService) performWriteAttempt(path string, content []byte, attempt, maxRetries int) writeAttemptResult {
	exists, err := rs.verifyFileExists(path)
	if err != nil {
		return writeAttemptResult{shouldReturn: true, err: err}
	}
	if !exists {
		return writeAttemptResult{shouldReturn: true, err: nil}
	}

	currentVersion, err := rs.getCurrentVersion(path)
	if err != nil {
		if stdErrors.Is(err, errors.ErrNotFound) {
			return writeAttemptResult{shouldReturn: true, err: nil}
		}
		return writeAttemptResult{shouldReturn: true, err: err}
	}

	err = rs.attemptWrite(path, content, currentVersion)
	if err == nil {
		if attempt > 1 {
			log.Printf("Successfully wrote %s after %d attempts", path, attempt)
		}
		return writeAttemptResult{shouldReturn: true, err: nil}
	}

	shouldRetry, retryErr := rs.handleWriteError(err, path, attempt, maxRetries)
	if !shouldRetry {
		return writeAttemptResult{shouldReturn: true, err: retryErr}
	}

	return writeAttemptResult{shouldReturn: false, err: nil}
}

// verifyFileExists checks if file still exists before write attempt
func (rs *RenewalService) verifyFileExists(path string) (bool, error) {
	exists, err := rs.storage.Exists(path)
	if err != nil {
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	if !exists {
		log.Printf("File %s no longer exists (may have been cleaned up), skipping write", path)
		return false, nil
	}

	return true, nil
}

// getCurrentVersion retrieves the current file version
// Returns (0, nil) if file was deleted - caller should handle gracefully
func (rs *RenewalService) getCurrentVersion(path string) (int, error) {
	currentVersion, err := rs.storage.GetVersion(path)
	if err != nil {
		if stdErrors.Is(err, errors.ErrNotFound) {
			log.Printf("File %s was deleted during operation, skipping write", path)
			return 0, errors.ErrNotFound
		}
		return 0, fmt.Errorf("failed to get current version: %w", err)
	}
	return currentVersion, nil
}

// attemptWrite performs a single write attempt
func (rs *RenewalService) attemptWrite(path string, content []byte, currentVersion int) error {
	return rs.storage.Write(path, content, currentVersion+1)
}

// handleWriteError determines if write should retry or fail
// Returns (shouldRetry, error) - if shouldRetry is false, the error should be returned to caller
func (rs *RenewalService) handleWriteError(err error, path string, attempt, maxRetries int) (bool, error) {
	if !stdErrors.Is(err, errors.ErrVersionMismatch) {
		return false, err
	}

	if attempt >= maxRetries {
		return false, fmt.Errorf("failed after %d attempts: %w", maxRetries, err)
	}

	log.Printf("Version conflict on %s (attempt %d/%d), retrying...", path, attempt, maxRetries)
	time.Sleep(time.Millisecond * 100 * time.Duration(attempt)) // Exponential backoff
	return true, nil                                            // Signal to continue retrying
}

// copyFile is no longer used with storage abstraction
// Backups should be handled at the infrastructure level

// dailyRenewal runs the renewal process daily at the configured time
func (rs *RenewalService) dailyRenewal() {
	for {
		now := time.Now()
		next := nextRun(now, rs.config.RenewalHour, rs.config.RenewalMinute)
		delay := time.Until(next)

		log.Printf("Next status list renewal scheduled in %02dh:%02dm:%02ds", int(delay.Hours()), int(delay.Minutes())%60, int(delay.Seconds())%60)

		time.Sleep(delay)

		log.Println("Renewing Revocation Lists")

		if err := rs.RenewLists(); err != nil {
			log.Printf("Error during renewal: %v", err)
		}
	}
}

// nextRun calculates the next execution time based on configured hour and minute
func nextRun(now time.Time, hour, minute int) time.Time {
	// Create time for today at the configured hour:minute
	today := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())

	// If the time has already passed today, schedule for tomorrow
	if now.After(today) || now.Equal(today) {
		return today.Add(24 * time.Hour)
	}

	return today
}

// StartRenewalThread starts the renewal thread as a global function
func StartRenewalThread(cfg *config.Config, stor storage.Storage) {
	if !cfg.RenewalEnabled {
		log.Println("Status list renewal disabled via configuration")
		return
	}

	go func() {
		renewalService := NewRenewalService(cfg, stor)
		renewalService.dailyRenewal()
	}()
}

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
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

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
	logger  *zap.Logger
}

// NewRenewalService creates a new renewal service
func NewRenewalService(cfg *config.Config, stor storage.Storage, logger *zap.Logger) *RenewalService {
	return &RenewalService{
		config:  cfg,
		storage: stor,
		logger:  logger,
	}
}

// RenewLists renews all status lists that haven't expired
func (rs *RenewalService) RenewLists() error {
	hostname, _ := os.Hostname()
	rs.logger.Info("starting list renewal process", zap.String("pod", hostname))

	formatter := services.NewStatusListFormatter(rs.config)

	allFiles, err := rs.storage.List("")
	if err != nil {
		rs.logger.Error("error listing files", zap.Error(err))
		return nil
	}

	for _, filePath := range allFiles {
		if filepath.Base(filePath) == "full_list.json" {
			if err := rs.processListFile(filePath, formatter); err != nil {
				rs.logger.Error("error processing file", zap.String("file", filePath), zap.Error(err))
			}
		}
	}

	rs.logger.Info("list renewal process completed")

	return nil
}

// processListFile processes a single list file
func (rs *RenewalService) processListFile(filePath string, formatter *services.StatusListFormatter) error {
	relativePath := rs.convertToRelativePath(filePath)
	dirPath := filepath.Dir(relativePath)

	statusListData, shouldSkip := rs.loadAndValidateData(relativePath, filePath, dirPath)
	if shouldSkip {
		return nil
	}

	// Determine list type and regenerate files
	if strings.Contains(dirPath, "token_status_list") {
		return rs.renewTokenStatusList(dirPath, statusListData, formatter)
	} else if strings.Contains(dirPath, "identifier_list") {
		return rs.renewIdentifierList(dirPath, statusListData, formatter)
	}

	return nil
}

// convertToRelativePath converts an absolute path to relative path if needed
func (rs *RenewalService) convertToRelativePath(filePath string) string {
	if !filepath.IsAbs(filePath) {
		return filePath
	}

	if rel, err := filepath.Rel(rs.config.StatusListDir, filePath); err == nil {
		return rel
	}

	// If we can't make it relative, use the original path and let storage handle it
	return filePath
}

// loadAndValidateData reads and validates the status list data
func (rs *RenewalService) loadAndValidateData(relativePath, filePath, dirPath string) (*models.StatusListData, bool) {
	jsonData, err := rs.storage.Read(relativePath)
	if err != nil {
		return nil, rs.handleReadError(err, filePath)
	}

	var statusListData models.StatusListData
	if err := json.Unmarshal(jsonData, &statusListData); err != nil {
		rs.logger.Error("error unmarshaling file", zap.String("file", filePath), zap.Error(err))
		return nil, true
	}

	if !rs.hasRequiredURIs(&statusListData, filePath) {
		return nil, true
	}

	if rs.isListExpired(&statusListData, dirPath) {
		return nil, true
	}

	return &statusListData, false
}

// handleReadError handles storage read errors and returns whether to skip
func (rs *RenewalService) handleReadError(err error, filePath string) bool {
	if stdErrors.Is(err, errors.ErrNotFound) {
		rs.logger.Debug("file no longer exists, skipping", zap.String("file", filePath))
		return true
	}

	rs.logger.Error("error reading file", zap.String("file", filePath), zap.Error(err))

	return true
}

// hasRequiredURIs checks if the status list has required URIs
func (rs *RenewalService) hasRequiredURIs(statusListData *models.StatusListData, filePath string) bool {
	if statusListData.StatusListURI == "" || statusListData.IdentifierListURI == "" {
		rs.logger.Warn("URIs missing in file", zap.String("file", filePath))
		return false
	}

	return true
}

// isListExpired checks if the status list has expired
func (rs *RenewalService) isListExpired(statusListData *models.StatusListData, dirPath string) bool {
	if statusListData.Expires == nil {
		return false
	}

	expiresDate, err := time.Parse("2006-01-02", *statusListData.Expires)
	if err != nil {
		rs.logger.Error("error parsing expiry date", zap.String("dir", dirPath), zap.Error(err))
		return true
	}

	if expiresDate.Before(time.Now()) {
		rs.logger.Info("list is expired, skipping renewal", zap.String("dir", dirPath))
		return true
	}

	return false
}

// renewTokenStatusList renews token status list files
func (rs *RenewalService) renewTokenStatusList(dirPath string, statusListData *models.StatusListData, formatter *services.StatusListFormatter) error {
	expiryDate := ""
	if statusListData.Expires != nil {
		expiryDate = *statusListData.Expires
	}

	rs.generateAndWrite(dirPath, "token_status_list.jwt", func() ([]byte, error) {
		s, err := formatter.GenerateJWT(statusListData.TokenStatusList, statusListData.Country, statusListData.StatusListURI, expiryDate)
		return []byte(s), err
	}, "JWT")

	rs.generateAndWrite(dirPath, "token_status_list.cwt", func() ([]byte, error) {
		return formatter.GenerateCWT(statusListData.TokenStatusList, statusListData.Country, statusListData.StatusListURI, expiryDate)
	}, "CWT")

	return nil
}

// renewIdentifierList renews identifier list files
func (rs *RenewalService) renewIdentifierList(dirPath string, statusListData *models.StatusListData, formatter *services.StatusListFormatter) error {
	expiryDate := ""
	if statusListData.Expires != nil {
		expiryDate = *statusListData.Expires
	}

	rs.generateAndWrite(dirPath, "identifier_list.jwt", func() ([]byte, error) {
		s, err := formatter.GenerateIdentifierJWT(statusListData.IdentifierList, statusListData.Country, statusListData.IdentifierListURI, expiryDate)
		return []byte(s), err
	}, "identifier JWT")

	rs.generateAndWrite(dirPath, "identifier_list.cwt", func() ([]byte, error) {
		return formatter.GenerateIdentifierCWT(statusListData.IdentifierList, statusListData.Country, statusListData.IdentifierListURI, expiryDate)
	}, "identifier CWT")

	return nil
}

// generateAndWrite handles the pattern of generating content and writing it to a file
func (rs *RenewalService) generateAndWrite(
	dirPath string,
	filename string,
	generateFunc func() ([]byte, error),
	description string,
) {
	content, err := generateFunc()
	if err != nil {
		rs.logger.Error("failed to generate token", zap.String("type", description), zap.String("dir", dirPath), zap.Error(err))
		return
	}

	filePath := filepath.Join(dirPath, filename)
	if err := rs.writeOrCreateFile(filePath, content); err != nil {
		if stdErrors.Is(err, errors.ErrNotFound) {
			rs.logger.Debug("directory no longer exists, skipping write", zap.String("dir", dirPath), zap.String("type", description))
		} else {
			rs.logger.Error("failed to write file", zap.String("type", description), zap.String("file", filePath), zap.Error(err))
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
			rs.logger.Debug("successfully wrote file after retries", zap.String("file", path), zap.Int("attempts", attempt))
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
		rs.logger.Debug("file no longer exists, skipping write", zap.String("file", path))
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
			rs.logger.Debug("file deleted during operation, skipping write", zap.String("file", path))
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
func (rs *RenewalService) handleWriteError(err error, path string, attempt, maxRetries int) (bool, error) {
	if !stdErrors.Is(err, errors.ErrVersionMismatch) {
		return false, err
	}

	if attempt >= maxRetries {
		return false, fmt.Errorf("failed after %d attempts: %w", maxRetries, err)
	}

	rs.logger.Debug("version conflict, retrying", zap.String("file", path), zap.Int("attempt", attempt), zap.Int("maxRetries", maxRetries))
	time.Sleep(time.Millisecond * 100 * time.Duration(attempt))

	return true, nil
}

// copyFile is no longer used with storage abstraction
// Backups should be handled at the infrastructure level

// dailyRenewal runs the renewal process daily at the configured time
func (rs *RenewalService) dailyRenewal() {
	for {
		now := time.Now()
		next := nextRun(now, rs.config.RenewalHour, rs.config.RenewalMinute)
		delay := time.Until(next)

		rs.logger.Debug("next renewal scheduled", zap.Duration("in", delay))

		time.Sleep(delay)

		rs.logger.Info("renewing status lists")

		if err := rs.RenewLists(); err != nil {
			rs.logger.Error("error during renewal", zap.Error(err))
		}
	}
}

// nextRun calculates the next execution time based on configured hour and minute
func nextRun(now time.Time, hour, minute int) time.Time {
	today := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if now.After(today) || now.Equal(today) {
		return today.Add(24 * time.Hour)
	}

	return today
}

// StartRenewalThread starts the renewal thread as a global function
func StartRenewalThread(cfg *config.Config, stor storage.Storage, logger *zap.Logger) {
	if !cfg.RenewalEnabled {
		logger.Info("status list renewal disabled via configuration")
		return
	}

	go func() {
		renewalService := NewRenewalService(cfg, stor, logger)
		renewalService.dailyRenewal()
	}()
}

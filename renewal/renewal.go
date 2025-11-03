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
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unknovs/status-list-go/config"
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
		return fmt.Errorf("status list directory does not exist: %s", rs.config.StatusListDir)
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
	jwtContent, err := formatter.GenerateJWT(statusListData.TokenStatusList, statusListData.Country, statusListData.StatusListURI)
	if err != nil {
		log.Printf("Failed to generate JWT for %s: %v", dirPath, err)
	} else {
		jwtPath := filepath.Join(dirPath, "token_status_list.jwt")
		if err := rs.writeOrCreateFile(jwtPath, []byte(jwtContent)); err != nil {
			log.Printf("Failed to write JWT file %s: %v", jwtPath, err)
		}
	}

	// Regenerate CWT
	cwtContent, err := formatter.GenerateCWT(statusListData.TokenStatusList, statusListData.Country, statusListData.StatusListURI)
	if err != nil {
		log.Printf("Failed to generate CWT for %s: %v", dirPath, err)
	} else {
		cwtPath := filepath.Join(dirPath, "token_status_list.cwt")
		if err := rs.writeOrCreateFile(cwtPath, []byte(cwtContent)); err != nil {
			log.Printf("Failed to write CWT file %s: %v", cwtPath, err)
		}
	}

	return nil
}

// renewIdentifierList renews identifier list files
func (rs *RenewalService) renewIdentifierList(dirPath string, statusListData *models.StatusListData, formatter *services.StatusListFormatter) error {
	// Note: Backup functionality removed as it requires filesystem-specific operations
	// In production, backups should be handled at the infrastructure level (S3 versioning, etc.)

	// Regenerate JWT
	jwtContent, err := formatter.GenerateIdentifierJWT(statusListData.IdentifierList, statusListData.Country, statusListData.IdentifierListURI)
	if err != nil {
		log.Printf("Failed to generate identifier JWT for %s: %v", dirPath, err)
	} else {
		jwtPath := filepath.Join(dirPath, "identifier_list.jwt")
		if err := rs.writeOrCreateFile(jwtPath, []byte(jwtContent)); err != nil {
			log.Printf("Failed to write identifier JWT file %s: %v", jwtPath, err)
		}
	}

	// Regenerate CWT
	cwtContent, err := formatter.GenerateIdentifierCWT(statusListData.IdentifierList, statusListData.Country, statusListData.IdentifierListURI)
	if err != nil {
		log.Printf("Failed to generate identifier CWT for %s: %v", dirPath, err)
	} else {
		cwtPath := filepath.Join(dirPath, "identifier_list.cwt")
		if err := rs.writeOrCreateFile(cwtPath, []byte(cwtContent)); err != nil {
			log.Printf("Failed to write identifier CWT file %s: %v", cwtPath, err)
		}
	}

	return nil
}

// writeOrCreateFile is a helper that creates a file if it doesn't exist, or writes to it if it does
func (rs *RenewalService) writeOrCreateFile(path string, content []byte) error {
	exists, err := rs.storage.Exists(path)
	if err != nil {
		return err
	}

	if exists {
		// For simplicity, we'll use version 2 for updates
		return rs.storage.Write(path, content, 2)
	}

	return rs.storage.Create(path, content)
}

// copyFile is no longer used with storage abstraction
// Backups should be handled at the infrastructure level

// dailyRenewal runs the renewal process daily at midnight
func (rs *RenewalService) dailyRenewal() {
	for {
		now := time.Now()

		var nextExecution time.Time
		if now.Hour() < 12 {
			nextExecution = now.Truncate(time.Hour * 24).Add(12 * time.Hour)
		} else {
			nextExecution = now.Truncate(time.Hour * 24).Add(24 * time.Hour)
		}

		duration := nextExecution.Sub(now)
		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60
		seconds := int(duration.Seconds()) % 60

		log.Printf("Renewing in %02d:%02d:%02d", hours, minutes, seconds)

		time.Sleep(duration)

		log.Println("Renewing Revocation Lists")

		if err := rs.RenewLists(); err != nil {
			log.Printf("Error during renewal: %v", err)
		}
	}
}

// StartRenewalThread starts the renewal thread as a global function
func StartRenewalThread(cfg *config.Config, stor storage.Storage) {
	go func() {
		renewalService := NewRenewalService(cfg, stor)
		renewalService.dailyRenewal()
	}()
}

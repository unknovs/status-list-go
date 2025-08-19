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
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/models"
	"github.com/unknovs/status-list-go/services"
)

// RenewalService handles the renewal of status lists
type RenewalService struct {
	config *config.Config
}

// NewRenewalService creates a new renewal service
func NewRenewalService(cfg *config.Config) *RenewalService {
	return &RenewalService{config: cfg}
}

// RenewLists renews all status lists that haven't expired
func (rs *RenewalService) RenewLists() error {
	baseDir := rs.config.StatusListDir
	backupDir := rs.config.BackupDir
	timestamp := time.Now().Format("2006-01-02_15-04-05")

	log.Println("Starting list renewal process")

	// Create formatter for JWT/CWT generation
	formatter := services.NewStatusListFormatter(rs.config)

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Name() == "full_list.json" {
			return rs.processListFile(path, baseDir, backupDir, timestamp, formatter)
		}

		return nil
	})

	if err != nil {
		log.Printf("Error during renewal: %v", err)
		return err
	}

	log.Println("List renewal process completed")
	return nil
}

// processListFile processes a single list file
func (rs *RenewalService) processListFile(filePath, baseDir, backupDir, timestamp string, formatter *services.StatusListFormatter) error {
	dirPath := filepath.Dir(filePath)

	// Read and parse the list file
	jsonData, err := os.ReadFile(filePath)
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
			log.Printf("Removing %s as it is expired", dirPath)
			return os.RemoveAll(dirPath)
		}
	}

	// Create backup
	relativePath, err := filepath.Rel(baseDir, dirPath)
	if err != nil {
		log.Printf("Error getting relative path for %s: %v", dirPath, err)
		return nil // Continue with other files
	}

	copyDir := filepath.Join(backupDir, timestamp, relativePath)
	if err := os.MkdirAll(copyDir, 0755); err != nil {
		log.Printf("Error creating backup directory %s: %v", copyDir, err)
		return nil // Continue with other files
	}

	// Determine list type and regenerate files
	if strings.Contains(dirPath, "token_status_list") {
		return rs.renewTokenStatusList(dirPath, copyDir, &statusListData, formatter)
	} else if strings.Contains(dirPath, "identifier_list") {
		return rs.renewIdentifierList(dirPath, copyDir, &statusListData, formatter)
	}

	return nil
}

// renewTokenStatusList renews token status list files
func (rs *RenewalService) renewTokenStatusList(dirPath, copyDir string, statusListData *models.StatusListData, formatter *services.StatusListFormatter) error {
	// Backup existing files
	rs.copyFile(filepath.Join(dirPath, "token_status_list.jwt"), filepath.Join(copyDir, "token_status_list.jwt"))
	rs.copyFile(filepath.Join(dirPath, "token_status_list.cwt"), filepath.Join(copyDir, "token_status_list.cwt"))
	rs.copyFile(filepath.Join(dirPath, "full_list.json"), filepath.Join(copyDir, "full_list.json"))

	// Regenerate JWT
	jwtContent, err := formatter.GenerateJWT(statusListData.TokenStatusList, statusListData.Country, statusListData.StatusListURI)
	if err != nil {
		log.Printf("Failed to generate JWT for %s: %v", dirPath, err)
	} else {
		jwtFilePath := filepath.Join(dirPath, "token_status_list.jwt")
		if err := os.WriteFile(jwtFilePath, []byte(jwtContent), 0600); err != nil {
			log.Printf("Failed to write JWT file %s: %v", jwtFilePath, err)
		}
	}

	// Regenerate CWT
	cwtContent, err := formatter.GenerateCWT(statusListData.TokenStatusList, statusListData.Country, statusListData.StatusListURI)
	if err != nil {
		log.Printf("Failed to generate CWT for %s: %v", dirPath, err)
	} else {
		cwtFilePath := filepath.Join(dirPath, "token_status_list.cwt")
		if err := os.WriteFile(cwtFilePath, []byte(cwtContent), 0600); err != nil {
			log.Printf("Failed to write CWT file %s: %v", cwtFilePath, err)
		}
	}

	return nil
}

// renewIdentifierList renews identifier list files
func (rs *RenewalService) renewIdentifierList(dirPath, copyDir string, statusListData *models.StatusListData, formatter *services.StatusListFormatter) error {
	// Backup existing files
	rs.copyFile(filepath.Join(dirPath, "identifier_list.jwt"), filepath.Join(copyDir, "identifier_list.jwt"))
	rs.copyFile(filepath.Join(dirPath, "identifier_list.cwt"), filepath.Join(copyDir, "identifier_list.cwt"))
	rs.copyFile(filepath.Join(dirPath, "full_list.json"), filepath.Join(copyDir, "full_list.json"))

	// Regenerate JWT
	jwtContent, err := formatter.GenerateIdentifierJWT(statusListData.IdentifierList, statusListData.Country, statusListData.IdentifierListURI)
	if err != nil {
		log.Printf("Failed to generate identifier JWT for %s: %v", dirPath, err)
	} else {
		jwtFilePath := filepath.Join(dirPath, "identifier_list.jwt")
		if err := os.WriteFile(jwtFilePath, []byte(jwtContent), 0600); err != nil {
			log.Printf("Failed to write identifier JWT file %s: %v", jwtFilePath, err)
		}
	}

	// Regenerate CWT
	cwtContent, err := formatter.GenerateIdentifierCWT(statusListData.IdentifierList, statusListData.Country, statusListData.IdentifierListURI)
	if err != nil {
		log.Printf("Failed to generate identifier CWT for %s: %v", dirPath, err)
	} else {
		cwtFilePath := filepath.Join(dirPath, "identifier_list.cwt")
		if err := os.WriteFile(cwtFilePath, []byte(cwtContent), 0600); err != nil {
			log.Printf("Failed to write identifier CWT file %s: %v", cwtFilePath, err)
		}
	}

	return nil
}

// copyFile copies a file from src to dst, preserving the source file's permissions
func (rs *RenewalService) copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Get source file info to preserve permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, srcInfo.Mode())
}

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
func StartRenewalThread(cfg *config.Config) {
	go func() {
		renewalService := NewRenewalService(cfg)
		renewalService.dailyRenewal()
	}()
}

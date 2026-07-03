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

package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/models"
	"github.com/unknovs/status-list-go/services/storage"

	"github.com/google/uuid"
)

const (
	FullListJSONFile = "full_list.json"
)

// ListManager manages status lists and identifier lists
type ListManager struct {
	config     *config.Config
	storage    storage.Storage
	statusList map[string]map[string]*models.StatusListData
	mutex      sync.RWMutex
}

// NewListManager creates a new list manager
func NewListManager(cfg *config.Config, stor storage.Storage) *ListManager {
	return &ListManager{
		config:     cfg,
		storage:    stor,
		statusList: make(map[string]map[string]*models.StatusListData),
	}
}

// GetStorage returns the storage backend for direct access when needed
func (lm *ListManager) GetStorage() storage.Storage {
	return lm.storage
}

// NewList initializes a new status list for a country and doctype.
// It is a no-op if the list already exists in memory, preventing TOCTOU overwrites.
func (lm *ListManager) NewList(country, doctype string) {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()

	if lm.statusList[country] == nil {
		lm.statusList[country] = make(map[string]*models.StatusListData)
	}

	if lm.statusList[country][doctype] != nil {
		return
	}

	newRand := uuid.New().String()
	lm.statusList[country][doctype] = &models.StatusListData{
		TokenStatusList: models.NewIssuerStatusList(1, lm.config.TokenStatusListSize, "random"),
		IdentifierList:  make(map[string]int),
		Expires:         nil,
		Rand:            newRand,
	}
}

// DumpList saves the status list to disk
func (lm *ListManager) DumpList(statusListData *models.StatusListData, country, doctype string) error {
	rand := statusListData.Rand

	statusListURI, identifierListURI := lm.buildURIs(country, doctype, rand)

	// Update URIs in the status list data before marshaling
	statusListData.StatusListURI = statusListURI
	statusListData.IdentifierListURI = identifierListURI

	// Copy data and set metadata
	statusListCopy := *statusListData
	statusListCopy.Country = country
	statusListCopy.Doctype = doctype

	jsonData, err := json.Marshal(statusListCopy)
	if err != nil {
		return err
	}

	if err := lm.saveJSONFiles(country, doctype, rand, jsonData); err != nil {
		return err
	}

	if err := lm.saveFormatFiles(statusListData, country, doctype, rand, statusListURI, identifierListURI); err != nil {
		return err
	}

	return nil
}

// saveJSONFiles saves both token and identifier JSON files
func (lm *ListManager) saveJSONFiles(country, doctype, rand string, jsonData []byte) error {
	tokenJSONPath := filepath.Join("token_status_list", country, doctype, rand, FullListJSONFile)
	if err := lm.saveJSONFile(tokenJSONPath, jsonData, "token"); err != nil {
		return err
	}

	identifierJSONPath := filepath.Join("identifier_list", country, doctype, rand, FullListJSONFile)
	return lm.saveJSONFile(identifierJSONPath, jsonData, "identifier")
}

// saveJSONFile saves a single JSON file, creating or updating as needed
func (lm *ListManager) saveJSONFile(path string, jsonData []byte, fileType string) error {
	exists, err := lm.storage.Exists(path)
	if err != nil {
		return fmt.Errorf("failed to check %s file existence: %w", fileType, err)
	}

	if exists {
		currentVersion, err := lm.storage.GetVersion(path)
		if err != nil {
			return fmt.Errorf("failed to get %s file version: %w", fileType, err)
		}
		if err := lm.storage.Write(path, jsonData, currentVersion+1); err != nil {
			return fmt.Errorf("failed to update %s JSON: %w", fileType, err)
		}
	} else {
		if err := lm.storage.Create(path, jsonData); err != nil {
			return fmt.Errorf("failed to create %s JSON: %w", fileType, err)
		}
	}

	return nil
}

// buildURIs constructs the status list and identifier list URIs
func (lm *ListManager) buildURIs(country, doctype, rand string) (string, string) {
	baseURL := strings.TrimSuffix(lm.config.ServiceURL, "/") + "/"
	statusListURI := baseURL + fmt.Sprintf("token_status_list/%s/%s/%s", country, doctype, rand)
	identifierListURI := baseURL + fmt.Sprintf("identifier_list/%s/%s/%s", country, doctype, rand)
	return statusListURI, identifierListURI
}

// saveFormatFiles generates and saves all JWT and CWT format files
func (lm *ListManager) saveFormatFiles(statusListData *models.StatusListData, country, doctype, rand, statusListURI, identifierListURI string) error {
	// Token status list formats
	if err := lm.saveTokenStatusListFormats(statusListData, country, doctype, rand, statusListURI); err != nil {
		return err
	}

	// Identifier list formats
	return lm.saveIdentifierListFormats(statusListData, country, doctype, rand, identifierListURI)
}

// saveTokenStatusListFormats generates and saves JWT and CWT for token status list
func (lm *ListManager) saveTokenStatusListFormats(statusListData *models.StatusListData, country, doctype, rand, statusListURI string) error {
	expiryDate := ""
	if statusListData.Expires != nil {
		expiryDate = *statusListData.Expires
	}

	jwtContent, err := lm.generateJWTFormat(statusListData.TokenStatusList, country, statusListURI, expiryDate)
	if err != nil {
		log.Printf("Failed to generate JWT: %v", err)
	} else {
		jwtPath := filepath.Join("token_status_list", country, doctype, rand, "token_status_list.jwt")
		if err := lm.writeOrCreateFile(jwtPath, []byte(jwtContent)); err != nil {
			return fmt.Errorf("failed to save JWT: %w", err)
		}
	}

	cwtContent, err := lm.generateCWTFormat(statusListData.TokenStatusList, country, statusListURI, expiryDate)
	if err != nil {
		log.Printf("Failed to generate CWT: %v", err)
	} else {
		cwtPath := filepath.Join("token_status_list", country, doctype, rand, "token_status_list.cwt")
		if err := lm.writeOrCreateFile(cwtPath, cwtContent); err != nil {
			return fmt.Errorf("failed to save CWT: %w", err)
		}
	}

	return nil
}

// saveIdentifierListFormats generates and saves JWT and CWT for identifier list
func (lm *ListManager) saveIdentifierListFormats(statusListData *models.StatusListData, country, doctype, rand, identifierListURI string) error {
	expiryDate := ""
	if statusListData.Expires != nil {
		expiryDate = *statusListData.Expires
	}

	identifierJWTContent, err := lm.generateIdentifierJWTFormat(statusListData.IdentifierList, country, identifierListURI, expiryDate)
	if err != nil {
		log.Printf("Failed to generate identifier JWT: %v", err)
	} else {
		identifierJWTPath := filepath.Join("identifier_list", country, doctype, rand, "identifier_list.jwt")
		if err := lm.writeOrCreateFile(identifierJWTPath, []byte(identifierJWTContent)); err != nil {
			return fmt.Errorf("failed to save identifier JWT: %w", err)
		}
	}

	identifierCWTContent, err := lm.generateIdentifierCWTFormat(statusListData.IdentifierList, country, identifierListURI, expiryDate)
	if err != nil {
		log.Printf("Failed to generate identifier CWT: %v", err)
	} else {
		identifierCWTPath := filepath.Join("identifier_list", country, doctype, rand, "identifier_list.cwt")
		if err := lm.writeOrCreateFile(identifierCWTPath, identifierCWTContent); err != nil {
			return fmt.Errorf("failed to save identifier CWT: %w", err)
		}
	}

	return nil
}

// writeOrCreateFile is a helper that creates a file if it doesn't exist, or writes to it if it does
func (lm *ListManager) writeOrCreateFile(path string, content []byte) error {
	exists, err := lm.storage.Exists(path)
	if err != nil {
		return err
	}

	if exists {
		// Get current version and increment it
		currentVersion, err := lm.storage.GetVersion(path)
		if err != nil {
			return fmt.Errorf("failed to get current version: %w", err)
		}
		return lm.storage.Write(path, content, currentVersion+1)
	}

	return lm.storage.Create(path, content)
}

// LoadList loads a status list from disk
func (lm *ListManager) LoadList(uri string) (*models.StatusListData, error) {
	parsedURI, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	// Construct path relative to storage root (remove leading slash)
	relativePath := strings.TrimPrefix(parsedURI.Path, "/")

	if lm.config.BasePath != "" {
		trimmedBase := strings.TrimPrefix(lm.config.BasePath, "/")
		if trimmedBase != "" {
			if relativePath == trimmedBase {
				relativePath = ""
			} else if strings.HasPrefix(relativePath, trimmedBase+"/") {
				relativePath = strings.TrimPrefix(relativePath, trimmedBase+"/")
			}
		}
	}
	folderPath := filepath.Join(relativePath, FullListJSONFile)

	jsonData, err := lm.storage.Read(folderPath)
	if err != nil {
		return nil, err
	}

	var statusListData models.StatusListData
	if err := json.Unmarshal(jsonData, &statusListData); err != nil {
		return nil, err
	}

	return &statusListData, nil
}

// TakeIndexList takes a new index from the list.
// It holds the write mutex for the entire operation to prevent concurrent allocation
// and avoid deadlock ā€” the recursive call that existed here has been replaced with
// an in-place list rotation, because sync.Mutex is not reentrant.
func (lm *ListManager) TakeIndexList(country, doctype, expiryDate string) (int, error) {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()

	if lm.statusList[country] == nil {
		lm.statusList[country] = make(map[string]*models.StatusListData)
	}

	if lm.statusList[country][doctype] == nil {
		newRand := uuid.New().String()
		lm.statusList[country][doctype] = &models.StatusListData{
			TokenStatusList: models.NewIssuerStatusList(1, lm.config.TokenStatusListSize, "random"),
			IdentifierList:  make(map[string]int),
			Expires:         &expiryDate,
			Rand:            newRand,
		}
	}

	statusListData := lm.statusList[country][doctype]

	index, err := statusListData.TokenStatusList.Allocator.Take()
	if err != nil {
		if dumpErr := lm.DumpList(statusListData, country, doctype); dumpErr != nil {
			return 0, fmt.Errorf("failed to persist full status list: %w", dumpErr)
		}

		newRand := uuid.New().String()
		lm.statusList[country][doctype] = &models.StatusListData{
			TokenStatusList: models.NewIssuerStatusList(1, lm.config.TokenStatusListSize, "random"),
			IdentifierList:  make(map[string]int),
			Expires:         &expiryDate,
			Rand:            newRand,
		}
		statusListData = lm.statusList[country][doctype]

		index, err = statusListData.TokenStatusList.Allocator.Take()
		if err != nil {
			return 0, fmt.Errorf("allocator empty on freshly created list: %w", err)
		}
	}

	// Update expiry date to the latest one
	if statusListData.Expires == nil {
		statusListData.Expires = &expiryDate
	} else {
		currentExp, _ := time.Parse("2006-01-02", *statusListData.Expires)
		newExp, _ := time.Parse("2006-01-02", expiryDate)
		if newExp.After(currentExp) {
			statusListData.Expires = &expiryDate
		}
	}

	if err := lm.DumpList(statusListData, country, doctype); err != nil {
		return 0, err
	}

	return index, nil
}

// GenerateStatusListInfo generates the structure sent to the issuer.
func (lm *ListManager) GenerateStatusListInfo(country, doctype, expiryDate string) (*models.StatusListInfo, error) {
	index, err := lm.TakeIndexList(country, doctype, expiryDate)
	if err != nil {
		return nil, err
	}

	lm.mutex.RLock()
	statusListData := lm.statusList[country][doctype]
	if statusListData == nil {
		lm.mutex.RUnlock()
		return nil, fmt.Errorf("list not found in memory after allocation for %s/%s", country, doctype)
	}
	statusListURI := statusListData.StatusListURI
	identifierListURI := statusListData.IdentifierListURI
	lm.mutex.RUnlock()

	statusListInfo := &models.StatusListInfo{}
	statusListInfo.StatusList.URI = statusListURI
	statusListInfo.StatusList.Idx = index
	statusListInfo.IdentifierList.URI = identifierListURI
	statusListInfo.IdentifierList.ID = fmt.Sprintf("%d", index)

	return statusListInfo, nil
}

// GetStatusFromURI gets status from a URI and index
func (lm *ListManager) GetStatusFromURI(uri string, index int) (int, error) {
	tempList, err := lm.LoadList(uri)
	if err != nil {
		return 0, err
	}

	if strings.Contains(uri, "token_status_list") {
		return tempList.TokenStatusList.StatusList.Get(index), nil
	} else if strings.Contains(uri, "identifier_list") {
		if status, exists := tempList.IdentifierList[fmt.Sprintf("%d", index)]; exists {
			return status, nil
		}
		return 0, nil
	}

	return 0, fmt.Errorf("unknown URI type")
}

// SetStatus updates the status at a given index
func (lm *ListManager) SetStatus(uri, country, doctype, listID string, index, status int) error {
	tempList, err := lm.LoadList(uri)
	if err != nil {
		return err
	}

	// Update the status
	tempList.TokenStatusList.StatusList.Set(index, status)
	tempList.IdentifierList[fmt.Sprintf("%d", index)] = status

	// Update the in-memory status list if it matches
	lm.mutex.Lock()
	if lm.statusList[country] != nil && lm.statusList[country][doctype] != nil &&
		lm.statusList[country][doctype].Rand == listID {
		lm.statusList[country][doctype].TokenStatusList.StatusList.Set(index, status)
		if lm.statusList[country][doctype].IdentifierList == nil {
			lm.statusList[country][doctype].IdentifierList = make(map[string]int)
		}
		lm.statusList[country][doctype].IdentifierList[fmt.Sprintf("%d", index)] = status
	}
	lm.mutex.Unlock()

	// Save the updated list
	return lm.DumpList(tempList, country, doctype)
}

// generateJWTFormat generates JWT format
func (lm *ListManager) generateJWTFormat(tokenStatusList *models.IssuerStatusList, country, listURL, expiryDate string) (string, error) {
	formatter := NewStatusListFormatter(lm.config)
	return formatter.GenerateJWT(tokenStatusList, country, listURL, expiryDate)
}

// generateCWTFormat generates CWT format
func (lm *ListManager) generateCWTFormat(tokenStatusList *models.IssuerStatusList, country, listURL, expiryDate string) ([]byte, error) {
	formatter := NewStatusListFormatter(lm.config)
	return formatter.GenerateCWT(tokenStatusList, country, listURL, expiryDate)
}

// generateIdentifierJWTFormat generates identifier JWT format
func (lm *ListManager) generateIdentifierJWTFormat(identifierList map[string]int, country, listURL, expiryDate string) (string, error) {
	formatter := NewStatusListFormatter(lm.config)
	return formatter.GenerateIdentifierJWT(identifierList, country, listURL, expiryDate)
}

// generateIdentifierCWTFormat generates identifier CWT format
func (lm *ListManager) generateIdentifierCWTFormat(identifierList map[string]int, country, listURL, expiryDate string) ([]byte, error) {
	formatter := NewStatusListFormatter(lm.config)
	return formatter.GenerateIdentifierCWT(identifierList, country, listURL, expiryDate)
}

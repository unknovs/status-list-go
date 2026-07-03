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
	stdErrors "errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/unknovs/status-list-go/config"
	localerrors "github.com/unknovs/status-list-go/errors"
	"github.com/unknovs/status-list-go/models"
)

// MockStorage implements the Storage interface for testing
type MockStorage struct {
	files   map[string][]byte
	mutex   sync.RWMutex
	version map[string]int
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		files:   make(map[string][]byte),
		version: make(map[string]int),
	}
}

func (m *MockStorage) Create(path string, content []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Normalize path to use forward slashes
	path = strings.ReplaceAll(path, "\\", "/")

	if _, exists := m.files[path]; exists {
		return fmt.Errorf("file already exists: %s", path)
	}

	m.files[path] = content
	m.version[path] = 1
	return nil
}

func (m *MockStorage) Read(path string) ([]byte, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Normalize path to use forward slashes
	path = strings.ReplaceAll(path, "\\", "/")

	content, exists := m.files[path]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	return content, nil
}

func (m *MockStorage) Write(path string, content []byte, version int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Normalize path to use forward slashes
	path = strings.ReplaceAll(path, "\\", "/")

	if _, exists := m.files[path]; !exists {
		return fmt.Errorf("file not found: %s", path)
	}

	// For testing, don't check version
	m.files[path] = content
	m.version[path] = version + 1
	return nil
}

func (m *MockStorage) Exists(path string) (bool, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Normalize path to use forward slashes
	path = strings.ReplaceAll(path, "\\", "/")

	_, exists := m.files[path]
	return exists, nil
}

func (m *MockStorage) List(prefix string) ([]string, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Normalize prefix to use forward slashes
	prefix = strings.ReplaceAll(prefix, "\\", "/")

	var paths []string
	for path := range m.files {
		// Paths are already normalized in storage
		if prefix == "" || strings.HasPrefix(path, prefix) {
			paths = append(paths, path)
		}
	}

	return paths, nil
}

func (m *MockStorage) GetVersion(path string) (int, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Normalize path to use forward slashes
	path = strings.ReplaceAll(path, "\\", "/")

	version, exists := m.version[path]
	if !exists {
		return 0, fmt.Errorf("file not found: %s", path)
	}

	return version, nil
}

func (m *MockStorage) DeleteTree(prefix string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	prefix = strings.ReplaceAll(prefix, "\\", "/")
	if strings.TrimSpace(prefix) == "" {
		return fmt.Errorf("prefix is required")
	}

	for path := range m.files {
		if strings.HasPrefix(path, prefix) {
			delete(m.files, path)
			delete(m.version, path)
		}
	}

	return nil
}

// TestNewListManager tests the creation of a new ListManager
func TestNewListManager(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 100,
	}
	storage := NewMockStorage()

	lm := NewListManager(cfg, storage)

	if lm == nil {
		t.Fatal("Expected ListManager to be created, got nil")
	}

	if lm.config != cfg {
		t.Error("Config not set correctly")
	}

	if lm.storage != storage {
		t.Error("Storage not set correctly")
	}

	if lm.statusList == nil {
		t.Error("Status list map should be initialized")
	}
}

// TestNewList tests creating a new status list
func TestNewList(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 100,
	}
	storage := NewMockStorage()
	lm := NewListManager(cfg, storage)

	country := "DE"
	doctype := "mDL"

	lm.NewList(country, doctype)

	if lm.statusList[country] == nil {
		t.Fatal("Country entry not created")
	}

	if lm.statusList[country][doctype] == nil {
		t.Fatal("Doctype entry not created")
	}

	statusData := lm.statusList[country][doctype]
	if statusData.TokenStatusList == nil {
		t.Error("TokenStatusList not initialized")
	}

	if statusData.IdentifierList == nil {
		t.Error("IdentifierList not initialized")
	}

	if statusData.Rand == "" {
		t.Error("Rand should be set")
	}
}

// TestDumpList tests saving a status list to storage
func TestDumpList(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 100,
		PrivKeyPath:         "",
		CertPath:            "",
		CountryCode:         "DE",
	}
	storage := NewMockStorage()
	lm := NewListManager(cfg, storage)

	country := "DE"
	doctype := "mDL"
	lm.NewList(country, doctype)

	statusData := lm.statusList[country][doctype]

	err := lm.DumpList(statusData, country, doctype)
	if err != nil {
		t.Fatalf("DumpList failed: %v", err)
	}

	// Debug: Check what files exist in storage
	allFiles, _ := storage.List("")
	t.Logf("Files in storage after DumpList: %v", allFiles)

	// Verify JSON files were created
	tokenJSONPath := fmt.Sprintf("token_status_list/%s/%s/%s/full_list.json", country, doctype, statusData.Rand)
	identifierJSONPath := fmt.Sprintf("identifier_list/%s/%s/%s/full_list.json", country, doctype, statusData.Rand)

	exists, _ := storage.Exists(tokenJSONPath)
	if !exists {
		t.Errorf("Token JSON file not created at %s", tokenJSONPath)
	}

	exists, _ = storage.Exists(identifierJSONPath)
	if !exists {
		t.Errorf("Identifier JSON file not created at %s", identifierJSONPath)
	}

	// Verify content
	content, err := storage.Read(tokenJSONPath)
	if err != nil {
		t.Fatalf("Failed to read token JSON: %v", err)
	}

	var loadedData models.StatusListData
	if err := json.Unmarshal(content, &loadedData); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if loadedData.Country != country {
		t.Errorf("Expected country %s, got %s", country, loadedData.Country)
	}

	if loadedData.Doctype != doctype {
		t.Errorf("Expected doctype %s, got %s", doctype, loadedData.Doctype)
	}
}

// TestDumpListUpdate tests updating an existing status list
func TestDumpListUpdate(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 100,
		PrivKeyPath:         "",
		CertPath:            "",
		CountryCode:         "DE",
	}
	storage := NewMockStorage()
	lm := NewListManager(cfg, storage)

	country := "DE"
	doctype := "mDL"
	lm.NewList(country, doctype)

	statusData := lm.statusList[country][doctype]

	// First dump
	err := lm.DumpList(statusData, country, doctype)
	if err != nil {
		t.Fatalf("First DumpList failed: %v", err)
	}

	// Modify the status list
	statusData.TokenStatusList.StatusList.Set(0, 1)

	// Second dump (update)
	err = lm.DumpList(statusData, country, doctype)
	if err != nil {
		t.Fatalf("Second DumpList failed: %v", err)
	}

	// Verify the update
	tokenJSONPath := fmt.Sprintf("token_status_list/%s/%s/%s/full_list.json", country, doctype, statusData.Rand)
	content, err := storage.Read(tokenJSONPath)
	if err != nil {
		t.Fatalf("Failed to read updated token JSON: %v", err)
	}

	var loadedData models.StatusListData
	if err := json.Unmarshal(content, &loadedData); err != nil {
		t.Fatalf("Failed to unmarshal updated JSON: %v", err)
	}

	// Verify the status was updated
	if loadedData.TokenStatusList.StatusList.Get(0) != 1 {
		t.Errorf("Expected status at index 0 to be 1, got %d", loadedData.TokenStatusList.StatusList.Get(0))
	}
}

// TestLoadList tests loading a status list from storage
func TestLoadList(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 100,
		PrivKeyPath:         "",
		CertPath:            "",
		CountryCode:         "DE",
	}
	storage := NewMockStorage()
	lm := NewListManager(cfg, storage)

	country := "DE"
	doctype := "mDL"
	lm.NewList(country, doctype)

	statusData := lm.statusList[country][doctype]

	// Dump the list
	err := lm.DumpList(statusData, country, doctype)
	if err != nil {
		t.Fatalf("DumpList failed: %v", err)
	}

	// Construct URI
	uri := fmt.Sprintf("http://localhost:8081/token_status_list/%s/%s/%s", country, doctype, statusData.Rand)

	// Load the list
	loadedData, err := lm.LoadList(uri)
	if err != nil {
		t.Fatalf("LoadList failed: %v", err)
	}

	if loadedData.Country != country {
		t.Errorf("Expected country %s, got %s", country, loadedData.Country)
	}

	if loadedData.Doctype != doctype {
		t.Errorf("Expected doctype %s, got %s", doctype, loadedData.Doctype)
	}
}

// TestLoadListRejectsPathTraversal ensures an attacker-supplied URI cannot use
// ".." segments to escape the storage root when the list path is derived.
func TestLoadListRejectsPathTraversal(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 100,
		CountryCode:         "DE",
	}
	lm := NewListManager(cfg, NewMockStorage())

	traversalURIs := []string{
		"http://localhost:8081/token_status_list/../../../../etc",
		"http://localhost:8081/../../../../etc/secret",
		"http://localhost:8081/token_status_list/DE/../../../../../root",
	}

	for _, uri := range traversalURIs {
		t.Run(uri, func(t *testing.T) {
			_, err := lm.LoadList(uri)
			if err == nil {
				t.Fatalf("LoadList(%q) should reject path traversal, got nil error", uri)
			}

			if !stdErrors.Is(err, localerrors.ErrPathTraversal) {
				t.Fatalf("LoadList(%q) error = %v, want it to wrap ErrPathTraversal", uri, err)
			}
		})
	}
}

// TestIsContainedPath covers the traversal-containment helper directly.
func TestIsContainedPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"token_status_list/DE/mDL/abc/full_list.json", true},
		{"identifier_list/DE/mDL/abc/full_list.json", true},
		{"../full_list.json", false},
		{"../../etc/full_list.json", false},
		{"token_status_list/../../../etc/full_list.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isContainedPath(tt.path); got != tt.expected {
				t.Errorf("isContainedPath(%q) = %v, expected %v", tt.path, got, tt.expected)
			}
		})
	}
}

// TestTakeIndexList tests taking indices from a status list
func TestTakeIndexList(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 10, // Small size for testing
		PrivKeyPath:         "",
		CertPath:            "",
		CountryCode:         "DE",
	}
	storage := NewMockStorage()
	lm := NewListManager(cfg, storage)

	country := "DE"
	doctype := "mDL"
	expiryDate := "2025-12-31"

	// Take first index
	index1, err := lm.TakeIndexList(country, doctype, expiryDate)
	if err != nil {
		t.Fatalf("TakeIndexList failed: %v", err)
	}

	if index1 < 0 {
		t.Errorf("Expected valid index, got %d", index1)
	}

	// Take second index
	index2, err := lm.TakeIndexList(country, doctype, expiryDate)
	if err != nil {
		t.Fatalf("Second TakeIndexList failed: %v", err)
	}

	if index2 < 0 {
		t.Errorf("Expected valid second index, got %d", index2)
	}

	// Verify indices are different
	if index1 == index2 {
		t.Errorf("Expected different indices, got %d and %d", index1, index2)
	}

	// Verify expiry date was set
	statusData := lm.statusList[country][doctype]
	if statusData.Expires == nil {
		t.Error("Expiry date should be set")
	} else if *statusData.Expires != expiryDate {
		t.Errorf("Expected expiry %s, got %s", expiryDate, *statusData.Expires)
	}
}

// TestGenerateStatusListInfo tests generating status list info
func TestGenerateStatusListInfo(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 100,
		PrivKeyPath:         "",
		CertPath:            "",
		CountryCode:         "DE",
	}
	storage := NewMockStorage()
	lm := NewListManager(cfg, storage)

	country := "DE"
	doctype := "mDL"
	expiryDate := "2025-12-31"

	info, err := lm.GenerateStatusListInfo(country, doctype, expiryDate)
	if err != nil {
		t.Fatalf("GenerateStatusListInfo failed: %v", err)
	}

	if info == nil {
		t.Fatal("Expected StatusListInfo, got nil")
	}

	if info.StatusList.URI == "" {
		t.Error("StatusList URI should not be empty")
	}

	if info.StatusList.Idx < 0 {
		t.Error("StatusList index should be valid")
	}

	if info.IdentifierList.URI == "" {
		t.Error("IdentifierList URI should not be empty")
	}

	if info.IdentifierList.ID == "" {
		t.Error("IdentifierList ID should not be empty")
	}
}

// TestGetStatusFromURI tests getting status from a URI
func TestGetStatusFromURI(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 100,
		PrivKeyPath:         "c:\\code\\github\\gatisb\\status-list-go\\temp\\private_key\\decrypted_key.pem",
		CertPath:            "c:\\code\\github\\gatisb\\status-list-go\\temp\\certificate\\certificate.pem",
		CountryCode:         "DE",
	}
	storage := NewMockStorage()
	lm := NewListManager(cfg, storage)

	country := "DE"
	doctype := "mDL"
	lm.NewList(country, doctype)

	statusData := lm.statusList[country][doctype]

	// Set a status
	statusData.TokenStatusList.StatusList.Set(5, 1)

	// Dump the list
	err := lm.DumpList(statusData, country, doctype)
	if err != nil {
		t.Fatalf("DumpList failed: %v", err)
	}

	// Construct URI
	uri := fmt.Sprintf("http://localhost:8081/token_status_list/%s/%s/%s", country, doctype, statusData.Rand)

	// Get status
	status, err := lm.GetStatusFromURI(uri, 5)
	if err != nil {
		t.Fatalf("GetStatusFromURI failed: %v", err)
	}

	if status != 1 {
		t.Errorf("Expected status 1, got %d", status)
	}
}

// TestSetStatus tests setting a status
func TestSetStatus(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 100,
		PrivKeyPath:         "c:\\code\\github\\gatisb\\status-list-go\\temp\\private_key\\decrypted_key.pem",
		CertPath:            "c:\\code\\github\\gatisb\\status-list-go\\temp\\certificate\\certificate.pem",
		CountryCode:         "DE",
	}
	storage := NewMockStorage()
	lm := NewListManager(cfg, storage)

	country := "DE"
	doctype := "mDL"
	lm.NewList(country, doctype)

	statusData := lm.statusList[country][doctype]
	listID := statusData.Rand

	// Dump the list
	err := lm.DumpList(statusData, country, doctype)
	if err != nil {
		t.Fatalf("DumpList failed: %v", err)
	}

	// Construct URI
	uri := fmt.Sprintf("http://localhost:8081/token_status_list/%s/%s/%s", country, doctype, listID)

	// Set status
	err = lm.SetStatus(uri, country, doctype, listID, 10, 1)
	if err != nil {
		t.Fatalf("SetStatus failed: %v", err)
	}

	// Verify status was set
	status, err := lm.GetStatusFromURI(uri, 10)
	if err != nil {
		t.Fatalf("GetStatusFromURI failed: %v", err)
	}

	if status != 1 {
		t.Errorf("Expected status 1, got %d", status)
	}

	// Verify in-memory status was updated
	if lm.statusList[country][doctype].TokenStatusList.StatusList.Get(10) != 1 {
		t.Errorf("In-memory status should be updated")
	}

	if lm.statusList[country][doctype].IdentifierList["10"] != 1 {
		t.Error("In-memory identifier list should be updated")
	}
}

// conflictInjectingStorage wraps MockStorage and forces the next N versioned writes to
// fail with ErrVersionMismatch, simulating a concurrent revocation that won the race.
type conflictInjectingStorage struct {
	*MockStorage
	writeFailures int
}

func (c *conflictInjectingStorage) Write(path string, content []byte, version int) error {
	if c.writeFailures > 0 {
		c.writeFailures--
		return fmt.Errorf("failed to update JSON: %w", localerrors.ErrVersionMismatch)
	}

	return c.MockStorage.Write(path, content, version)
}

// TestSetStatusRetriesOnVersionMismatch verifies that a version conflict during persist is
// retried with a fresh load instead of being silently dropped (finding 5.1).
func TestSetStatusRetriesOnVersionMismatch(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 100,
		CountryCode:         "DE",
	}
	stor := &conflictInjectingStorage{MockStorage: NewMockStorage()}
	lm := NewListManager(cfg, stor)

	country := "DE"
	doctype := "mDL"
	lm.NewList(country, doctype)

	statusData := lm.statusList[country][doctype]
	listID := statusData.Rand

	if err := lm.DumpList(statusData, country, doctype); err != nil {
		t.Fatalf("DumpList failed: %v", err)
	}

	uri := fmt.Sprintf("http://localhost:8081/token_status_list/%s/%s/%s", country, doctype, listID)

	// Force the first persisted write to conflict; SetStatus must reload and retry.
	stor.writeFailures = 1

	if err := lm.SetStatus(uri, country, doctype, listID, 10, 1); err != nil {
		t.Fatalf("SetStatus should retry past a version conflict, got: %v", err)
	}

	if stor.writeFailures != 0 {
		t.Fatalf("expected injected conflict to be consumed, %d remaining", stor.writeFailures)
	}

	status, err := lm.GetStatusFromURI(uri, 10)
	if err != nil {
		t.Fatalf("GetStatusFromURI failed: %v", err)
	}

	if status != 1 {
		t.Errorf("expected status 1 after retry, got %d", status)
	}
}

// TestSetStatusFailsAfterPersistentConflict verifies that a conflict on every attempt
// surfaces an error rather than looping forever or reporting false success (finding 5.1).
func TestSetStatusFailsAfterPersistentConflict(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 100,
		CountryCode:         "DE",
	}
	stor := &conflictInjectingStorage{MockStorage: NewMockStorage()}
	lm := NewListManager(cfg, stor)

	country := "DE"
	doctype := "mDL"
	lm.NewList(country, doctype)

	statusData := lm.statusList[country][doctype]
	listID := statusData.Rand

	if err := lm.DumpList(statusData, country, doctype); err != nil {
		t.Fatalf("DumpList failed: %v", err)
	}

	uri := fmt.Sprintf("http://localhost:8081/token_status_list/%s/%s/%s", country, doctype, listID)

	// Conflict on every attempt (more than maxAttempts).
	stor.writeFailures = 100

	err := lm.SetStatus(uri, country, doctype, listID, 10, 1)
	if err == nil {
		t.Fatal("expected SetStatus to fail after exhausting retries")
	}

	if !stdErrors.Is(err, localerrors.ErrVersionMismatch) {
		t.Errorf("expected wrapped ErrVersionMismatch, got: %v", err)
	}
}

// TestConcurrentAccess tests concurrent access to ListManager
func TestConcurrentAccess(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:          "http://localhost:8081/",
		TokenStatusListSize: 1000,
		PrivKeyPath:         "c:\\code\\github\\gatisb\\status-list-go\\temp\\private_key\\decrypted_key.pem",
		CertPath:            "c:\\code\\github\\gatisb\\status-list-go\\temp\\certificate\\certificate.pem",
		CountryCode:         "DE",
	}
	storage := NewMockStorage()
	lm := NewListManager(cfg, storage)

	country := "DE"
	doctype := "mDL"
	expiryDate := "2025-12-31"

	// Concurrently take indices
	var wg sync.WaitGroup
	indices := make([]int, 10)
	errors := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			index, err := lm.TakeIndexList(country, doctype, expiryDate)
			indices[idx] = index
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// Check for errors and collect successful indices
	var successfulIndices []int
	for i, err := range errors {
		if err != nil {
			t.Errorf("Goroutine %d failed: %v", i, err)
		} else {
			successfulIndices = append(successfulIndices, indices[i])
		}
	}

	// Verify all successful indices are unique
	uniqueIndices := make(map[int]bool)
	for _, index := range successfulIndices {
		if uniqueIndices[index] {
			t.Errorf("Duplicate index found: %d", index)
		}
		uniqueIndices[index] = true
	}
}

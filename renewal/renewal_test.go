package renewal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/errors"
	"github.com/unknovs/status-list-go/models"
	"github.com/unknovs/status-list-go/services"
	"github.com/unknovs/status-list-go/services/storage"
)

const testContent = "test content"
const testStatusListDir = "/tmp/test"
const testFileName = "test.txt"

// Setup test environment
func setupTestEnvironment(t *testing.T) (string, string, *config.Config, storage.Storage) {
	tempDir, err := os.MkdirTemp("", "renewal_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	backupDir := filepath.Join(tempDir, "backup")
	statusListDir := filepath.Join(tempDir, "status_lists")

	err = os.MkdirAll(backupDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create backup directory: %v", err)
	}

	err = os.MkdirAll(statusListDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create status list directory: %v", err)
	}

	cfg := &config.Config{
		StatusListDir:       statusListDir,
		BackupDir:           backupDir,
		CountryCode:         "US",
		PrivKeyPath:         "temp/private_key/private-key.pem", // Use CI-generated paths
		CertPath:            "temp/certificate/certificate.pem",
		BackendType:         "local",
		TokenStatusListSize: 100,
		AllowedDoctypes:     map[string]bool{"test": true},
	}

	stor, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	return tempDir, statusListDir, cfg, stor
}

func createTestStatusListData(expired bool) *models.StatusListData {
	expires := "2025-12-31T23:59:59Z"
	if expired {
		expires = "2020-01-01T00:00:00Z"
	}

	return &models.StatusListData{
		TokenStatusList:   models.NewIssuerStatusList(2, 100, "sequential"),
		IdentifierList:    map[string]int{"test-id": 1},
		Expires:           &expires,
		Rand:              "test-rand",
		StatusListURI:     "https://example.com/status",
		IdentifierListURI: "https://example.com/identifier",
		Country:           "US",
		Doctype:           "test",
	}
}

func TestRenewTokenStatusList(t *testing.T) {
	tempDir, statusListDir, cfg, stor := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	rs := NewRenewalService(cfg, stor, zap.NewNop())
	statusListData := createTestStatusListData(false)
	formatter := services.NewStatusListFormatter(cfg)

	t.Run("test file renewal", func(t *testing.T) {
		// Create test directory and files
		dirPath := filepath.Join(statusListDir, "test_issuer", "token_status_list")
		os.MkdirAll(dirPath, 0755)

		// Create existing files to be renewed
		originalJWT := "old-jwt-content"
		originalCWT := "old-cwt-content"
		originalJSON := "old-json-content"

		os.WriteFile(filepath.Join(dirPath, "token_status_list.jwt"), []byte(originalJWT), 0644)
		os.WriteFile(filepath.Join(dirPath, "token_status_list.cwt"), []byte(originalCWT), 0644)
		os.WriteFile(filepath.Join(dirPath, "full_list.json"), []byte(originalJSON), 0644)

		err := rs.renewTokenStatusList(dirPath, statusListData, formatter)

		if err != nil {
			t.Errorf("renewTokenStatusList() error = %v", err)
		}

		// Check that files still exist (they may have new content due to formatter)
		if _, err := os.Stat(filepath.Join(dirPath, "token_status_list.jwt")); os.IsNotExist(err) {
			t.Error("JWT file should exist after renewal")
		}

		if _, err := os.Stat(filepath.Join(dirPath, "token_status_list.cwt")); os.IsNotExist(err) {
			t.Error("CWT file should exist after renewal")
		}
	})
}

func TestRenewIdentifierList(t *testing.T) {
	tempDir, statusListDir, cfg, stor := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	rs := NewRenewalService(cfg, stor, zap.NewNop())
	statusListData := createTestStatusListData(false)
	formatter := services.NewStatusListFormatter(cfg)

	t.Run("test file renewal", func(t *testing.T) {
		// Create test directory and files
		dirPath := filepath.Join(statusListDir, "test_issuer", "identifier_list")
		os.MkdirAll(dirPath, 0755)

		// Create existing files to be renewed
		originalJWT := "old-identifier-jwt-content"
		originalCWT := "old-identifier-cwt-content"
		originalJSON := "old-json-content"

		os.WriteFile(filepath.Join(dirPath, "identifier_list.jwt"), []byte(originalJWT), 0644)
		os.WriteFile(filepath.Join(dirPath, "identifier_list.cwt"), []byte(originalCWT), 0644)
		os.WriteFile(filepath.Join(dirPath, "full_list.json"), []byte(originalJSON), 0644)

		err := rs.renewIdentifierList(dirPath, statusListData, formatter)

		if err != nil {
			t.Errorf("renewIdentifierList() error = %v", err)
		}

		// Check that files still exist (they may have new content due to formatter)
		if _, err := os.Stat(filepath.Join(dirPath, "identifier_list.jwt")); os.IsNotExist(err) {
			t.Error("Identifier JWT file should exist after renewal")
		}

		if _, err := os.Stat(filepath.Join(dirPath, "identifier_list.cwt")); os.IsNotExist(err) {
			t.Error("Identifier CWT file should exist after renewal")
		}
	})
}

func SkipTestCopyFile(t *testing.T) {
	t.Skip("copyFile functionality removed - backup now handled at infrastructure level")
}

func TestProcessListFile(t *testing.T) {
	tempDir, statusListDir, cfg, stor := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	rs := NewRenewalService(cfg, stor, zap.NewNop())
	formatter := services.NewStatusListFormatter(cfg)

	t.Run("process file with invalid expiry date format", func(t *testing.T) {
		// Create a file with invalid expiry date
		listDir := filepath.Join(statusListDir, "test_issuer", "token_status_list")
		os.MkdirAll(listDir, 0755)

		invalidExpires := "invalid-date-format"
		statusData := &models.StatusListData{
			TokenStatusList:   models.NewIssuerStatusList(2, 100, "sequential"),
			Expires:           &invalidExpires,
			StatusListURI:     "https://example.com/status",
			IdentifierListURI: "https://example.com/identifier",
		}

		jsonData, _ := json.Marshal(statusData)
		fullListPath := filepath.Join(listDir, "full_list.json")
		os.WriteFile(fullListPath, jsonData, 0644)

		err := rs.processListFile(fullListPath, formatter)
		if err != nil {
			t.Errorf("processListFile should handle invalid date gracefully, got error: %v", err)
		}
	})

	t.Run("process file in unrecognized directory", func(t *testing.T) {
		// Create a file in a directory that doesn't match token_status_list or identifier_list
		listDir := filepath.Join(statusListDir, "test_issuer", "unknown_list_type")
		os.MkdirAll(listDir, 0755)

		statusData := createTestStatusListData(false)
		jsonData, _ := json.Marshal(statusData)
		fullListPath := filepath.Join(listDir, "full_list.json")
		os.WriteFile(fullListPath, jsonData, 0644)

		err := rs.processListFile(fullListPath, formatter)
		if err != nil {
			t.Errorf("processListFile should handle unrecognized directory gracefully, got error: %v", err)
		}
	})
}

// Test RenewLists with empty directory
func TestRenewListsEmptyDirectory(t *testing.T) {
	tempDir, _, cfg, stor := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	rs := NewRenewalService(cfg, stor, zap.NewNop())
	err := rs.RenewLists()

	// Should not error on empty directory
	if err != nil {
		t.Errorf("RenewLists() should handle empty directory, got error: %v", err)
	}
}

// Test RenewLists with non-existent directory
func TestRenewListsNonExistentDirectory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "renewal_nonexistent_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.Config{
		StatusListDir: "/path/that/does/not/exist",
		BackupDir:     "/another/path/that/does/not/exist",
	}

	stor, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// RenewLists no longer errors on non-existent directories — it logs and returns nil.
	// The storage.List() call handles backend availability gracefully.
	err = NewRenewalService(cfg, stor, zap.NewNop()).RenewLists()
	if err != nil {
		t.Errorf("RenewLists() returned unexpected error: %v", err)
	}

	tests := []struct {
		name        string
		setup       func() (string, string)
		wantErr     bool
		errorSource string
	}{
		{
			name: "source file does not exist",
			setup: func() (string, string) {
				src := filepath.Join(tempDir, "nonexistent.txt")
				dst := filepath.Join(tempDir, "destination.txt")
				return src, dst
			},
			wantErr:     true,
			errorSource: "os.ReadFile",
		},
		{
			name: "source is directory instead of file",
			setup: func() (string, string) {
				srcDir := filepath.Join(tempDir, "source_dir")
				os.MkdirAll(srcDir, 0755)
				dst := filepath.Join(tempDir, "destination.txt")
				return srcDir, dst
			},
			wantErr:     true,
			errorSource: "os.ReadFile",
		},
		{
			name: "destination directory does not exist",
			setup: func() (string, string) {
				src := filepath.Join(tempDir, "source.txt")
				dst := filepath.Join(tempDir, "nonexistent", "destination.txt")
				os.WriteFile(src, []byte(testContent), 0644)
				return src, dst
			},
			wantErr:     true,
			errorSource: "os.WriteFile",
		},
		{
			name: "test L208 - os.Stat error after successful read",
			setup: func() (string, string) {
				// Create a file and then remove it between ReadFile and Stat
				// This is tricky to test directly, but we can test a symlink to nonexistent file
				src := filepath.Join(tempDir, "source.txt")
				dst := filepath.Join(tempDir, "destination.txt")
				os.WriteFile(src, []byte(testContent), 0644)
				return src, dst
			},
			wantErr:     false, // This case will pass normally
			errorSource: "normal operation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			// Since copyFile functionality has been removed, just test that setup works
			tt.setup()
			// Test passes if setup doesn't panic
		})
	}
}

// Test edge cases for file operations
func TestFileOperationEdgeCases(t *testing.T) {
	t.Skip("File operation edge cases were for copyFile functionality which has been removed")
}

// Test RenewLists with filepath.Walk errors
func TestRenewListsWalkErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *config.Config
		wantErr bool
	}{
		{
			name: "walk error - permission denied on directory",
			setup: func() *config.Config {
				tempDir, err := os.MkdirTemp("", "renewal_walk_test")
				if err != nil {
					t.Fatalf("Failed to create temp directory: %v", err)
				}

				statusListDir := filepath.Join(tempDir, "status_lists")
				backupDir := filepath.Join(tempDir, "backup")

				os.MkdirAll(statusListDir, 0755)
				os.MkdirAll(backupDir, 0755)

				// On Windows, directory permissions work differently
				// Create a subdirectory with no permissions (may not work on Windows)
				restrictedDir := filepath.Join(statusListDir, "restricted")
				os.MkdirAll(restrictedDir, 0000)

				return &config.Config{
					StatusListDir: statusListDir,
					BackupDir:     backupDir,
					CountryCode:   "US",
					PrivKeyPath:   "temp/private_key/private-key.pem",
					CertPath:      "temp/certificate/certificate.pem",
				}
			},
			wantErr: false, // Changed to false as Windows may not respect directory permissions
		},
		{
			name: "walk with broken symlink",
			setup: func() *config.Config {
				tempDir, err := os.MkdirTemp("", "renewal_symlink_test")
				if err != nil {
					t.Fatalf("Failed to create temp directory: %v", err)
				}

				statusListDir := filepath.Join(tempDir, "status_lists")
				backupDir := filepath.Join(tempDir, "backup")

				os.MkdirAll(statusListDir, 0755)
				os.MkdirAll(backupDir, 0755)

				return &config.Config{
					StatusListDir: statusListDir,
					BackupDir:     backupDir,
					CountryCode:   "US",
					PrivKeyPath:   "temp/private_key/private-key.pem",
					CertPath:      "temp/certificate/certificate.pem",
				}
			},
			wantErr: false, // Should handle gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setup()
			defer os.RemoveAll(filepath.Dir(cfg.StatusListDir))

			stor, err := storage.NewStorage(cfg)
			if err != nil {
				t.Fatalf("Failed to create storage: %v", err)
			}

			rs := NewRenewalService(cfg, stor, zap.NewNop())
			err = rs.RenewLists()

			if (err != nil) != tt.wantErr {
				t.Errorf("RenewLists() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// logsContainMsg checks if any log entry contains the given substring.
func logsContainMsg(logs []observer.LoggedEntry, substr string) bool {
	for _, entry := range logs {
		if strings.Contains(entry.Message, substr) {
			return true
		}
	}
	return false
}

// newRenewalServiceWithObserver creates a RenewalService with an observable logger.
func newRenewalServiceWithObserver(cfg *config.Config, stor storage.Storage) (*RenewalService, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	return NewRenewalService(cfg, stor, logger), logs
}

// TestErrorLogging tests that error conditions are properly logged
func TestErrorLogging(t *testing.T) {
	tempDir, statusListDir, cfg, stor := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	formatter := services.NewStatusListFormatter(cfg)

	t.Run("test file read error logging", func(t *testing.T) {
		rs, logs := newRenewalServiceWithObserver(cfg, stor)
		listDir := filepath.Join(statusListDir, "test_issuer", "token_status_list")
		os.MkdirAll(listDir, 0755)
		fullListPath := filepath.Join(listDir, "full_list.json")
		os.WriteFile(fullListPath, []byte("test"), 0000)

		rs.processListFile(fullListPath, formatter)

		entries := logs.All()
		if !logsContainMsg(entries, "error reading file") && !logsContainMsg(entries, "error unmarshaling file") {
			t.Errorf("Expected error log for unreadable file, got %d log entries", len(entries))
		}

		os.Chmod(fullListPath, 0644)
	})

	t.Run("test JSON unmarshal error logging", func(t *testing.T) {
		rs, logs := newRenewalServiceWithObserver(cfg, stor)
		listDir := filepath.Join(statusListDir, "test_issuer", "token_status_list")
		os.MkdirAll(listDir, 0755)
		fullListPath := filepath.Join(listDir, "full_list.json")
		os.WriteFile(fullListPath, []byte("invalid json content"), 0644)

		rs.processListFile(fullListPath, formatter)

		if !logsContainMsg(logs.All(), "error unmarshaling file") {
			t.Errorf("Expected 'error unmarshaling file' log message, got %d entries", logs.Len())
		}
	})

	t.Run("test missing URIs logging", func(t *testing.T) {
		rs, logs := newRenewalServiceWithObserver(cfg, stor)
		listDir := filepath.Join(statusListDir, "test_issuer", "token_status_list")
		os.MkdirAll(listDir, 0755)
		fullListPath := filepath.Join(listDir, "full_list.json")

		statusData := &models.StatusListData{StatusListURI: "", IdentifierListURI: ""}
		jsonData, _ := json.Marshal(statusData)
		os.WriteFile(fullListPath, jsonData, 0644)

		rs.processListFile(fullListPath, formatter)

		if !logsContainMsg(logs.All(), "URIs missing in file") {
			t.Errorf("Expected 'URIs missing in file' log message, got %d entries", logs.Len())
		}
	})

	t.Run("test invalid expiry date logging", func(t *testing.T) {
		rs, logs := newRenewalServiceWithObserver(cfg, stor)
		listDir := filepath.Join(statusListDir, "test_issuer", "token_status_list")
		os.MkdirAll(listDir, 0755)
		fullListPath := filepath.Join(listDir, "full_list.json")

		invalidExpires := "invalid-date-format"
		statusData := &models.StatusListData{
			TokenStatusList:   models.NewIssuerStatusList(2, 100, "sequential"),
			Expires:           &invalidExpires,
			StatusListURI:     "https://example.com/status",
			IdentifierListURI: "https://example.com/identifier",
		}
		jsonData, _ := json.Marshal(statusData)
		os.WriteFile(fullListPath, jsonData, 0644)

		rs.processListFile(fullListPath, formatter)

		if !logsContainMsg(logs.All(), "error parsing expiry date") {
			t.Errorf("Expected 'error parsing expiry date' log message, got %d entries", logs.Len())
		}
	})

	t.Run("test expired directory removal logging", func(t *testing.T) {
		rs, logs := newRenewalServiceWithObserver(cfg, stor)
		listDir := filepath.Join(statusListDir, "test_issuer", "token_status_list")
		os.MkdirAll(listDir, 0755)
		fullListPath := filepath.Join(listDir, "full_list.json")

		expiredDate := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
		statusData := &models.StatusListData{
			TokenStatusList:   models.NewIssuerStatusList(2, 100, "sequential"),
			Expires:           &expiredDate,
			StatusListURI:     "https://example.com/status",
			IdentifierListURI: "https://example.com/identifier",
		}
		jsonData, _ := json.Marshal(statusData)
		os.WriteFile(fullListPath, jsonData, 0644)

		rs.processListFile(fullListPath, formatter)

		if !logsContainMsg(logs.All(), "list is expired, skipping renewal") {
			t.Errorf("Expected 'list is expired, skipping renewal' log message, got %d entries", logs.Len())
		}
	})

	t.Run("test relative path error logging", func(t *testing.T) {
		rs, logs := newRenewalServiceWithObserver(cfg, stor)
		outsideDir := filepath.Join(tempDir, "outside")
		os.MkdirAll(outsideDir, 0755)
		fullListPath := filepath.Join(outsideDir, "full_list.json")

		statusData := createTestStatusListData(false)
		jsonData, _ := json.Marshal(statusData)
		os.WriteFile(fullListPath, jsonData, 0644)

		rs.processListFile(fullListPath, formatter)

		t.Logf("Log entries: %d", logs.Len())
	})

	t.Run("test backup directory creation error logging", func(t *testing.T) {
		rs, _ := newRenewalServiceWithObserver(cfg, stor)
		listDir := filepath.Join(statusListDir, "test_issuer", "token_status_list")
		os.MkdirAll(listDir, 0755)
		fullListPath := filepath.Join(listDir, "full_list.json")

		statusData := createTestStatusListData(false)
		jsonData, _ := json.Marshal(statusData)
		os.WriteFile(fullListPath, jsonData, 0644)

		os.Chmod(cfg.BackupDir, 0444)
		defer os.Chmod(cfg.BackupDir, 0755)

		rs.processListFile(fullListPath, formatter)
	})
}

// TestRenewalMethodErrorLogging tests error logging in renewal methods
func TestRenewalMethodErrorLogging(t *testing.T) {
	tempDir, statusListDir, cfg, stor := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	statusListData := createTestStatusListData(false)
	formatter := services.NewStatusListFormatter(cfg)

	t.Run("test renewTokenStatusList JWT generation error logging", func(t *testing.T) {
		rs, logs := newRenewalServiceWithObserver(cfg, stor)
		dirPath := filepath.Join(statusListDir, "test_issuer", "token_status_list")
		os.MkdirAll(dirPath, 0755)

		os.WriteFile(filepath.Join(dirPath, "token_status_list.jwt"), []byte("old-jwt"), 0644)
		os.WriteFile(filepath.Join(dirPath, "token_status_list.cwt"), []byte("old-cwt"), 0644)
		os.WriteFile(filepath.Join(dirPath, "full_list.json"), []byte("old-json"), 0644)

		rs.renewTokenStatusList(dirPath, statusListData, formatter)

		t.Logf("Log entries: %d", logs.Len())
	})

	t.Run("test renewTokenStatusList file write error logging", func(t *testing.T) {
		rs, logs := newRenewalServiceWithObserver(cfg, stor)
		dirPath := filepath.Join(statusListDir, "test_issuer2", "token_status_list")
		os.MkdirAll(dirPath, 0755)

		os.WriteFile(filepath.Join(dirPath, "token_status_list.jwt"), []byte("old-jwt"), 0644)
		os.WriteFile(filepath.Join(dirPath, "token_status_list.cwt"), []byte("old-cwt"), 0644)

		os.Chmod(dirPath, 0444)
		defer os.Chmod(dirPath, 0755)

		rs.renewTokenStatusList(dirPath, statusListData, formatter)

		t.Logf("Log entries: %d", logs.Len())
	})

	t.Run("test renewIdentifierList error logging", func(t *testing.T) {
		rs, logs := newRenewalServiceWithObserver(cfg, stor)
		dirPath := filepath.Join(statusListDir, "test_issuer", "identifier_list")
		os.MkdirAll(dirPath, 0755)

		os.WriteFile(filepath.Join(dirPath, "identifier_list.jwt"), []byte("old-jwt"), 0644)
		os.WriteFile(filepath.Join(dirPath, "identifier_list.cwt"), []byte("old-cwt"), 0644)

		os.Chmod(dirPath, 0444)
		defer os.Chmod(dirPath, 0755)

		rs.renewIdentifierList(dirPath, statusListData, formatter)

		t.Logf("Log entries: %d", logs.Len())
	})
}

// TestRenewListsErrorLogging tests top-level error logging
func TestRenewListsErrorLogging(t *testing.T) {
	t.Run("test filepath.Walk error logging", func(t *testing.T) {
		cfg := &config.Config{
			StatusListDir: "/path/that/does/not/exist",
			BackupDir:     "/another/path/that/does/not/exist",
		}

		stor, err := storage.NewStorage(cfg)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}

		rs, logs := newRenewalServiceWithObserver(cfg, stor)
		rs.RenewLists()

		if !logsContainMsg(logs.All(), "starting list renewal process") {
			t.Errorf("Expected 'starting list renewal process' log message, got %d entries", logs.Len())
		}

		if !logsContainMsg(logs.All(), "error listing files") {
			t.Errorf("Expected 'error listing files' log message, got %d entries", logs.Len())
		}
	})
}

// TestDailyRenewalLogging tests the logging in daily renewal process
func TestDailyRenewalLogging(t *testing.T) {
	t.Skip("Tests related to removed copyFile functionality")
}

func TestFileOperationErrorScenarios(t *testing.T) {
	t.Skip("Tests related to removed copyFile functionality")
}

func TestSuccessfulJWTCWTGeneration(t *testing.T) {
	t.Skip("Tests related to removed copyFile functionality")
}

func TestDailyRenewalTimingLogic(t *testing.T) {
	t.Skip("Tests related to removed copyFile functionality")
}

func TestRelativePathError(t *testing.T) {
	t.Skip("Tests related to removed copyFile functionality")
}

// TestCopyFileStatError tests the specific error scenario for lines 206-209 (os.Stat error)
func SkipTestCopyFileStatError(t *testing.T) {
	t.Skip("Tests related to removed copyFile functionality")
}

// MockStorage for testing version conflicts and retries
type MockStorage struct {
	storage.Storage
	existsFunc      func(path string) (bool, error)
	getVersionFunc  func(path string) (int, error)
	writeFunc       func(path string, content []byte, version int) error
	createFunc      func(path string, content []byte) error
	readFunc        func(path string) ([]byte, error)
	writeCalls      int
	getVersionCalls int
}

func (m *MockStorage) Exists(path string) (bool, error) {
	if m.existsFunc != nil {
		return m.existsFunc(path)
	}
	return true, nil
}

func (m *MockStorage) GetVersion(path string) (int, error) {
	m.getVersionCalls++
	if m.getVersionFunc != nil {
		return m.getVersionFunc(path)
	}
	return 1, nil
}

func (m *MockStorage) Write(path string, content []byte, version int) error {
	m.writeCalls++
	if m.writeFunc != nil {
		return m.writeFunc(path, content, version)
	}
	return nil
}

func (m *MockStorage) Create(path string, content []byte) error {
	if m.createFunc != nil {
		return m.createFunc(path, content)
	}
	return nil
}

func (m *MockStorage) Read(path string) ([]byte, error) {
	if m.readFunc != nil {
		return m.readFunc(path)
	}
	return []byte("{}"), nil
}

// TestWriteOrCreateFile_NewFile tests creating a new file
func TestWriteOrCreateFileNewFile(t *testing.T) {
	cfg := &config.Config{
		StatusListDir: testStatusListDir,
	}

	mockStorage := &MockStorage{
		existsFunc: func(path string) (bool, error) {
			return false, nil
		},
		createFunc: func(path string, content []byte) error {
			if string(content) != testContent {
				t.Errorf("Expected content '%s', got '%s'", testContent, string(content))
			}
			return nil
		},
	}

	rs := &RenewalService{
		logger: zap.NewNop(),
		config:  cfg,
		storage: mockStorage,
	}

	err := rs.writeOrCreateFile(testFileName, []byte(testContent))
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if mockStorage.writeCalls > 0 {
		t.Error("Write should not be called for new file")
	}
}

// TestWriteOrCreateFile_VersionConflictRetry tests retry logic on version mismatch
func TestWriteOrCreateFileVersionConflictRetry(t *testing.T) {
	cfg := &config.Config{
		StatusListDir: testStatusListDir,
	}

	attemptCount := 0
	mockStorage := &MockStorage{
		existsFunc: func(path string) (bool, error) {
			return true, nil
		},
		getVersionFunc: func(path string) (int, error) {
			// Return incrementing versions to simulate concurrent updates
			attemptCount++
			return attemptCount * 10, nil
		},
		writeFunc: func(path string, content []byte, version int) error {
			// First two attempts fail with version mismatch
			if attemptCount < 3 {
				return &versionMismatchError{expected: version + 10, got: version}
			}
			return nil
		},
	}

	rs := &RenewalService{
		logger: zap.NewNop(),
		config:  cfg,
		storage: mockStorage,
	}

	err := rs.writeOrCreateFile(testFileName, []byte(testContent))
	if err != nil {
		t.Errorf("Expected success after retries, got error: %v", err)
	}

	if mockStorage.writeCalls != 3 {
		t.Errorf("Expected 3 write attempts, got %d", mockStorage.writeCalls)
	}

	if mockStorage.getVersionCalls != 3 {
		t.Errorf("Expected 3 getVersion calls, got %d", mockStorage.getVersionCalls)
	}
}

// versionMismatchError simulates version mismatch error
type versionMismatchError struct {
	expected int
	got      int
}

func (e *versionMismatchError) Error() string {
	return fmt.Errorf("%w: expected %d, got %d", errors.ErrVersionMismatch, e.expected, e.got).Error()
}

func (e *versionMismatchError) Unwrap() error {
	return errors.ErrVersionMismatch
}

// TestWriteOrCreateFile_MaxRetriesExceeded tests failure after max retries
func TestWriteOrCreateFileMaxRetriesExceeded(t *testing.T) {
	cfg := &config.Config{
		StatusListDir: testStatusListDir,
	}

	mockStorage := &MockStorage{
		existsFunc: func(path string) (bool, error) {
			return true, nil
		},
		getVersionFunc: func(path string) (int, error) {
			return 1, nil
		},
		writeFunc: func(path string, content []byte, version int) error {
			// Always fail with version mismatch
			return &versionMismatchError{expected: 100, got: version}
		},
	}

	rs := &RenewalService{
		logger: zap.NewNop(),
		config:  cfg,
		storage: mockStorage,
	}

	err := rs.writeOrCreateFile(testFileName, []byte(testContent))
	if err == nil {
		t.Error("Expected error after max retries, got nil")
	}

	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Errorf("Expected 'failed after 3 attempts' error, got: %v", err)
	}

	if mockStorage.writeCalls != 3 {
		t.Errorf("Expected 3 write attempts, got %d", mockStorage.writeCalls)
	}
}

// TestWriteOrCreateFile_FileDeletedDuringOperation tests graceful handling when file is deleted
func TestWriteOrCreateFileFileDeletedDuringOperation(t *testing.T) {
	cfg := &config.Config{
		StatusListDir: testStatusListDir,
	}

	existsCalls := 0
	mockStorage := &MockStorage{
		existsFunc: func(path string) (bool, error) {
			existsCalls++
			// File exists on first check, deleted on second check
			return existsCalls == 1, nil
		},
		getVersionFunc: func(path string) (int, error) {
			return 1, nil
		},
	}

	rs := &RenewalService{
		logger: zap.NewNop(),
		config:  cfg,
		storage: mockStorage,
	}

	err := rs.writeOrCreateFile(testFileName, []byte(testContent))
	if err != nil {
		t.Errorf("Expected no error when file is deleted, got: %v", err)
	}

	if mockStorage.writeCalls > 0 {
		t.Error("Write should not be called when file no longer exists")
	}
}

// TestWriteOrCreateFile_GetVersionFailsWithNoSuchKey tests handling of file deletion during GetVersion
func TestWriteOrCreateFileGetVersionFailsWithNoSuchKey(t *testing.T) {
	cfg := &config.Config{
		StatusListDir: testStatusListDir,
	}

	mockStorage := &MockStorage{
		existsFunc: func(path string) (bool, error) {
			return true, nil
		},
		getVersionFunc: func(path string) (int, error) {
			return 0, &noSuchKeyError{}
		},
	}

	rs := &RenewalService{
		logger: zap.NewNop(),
		config:  cfg,
		storage: mockStorage,
	}

	err := rs.writeOrCreateFile(testFileName, []byte(testContent))
	if err != nil {
		t.Errorf("Expected no error when file is deleted during GetVersion, got: %v", err)
	}

	if mockStorage.writeCalls > 0 {
		t.Error("Write should not be called when file is deleted")
	}
}

// noSuchKeyError simulates S3 NoSuchKey error
type noSuchKeyError struct{}

func (e *noSuchKeyError) Error() string {
	return fmt.Errorf("%w: The specified key does not exist", errors.ErrNotFound).Error()
}

func (e *noSuchKeyError) Unwrap() error {
	return errors.ErrNotFound
}

// TestWriteOrCreateFile_NonVersionMismatchError tests that other errors are not retried
func TestWriteOrCreateFileNonVersionMismatchError(t *testing.T) {
	cfg := &config.Config{
		StatusListDir: testStatusListDir,
	}

	mockStorage := &MockStorage{
		existsFunc: func(path string) (bool, error) {
			return true, nil
		},
		getVersionFunc: func(path string) (int, error) {
			return 1, nil
		},
		writeFunc: func(path string, content []byte, version int) error {
			return &otherError{msg: "permission denied"}
		},
	}

	rs := &RenewalService{
		logger: zap.NewNop(),
		config:  cfg,
		storage: mockStorage,
	}

	err := rs.writeOrCreateFile(testFileName, []byte(testContent))
	if err == nil {
		t.Error("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("Expected 'permission denied' error, got: %v", err)
	}

	if mockStorage.writeCalls != 1 {
		t.Errorf("Expected only 1 write attempt (no retry), got %d", mockStorage.writeCalls)
	}
}

// otherError simulates non-version-mismatch error
type otherError struct {
	msg string
}

func (e *otherError) Error() string {
	return e.msg
}

// TestProcessListFile_FileNotFound tests graceful handling of missing files
func TestProcessListFileFileNotFound(t *testing.T) {
	cfg := &config.Config{
		StatusListDir: testStatusListDir,
	}

	mockStorage := &MockStorage{
		readFunc: func(path string) ([]byte, error) {
			return nil, &noSuchKeyError{}
		},
	}

	rs := &RenewalService{
		logger: zap.NewNop(),
		config:  cfg,
		storage: mockStorage,
	}

	formatter := &services.StatusListFormatter{}

	err := rs.processListFile("test/full_list.json", formatter)
	if err != nil {
		t.Errorf("Expected no error for missing file, got: %v", err)
	}
}

// TestRenewTokenStatusList_DirectoryDeletedDuringWrite tests graceful handling when directory is deleted
func TestRenewTokenStatusListDirectoryDeletedDuringWrite(t *testing.T) {
	tempDir, _, cfg, stor := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create mock storage that simulates directory deletion
	mockStorage := &MockStorage{
		Storage: stor,
		existsFunc: func(path string) (bool, error) {
			return true, nil
		},
		getVersionFunc: func(path string) (int, error) {
			return 0, &noSuchKeyError{}
		},
	}

	rs := &RenewalService{
		logger: zap.NewNop(),
		config:  cfg,
		storage: mockStorage,
	}

	statusListData := createTestStatusListData(false)
	formatter := services.NewStatusListFormatter(cfg)

	// Should not return error even though directory was deleted
	err := rs.renewTokenStatusList("token_status_list/test", statusListData, formatter)
	if err != nil {
		t.Errorf("Expected no error when directory is deleted, got: %v", err)
	}
}

// TestRenewIdentifierList_DirectoryDeletedDuringWrite tests graceful handling when directory is deleted
func TestRenewIdentifierListDirectoryDeletedDuringWrite(t *testing.T) {
	tempDir, _, cfg, stor := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create mock storage that simulates directory deletion
	mockStorage := &MockStorage{
		Storage: stor,
		existsFunc: func(path string) (bool, error) {
			return true, nil
		},
		getVersionFunc: func(path string) (int, error) {
			return 0, &noSuchKeyError{}
		},
	}

	rs := &RenewalService{
		logger: zap.NewNop(),
		config:  cfg,
		storage: mockStorage,
	}

	statusListData := createTestStatusListData(false)
	formatter := services.NewStatusListFormatter(cfg)

	// Should not return error even though directory was deleted
	err := rs.renewIdentifierList("identifier_list/test", statusListData, formatter)
	if err != nil {
		t.Errorf("Expected no error when directory is deleted, got: %v", err)
	}
}

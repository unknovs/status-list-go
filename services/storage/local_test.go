package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLocalStorageCreate tests the Create operation
func TestLocalStorageCreate(t *testing.T) {
	// Create temporary test directory
	tempDir := t.TempDir()

	storage := &LocalStorage{StatusListDir: tempDir}

	tests := []struct {
		name        string
		path        string
		content     []byte
		expectError bool
	}{
		{
			name:        "create new file successfully",
			path:        "test/file.json",
			content:     []byte(`{"test": "data"}`),
			expectError: false,
		},
		{
			name:        "create file with nested directories",
			path:        "deep/nested/path/file.txt",
			content:     []byte("nested content"),
			expectError: false,
		},
		{
			name:        "create empty file",
			path:        "empty.json",
			content:     []byte{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := storage.Create(tt.path, tt.content)

			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.expectError {
				// Verify file exists
				fullPath := filepath.Join(tempDir, tt.path)
				if _, err := os.Stat(fullPath); os.IsNotExist(err) {
					t.Errorf("File was not created: %s", fullPath)
				}

				// Verify content
				data, err := os.ReadFile(fullPath)
				if err != nil {
					t.Errorf("Failed to read created file: %v", err)
				}
				if string(data) != string(tt.content) {
					t.Errorf("Content mismatch. Expected %s, got %s", tt.content, data)
				}
			}
		})
	}
}

func TestLocalStorageCreateExistingFile(t *testing.T) {
	tempDir := t.TempDir()
	storage := &LocalStorage{StatusListDir: tempDir}

	path := "existing.json"
	content1 := []byte("first content")
	content2 := []byte("second content")

	// Create file first time
	err := storage.Create(path, content1)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Attempt to create same file again should fail
	err = storage.Create(path, content2)
	if err == nil {
		t.Error("Expected error when creating existing file, got nil")
	}

	// Verify original content is unchanged
	data, _ := os.ReadFile(filepath.Join(tempDir, path))
	if string(data) != string(content1) {
		t.Error("Original file content was modified")
	}
}

// TestLocalStorageRead tests the Read operation
func TestLocalStorageRead(t *testing.T) {
	tempDir := t.TempDir()
	storage := &LocalStorage{StatusListDir: tempDir}

	// Create test file
	testPath := "test/read.json"
	testContent := []byte(`{"status": "valid"}`)
	fullPath := filepath.Join(tempDir, testPath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	os.WriteFile(fullPath, testContent, 0644)

	t.Run("read existing file", func(t *testing.T) {
		data, err := storage.Read(testPath)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if string(data) != string(testContent) {
			t.Errorf("Content mismatch. Expected %s, got %s", testContent, data)
		}
	})

	t.Run("read non-existent file", func(t *testing.T) {
		_, err := storage.Read("nonexistent/file.json")
		if err == nil {
			t.Error("Expected error for non-existent file, got nil")
		}
	})
}

// TestLocalStorageWrite tests the Write operation with version checking
func TestLocalStorageWrite(t *testing.T) {
	tempDir := t.TempDir()
	storage := &LocalStorage{StatusListDir: tempDir}

	testPath := "test/write.json"
	initialContent := []byte("initial")

	t.Run("write to new file with version 1", func(t *testing.T) {
		err := storage.Write(testPath, initialContent, 1)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Verify content
		data, _ := os.ReadFile(filepath.Join(tempDir, testPath))
		if string(data) != string(initialContent) {
			t.Errorf("Content mismatch. Expected %s, got %s", initialContent, data)
		}

		// Verify version metadata file exists
		versionPath := filepath.Join(tempDir, testPath+".version")
		if _, err := os.Stat(versionPath); os.IsNotExist(err) {
			t.Error("Version metadata file was not created")
		}
	})

	t.Run("update existing file with correct version", func(t *testing.T) {
		newContent := []byte("updated content")
		err := storage.Write(testPath, newContent, 2)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		data, _ := os.ReadFile(filepath.Join(tempDir, testPath))
		if string(data) != string(newContent) {
			t.Errorf("Content mismatch. Expected %s, got %s", newContent, data)
		}
	})

	t.Run("write with incorrect version fails", func(t *testing.T) {
		err := storage.Write(testPath, []byte("wrong version"), 1)
		if err == nil {
			t.Error("Expected version mismatch error, got nil")
		}
	})
}

// TestLocalStorageExists tests the Exists operation
func TestLocalStorageExists(t *testing.T) {
	tempDir := t.TempDir()
	storage := &LocalStorage{StatusListDir: tempDir}

	// Create test file
	existingPath := "test/exists.json"
	fullPath := filepath.Join(tempDir, existingPath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	os.WriteFile(fullPath, []byte("test"), 0644)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "existing file returns true",
			path:     existingPath,
			expected: true,
		},
		{
			name:     "non-existent file returns false",
			path:     "nonexistent/file.json",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, err := storage.Exists(tt.path)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if exists != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, exists)
			}
		})
	}
}

// TestLocalStorageList tests the List operation
func TestLocalStorageList(t *testing.T) {
	tempDir := t.TempDir()
	storage := &LocalStorage{StatusListDir: tempDir}

	// Create test file structure
	files := []string{
		"token_status_list/DE/mdl/list1.json",
		"token_status_list/DE/mdl/list2.json",
		"token_status_list/DE/pid/list3.json",
		"token_status_list/LV/mdl/list4.json",
		"other/file.json",
	}

	for _, file := range files {
		fullPath := filepath.Join(tempDir, file)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, []byte("test"), 0644)
	}

	tests := []struct {
		name          string
		prefix        string
		expectedMin   int // Minimum number of files expected
		shouldFindAny bool
	}{
		{
			name:          "list all files with token_status_list prefix",
			prefix:        "token_status_list",
			expectedMin:   4,
			shouldFindAny: true,
		},
		{
			name:          "list files in specific country",
			prefix:        "token_status_list/DE",
			expectedMin:   3,
			shouldFindAny: true,
		},
		{
			name:          "list files for specific doctype",
			prefix:        "token_status_list/DE/mdl",
			expectedMin:   2,
			shouldFindAny: true,
		},
		{
			name:          "list with non-existent prefix",
			prefix:        "nonexistent",
			expectedMin:   0,
			shouldFindAny: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := storage.List(tt.prefix)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.shouldFindAny && len(results) < tt.expectedMin {
				t.Errorf("Expected at least %d files, got %d", tt.expectedMin, len(results))
			}

			if !tt.shouldFindAny && len(results) > 0 {
				t.Errorf("Expected no files, got %d", len(results))
			}

			// Verify all returned paths start with the prefix
			for _, path := range results {
				if len(tt.prefix) > 0 && len(path) >= len(tt.prefix) {
					// Check if path starts with prefix (accounting for path separators)
					if !pathStartsWith(path, tt.prefix) {
						t.Errorf("Path %s does not start with prefix %s", path, tt.prefix)
					}
				}
			}
		})
	}
}

// TestLocalStorageAtomicWrite tests atomic file writes (temp file + rename)
func TestLocalStorageAtomicWrite(t *testing.T) {
	tempDir := t.TempDir()
	storage := &LocalStorage{StatusListDir: tempDir}

	testPath := "test/atomic.json"
	content := []byte("atomic content")

	t.Run("atomic create leaves no temp files on success", func(t *testing.T) {
		err := storage.Create(testPath, content)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Check for temporary files
		tempFiles, _ := filepath.Glob(filepath.Join(tempDir, "test", "*.tmp"))
		if len(tempFiles) > 0 {
			t.Errorf("Found %d temporary files after successful create", len(tempFiles))
		}

		// Verify final file exists
		fullPath := filepath.Join(tempDir, testPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Error("Final file does not exist after atomic create")
		}
	})

	t.Run("atomic write leaves no temp files on success", func(t *testing.T) {
		updatedContent := []byte("updated atomic content")
		err := storage.Write(testPath, updatedContent, 2)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Check for temporary files
		tempFiles, _ := filepath.Glob(filepath.Join(tempDir, "test", "*.tmp"))
		if len(tempFiles) > 0 {
			t.Errorf("Found %d temporary files after successful write", len(tempFiles))
		}
	})
}

// Helper function to check if path starts with prefix
func pathStartsWith(path, prefix string) bool {
	// Normalize path separators
	path = filepath.ToSlash(path)
	prefix = filepath.ToSlash(prefix)

	if len(path) < len(prefix) {
		return false
	}

	return path[:len(prefix)] == prefix
}

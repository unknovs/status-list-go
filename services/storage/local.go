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

package storage

import (
	stdErrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/unknovs/status-list-go/errors"
)

const (
	VersionFileSuffix = ".version"
)

// LocalStorage implements the Storage interface using the local filesystem.
type LocalStorage struct {
	StatusListDir string
}

// NewLocalStorage creates a new local filesystem storage backend.
func NewLocalStorage(statusListDir string) (*LocalStorage, error) {
	if statusListDir == "" {
		return nil, stdErrors.New("STATUS_LIST_DIR is required for local storage")
	}

	return &LocalStorage{
		StatusListDir: statusListDir,
	}, nil
}

// Create creates a new file with the given content.
// Uses atomic write (temp file + rename) to prevent partial writes.
// Returns an error if the file already exists.
func (ls *LocalStorage) Create(path string, content []byte) error {
	fullPath := filepath.Join(ls.StatusListDir, path)

	// Check if file already exists
	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("%w: %s", errors.ErrAlreadyExists, path)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write atomically using temp file + rename
	if err := atomicWrite(fullPath, content); err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	// Initialize version metadata
	versionPath := fullPath + VersionFileSuffix
	if err := os.WriteFile(versionPath, []byte("1"), 0644); err != nil {
		return fmt.Errorf("failed to create version metadata: %w", err)
	}

	return nil
}

// Read retrieves the content of a file.
func (ls *LocalStorage) Read(path string) ([]byte, error) {
	fullPath := filepath.Join(ls.StatusListDir, path)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", errors.ErrNotFound, path)
		}

		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, nil
}

// Write updates an existing file with optimistic locking.
// The version parameter must match the current file version.
// Uses atomic write (temp file + rename) to prevent partial writes.
func (ls *LocalStorage) Write(path string, content []byte, version int) error {
	fullPath := filepath.Join(ls.StatusListDir, path)
	versionPath := fullPath + VersionFileSuffix

	// Check current version
	currentVersion, err := ls.GetVersion(path)
	if err != nil {
		// If version file doesn't exist, treat as version 0 (new file)
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read version: %w", err)
		}

		currentVersion = 0
	}

	// Validate version (optimistic locking)
	if version != currentVersion+1 {
		return fmt.Errorf("%w: expected %d, got %d", errors.ErrVersionMismatch, currentVersion+1, version)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write atomically using temp file + rename
	if err := atomicWrite(fullPath, content); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Update version metadata
	if err := os.WriteFile(versionPath, []byte(strconv.Itoa(version)), 0644); err != nil {
		return fmt.Errorf("failed to update version metadata: %w", err)
	}

	return nil
}

// Exists checks if a file exists at the given path.
func (ls *LocalStorage) Exists(path string) (bool, error) {
	fullPath := filepath.Join(ls.StatusListDir, path)

	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, fmt.Errorf("failed to check file existence: %w", err)
}

// List returns a list of file paths with the given prefix.
// Used by renewal process to discover status list files.
func (ls *LocalStorage) List(prefix string) ([]string, error) {
	var results []string

	// Walk the directory tree
	err := filepath.Walk(ls.StatusListDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and version metadata files
		if info.IsDir() || strings.HasSuffix(path, VersionFileSuffix) {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(ls.StatusListDir, path)
		if err != nil {
			return err
		}

		// Normalize path separators to forward slashes for consistency
		relPath = filepath.ToSlash(relPath)

		// Check if path starts with prefix
		if prefix == "" || strings.HasPrefix(relPath, prefix) {
			results = append(results, relPath)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	return results, nil
}

// GetVersion reads the current version of a file from its metadata.
func (ls *LocalStorage) GetVersion(path string) (int, error) {
	fullPath := filepath.Join(ls.StatusListDir, path)
	versionPath := fullPath + VersionFileSuffix

	data, err := os.ReadFile(versionPath)
	if err != nil {
		return 0, err
	}

	version, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid version format: %w", err)
	}

	return version, nil
}

// DeleteTree removes all files and directories under the given prefix.
func (ls *LocalStorage) DeleteTree(prefix string) error {
	if strings.TrimSpace(prefix) == "" {
		return fmt.Errorf("prefix is required for DeleteTree")
	}

	relPath := filepath.FromSlash(prefix)
	fullPath := filepath.Join(ls.StatusListDir, relPath)

	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("failed to inspect path: %w", err)
	}

	if err := os.RemoveAll(fullPath); err != nil {
		return fmt.Errorf("failed to remove path: %w", err)
	}

	return nil
}

// atomicWrite writes data to a file atomically using a temp file and rename.
// This prevents partial writes in case of failures.
func atomicWrite(path string, data []byte) error {
	// Create temp file in the same directory as the target file
	dir := filepath.Dir(path)

	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	tmpPath := tmpFile.Name()

	// Ensure temp file is removed on error
	defer func() {
		if tmpFile != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	// Write data to temp file
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}

	// Sync to ensure data is written to disk
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	// Close temp file before rename
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	tmpFile = nil // Prevent cleanup in defer

	// Rename temp file to final location (atomic on POSIX systems)
	if err := os.Rename(tmpPath, path); err != nil {
		// Cleanup on Windows if rename fails
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Verify LocalStorage implements Storage interface
var _ Storage = (*LocalStorage)(nil)

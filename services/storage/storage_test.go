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
	"testing"
)

// TestStorageInterfaceContract validates that implementations satisfy the Storage interface
func TestStorageInterfaceContract(t *testing.T) {
	t.Run("interface has required methods", func(t *testing.T) {
		// This test ensures the interface is defined correctly
		// Actual implementation tests will be in local_test.go and s3_test.go
		var _ Storage = (*mockStorage)(nil)
	})
}

// mockStorage is a minimal mock for interface verification
type mockStorage struct{}

func (m *mockStorage) Create(path string, content []byte) error {
	return nil
}

func (m *mockStorage) Read(path string) ([]byte, error) {
	return nil, nil
}

func (m *mockStorage) Write(path string, content []byte, version int) error {
	return nil
}

func (m *mockStorage) Exists(path string) (bool, error) {
	return false, nil
}

func (m *mockStorage) List(prefix string) ([]string, error) {
	return nil, nil
}

func (m *mockStorage) GetVersion(path string) (int, error) {
	return 0, nil
}

func (m *mockStorage) DeleteTree(prefix string) error {
	return nil
}

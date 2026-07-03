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

	"github.com/unknovs/status-list-go/config"
)

func TestNewStorage(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "local storage backend",
			cfg: &config.Config{
				BackendType:   "local",
				StatusListDir: "/tmp/test_storage",
			},
			expectError: false,
		},
		{
			name: "empty backend type defaults to local",
			cfg: &config.Config{
				BackendType:   "",
				StatusListDir: "/tmp/test_storage",
			},
			expectError: false,
		},
		{
			name: "invalid backend type",
			cfg: &config.Config{
				BackendType: "invalid",
			},
			expectError: true,
			errorMsg:    "unsupported storage backend",
		},
		{
			name: "s3 backend with valid config",
			cfg: &config.Config{
				BackendType:       "s3",
				S3Bucket:          "test-bucket",
				S3AccessKeyID:     "test-key",
				S3SecretAccessKey: "test-secret",
				S3Region:          "us-east-1",
			},
			expectError: true, // Will fail on connection validation in real test
			errorMsg:    "failed to validate S3 connection",
		},
		{
			name: "s3 backend missing bucket",
			cfg: &config.Config{
				BackendType:       "s3",
				S3AccessKeyID:     "test-key",
				S3SecretAccessKey: "test-secret",
			},
			expectError: true,
			errorMsg:    "S3_BUCKET is required",
		},
		{
			name: "s3 backend missing access key",
			cfg: &config.Config{
				BackendType:       "s3",
				S3Bucket:          "test-bucket",
				S3SecretAccessKey: "test-secret",
			},
			expectError: true,
			errorMsg:    "S3_ACCESS_KEY_ID is required",
		},
		{
			name: "s3 backend missing secret key",
			cfg: &config.Config{
				BackendType:   "s3",
				S3Bucket:      "test-bucket",
				S3AccessKeyID: "test-key",
			},
			expectError: true,
			errorMsg:    "S3_SECRET_ACCESS_KEY is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := NewStorage(tt.cfg)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if storage == nil {
					t.Error("Expected storage instance, got nil")
				}
			}
		})
	}
}

func TestNewStorageValidation(t *testing.T) {
	t.Run("local storage requires STATUS_LIST_DIR", func(t *testing.T) {
		cfg := &config.Config{
			BackendType:   "local",
			StatusListDir: "",
		}

		_, err := NewStorage(cfg)
		if err == nil {
			t.Error("Expected error for missing STATUS_LIST_DIR, got nil")
		}
	})
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

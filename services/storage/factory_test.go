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
			name: "s3 backend - will be tested in US2",
			cfg: &config.Config{
				BackendType:       "s3",
				S3Bucket:          "test-bucket",
				S3AccessKeyID:     "test-key",
				S3SecretAccessKey: "test-secret",
			},
			expectError: true, // Not yet implemented in US1
			errorMsg:    "S3 storage not yet implemented",
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

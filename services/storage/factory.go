package storage

import (
	"errors"
	"fmt"

	"github.com/unknovs/status-list-go/config"
)

// NewStorage creates a storage backend based on the configuration.
// Returns an error if the backend type is unsupported or configuration is invalid.
func NewStorage(cfg *config.Config) (Storage, error) {
	// Normalize empty backend type to "local"
	backendType := cfg.BackendType
	if backendType == "" {
		backendType = "local"
	}

	switch backendType {
	case "local":
		return newLocalStorage(cfg)
	case "s3":
		// S3 storage will be implemented later
		return nil, errors.New("S3 storage not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported storage backend: %s", backendType)
	}
}

// newLocalStorage creates a local filesystem storage backend
func newLocalStorage(cfg *config.Config) (Storage, error) {
	if cfg.StatusListDir == "" {
		return nil, errors.New("STATUS_LIST_DIR is required for local storage")
	}

	return NewLocalStorage(cfg.StatusListDir)
}

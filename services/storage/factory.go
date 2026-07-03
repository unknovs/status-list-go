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
		return newS3Storage(cfg)
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

// newS3Storage creates an S3 storage backend
func newS3Storage(cfg *config.Config) (Storage, error) {
	if cfg.S3Bucket == "" {
		return nil, errors.New("S3_BUCKET is required for S3 storage")
	}
	if cfg.S3AccessKeyID == "" {
		return nil, errors.New("S3_ACCESS_KEY_ID is required for S3 storage")
	}
	if cfg.S3SecretAccessKey == "" {
		return nil, errors.New("S3_SECRET_ACCESS_KEY is required for S3 storage")
	}

	s3Config := S3Config{
		Bucket:          cfg.S3Bucket,
		Region:          cfg.S3Region,
		AccessKeyID:     cfg.S3AccessKeyID,
		SecretAccessKey: cfg.S3SecretAccessKey,
		Endpoint:        cfg.S3Endpoint,
	}

	return NewS3Storage(s3Config)
}

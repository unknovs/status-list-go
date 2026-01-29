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

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/errors"
	"github.com/unknovs/status-list-go/services/storage"
)

const expectedErrorCodeMsg = "Expected error code %s, got %s"

func setupTestConfig(t *testing.T) (*config.Config, string, storage.Storage) {
	tempDir, err := os.MkdirTemp("", "serve_status_list_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cfg := &config.Config{
		APIKey:              "test-api-key",
		ServiceURL:          "http://localhost:8080/",
		TokenStatusListSize: 10000,
		StatusListDir:       tempDir,
		BackupDir:           filepath.Join(tempDir, "backup"),
		LogDir:              filepath.Join(tempDir, "logs"),
		PrivKeyPath:         "temp/private_key/decrypted_key.pem",
		CertPath:            "temp/certificate/PID-DS-0002.cert.der",
		CountryCode:         "LV",
		AllowedDoctypes:     map[string]bool{"PID": true, "MDL": true},
		BackendType:         "local",
	}

	stor, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	return cfg, tempDir, stor
}

func createTestStatusFile(t *testing.T, basePath, country, doctype, rand, fileName, content string) {
	fullPath := filepath.Join(basePath, "token_status_list", country, doctype, rand)
	err := os.MkdirAll(fullPath, 0755)
	if err != nil {
		t.Fatalf("Failed to create directory structure: %v", err)
	}

	filePath := filepath.Join(fullPath, fileName)
	err = os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
}

func TestServeStatusList(t *testing.T) {
	cfg, tempDir, stor := setupTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	// Create test files
	jwtContent := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdGF0dXNfbGlzdCI6ImVOcnN6ZyJ9.test-signature"
	cwtContent := "test-cwt-content"
	createTestStatusFile(t, tempDir, "LV", "PID", "test123", "token_status_list.jwt", jwtContent)
	createTestStatusFile(t, tempDir, "LV", "PID", "test123", "token_status_list.cwt", cwtContent)

	tests := []struct {
		name                string
		method              string
		path                string
		acceptHeader        string
		expectedStatus      int
		expectedContentType string
		expectedContent     string
		expectedError       errors.ErrorCode
	}{
		{
			name:                "Valid JWT request",
			method:              "GET",
			path:                "/token_status_list/LV/PID/test123",
			acceptHeader:        "application/statuslist+jwt",
			expectedStatus:      http.StatusOK,
			expectedContentType: "application/statuslist+jwt",
			expectedContent:     jwtContent,
		},
		{
			name:                "Valid CWT request",
			method:              "GET",
			path:                "/token_status_list/LV/PID/test123",
			acceptHeader:        "application/statuslist+cwt",
			expectedStatus:      http.StatusOK,
			expectedContentType: "application/statuslist+cwt",
			expectedContent:     cwtContent,
		},
		{
			name:                "Default to JWT when no Accept header",
			method:              "GET",
			path:                "/token_status_list/LV/PID/test123",
			acceptHeader:        "",
			expectedStatus:      http.StatusOK,
			expectedContentType: "application/statuslist+jwt",
			expectedContent:     jwtContent,
		},
		{
			name:                "Default to JWT with */* Accept header",
			method:              "GET",
			path:                "/token_status_list/LV/PID/test123",
			acceptHeader:        "*/*",
			expectedStatus:      http.StatusOK,
			expectedContentType: "application/statuslist+jwt",
			expectedContent:     jwtContent,
		},
		{
			name:           "Invalid method POST",
			method:         "POST",
			path:           "/token_status_list/LV/PID/test123",
			acceptHeader:   "application/statuslist+jwt",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  errors.ErrBadRequest,
		},
		{
			name:           "Invalid method PUT",
			method:         "PUT",
			path:           "/token_status_list/LV/PID/test123",
			acceptHeader:   "application/statuslist+jwt",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  errors.ErrBadRequest,
		},
		{
			name:           "Invalid path - missing country",
			method:         "GET",
			path:           "/token_status_list//PID/test123",
			acceptHeader:   "application/statuslist+jwt",
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidPath,
		},
		{
			name:           "Invalid path - missing doctype",
			method:         "GET",
			path:           "/token_status_list/LV//test123",
			acceptHeader:   "application/statuslist+jwt",
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidPath,
		},
		{
			name:           "Invalid path - missing rand",
			method:         "GET",
			path:           "/token_status_list/LV/PID/",
			acceptHeader:   "application/statuslist+jwt",
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidPath,
		},
		{
			name:           "Invalid path - too few parts",
			method:         "GET",
			path:           "/token_status_list/LV/PID",
			acceptHeader:   "application/statuslist+jwt",
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidPath,
		},
		{
			name:           "Invalid path - too many parts",
			method:         "GET",
			path:           "/token_status_list/LV/PID/test123/extra",
			acceptHeader:   "application/statuslist+jwt",
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidPath,
		},
		{
			name:           "Invalid country",
			method:         "GET",
			path:           "/token_status_list/US/PID/test123",
			acceptHeader:   "application/statuslist+jwt",
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidCountry,
		},
		{
			name:           "Invalid doctype",
			method:         "GET",
			path:           "/token_status_list/LV/INVALID/test123",
			acceptHeader:   "application/statuslist+jwt",
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidDoctype,
		},
		{
			name:           "Invalid Accept header",
			method:         "GET",
			path:           "/token_status_list/LV/PID/test123",
			acceptHeader:   "application/json",
			expectedStatus: http.StatusNotAcceptable,
			expectedError:  errors.ErrInvalidAccept,
		},
		{
			name:           "File not found",
			method:         "GET",
			path:           "/token_status_list/LV/PID/nonexistent",
			acceptHeader:   "application/statuslist+jwt",
			expectedStatus: http.StatusNotFound,
			expectedError:  errors.ErrListNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.acceptHeader != "" {
				req.Header.Set("Accept", tt.acceptHeader)
			}

			rr := httptest.NewRecorder()
			handler.ServeStatusList(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.expectedContentType != "" {
				contentType := rr.Header().Get("Content-Type")
				if contentType != tt.expectedContentType {
					t.Errorf("Expected Content-Type %s, got %s", tt.expectedContentType, contentType)
				}
			}

			if tt.expectedContent != "" {
				body := strings.TrimSpace(rr.Body.String())
				if body != tt.expectedContent {
					t.Errorf("Expected content %s, got %s", tt.expectedContent, body)
				}
			}

			if tt.expectedError != "" {
				var errorResponse errors.ErrorResponse
				err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
				if err != nil {
					t.Errorf("Failed to unmarshal error response: %v", err)
				}
				if errorResponse.Error.Code != tt.expectedError {
					t.Errorf(expectedErrorCodeMsg, tt.expectedError, errorResponse.Error.Code)
				}
			}

			// Verify security headers are set for successful requests
			if tt.expectedStatus == http.StatusOK {
				cacheControl := rr.Header().Get("Cache-Control")
				if cacheControl != "public, max-age=3600" {
					t.Errorf("Expected Cache-Control 'public, max-age=3600', got '%s'", cacheControl)
				}

				xContentTypeOptions := rr.Header().Get("X-Content-Type-Options")
				if xContentTypeOptions != "nosniff" {
					t.Errorf("Expected X-Content-Type-Options 'nosniff', got '%s'", xContentTypeOptions)
				}
			}
		})
	}
}

func TestServeStatusListWithBasePath(t *testing.T) {
	cfg, tempDir, stor := setupTestConfig(t)
	defer os.RemoveAll(tempDir)

	cfg.BasePath = config.NormalizeBasePath("/status-list/")
	handler := NewStatusListHandler(cfg, stor)

	jwtContent := "jwt-content"
	createTestStatusFile(t, tempDir, "LV", "PID", "test123", "token_status_list.jwt", jwtContent)

	req := httptest.NewRequest("GET", "/status-list/token_status_list/LV/PID/test123", nil)
	req.Header.Set("Accept", "application/statuslist+jwt")

	rr := httptest.NewRecorder()
	handler.ServeStatusList(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	if body := strings.TrimSpace(rr.Body.String()); body != jwtContent {
		t.Fatalf("expected body %s, got %s", jwtContent, body)
	}
}

func TestServeStatusListResponseHeaders(t *testing.T) {
	cfg, tempDir, stor := setupTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	// Create test file
	content := "test-jwt-content"
	createTestStatusFile(t, tempDir, "LV", "PID", "test123", "token_status_list.jwt", content)

	req := httptest.NewRequest("GET", "/token_status_list/LV/PID/test123", nil)
	req.Header.Set("Accept", "application/statuslist+jwt")

	rr := httptest.NewRecorder()
	handler.ServeStatusList(rr, req)

	// Test all expected headers
	headers := map[string]string{
		"Content-Type":           "application/statuslist+jwt",
		"Cache-Control":          "public, max-age=3600",
		"X-Content-Type-Options": "nosniff",
	}

	for headerName, expectedValue := range headers {
		actualValue := rr.Header().Get(headerName)
		if actualValue != expectedValue {
			t.Errorf("Expected header %s: %s, got: %s", headerName, expectedValue, actualValue)
		}
	}
}

func TestServeStatusListFileSystemErrors(t *testing.T) {
	cfg, tempDir, stor := setupTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	t.Run("file not found scenario", func(t *testing.T) {
		// Test the file not found path explicitly
		req := httptest.NewRequest("GET", "/token_status_list/LV/PID/nonexistent", nil)
		req.Header.Set("Accept", "application/statuslist+jwt")

		rr := httptest.NewRecorder()
		handler.ServeStatusList(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, rr.Code)
		}

		var errorResponse errors.ErrorResponse
		err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
		if err != nil {
			t.Errorf("Failed to unmarshal error response: %v", err)
		}
		if errorResponse.Error.Code != errors.ErrListNotFound {
			t.Errorf(expectedErrorCodeMsg, errors.ErrListNotFound, errorResponse.Error.Code)
		}
	})
}

func TestServeStatusListPathParsing(t *testing.T) {
	cfg, tempDir, stor := setupTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	// Test various edge cases in path parsing
	pathTests := []struct {
		name         string
		path         string
		expectedCode int
		expectedErr  errors.ErrorCode
	}{
		{
			name:         "Empty path after prefix",
			path:         "/token_status_list/",
			expectedCode: http.StatusBadRequest,
			expectedErr:  errors.ErrInvalidPath,
		},
		{
			name:         "Only slash after prefix",
			path:         "/token_status_list//",
			expectedCode: http.StatusBadRequest,
			expectedErr:  errors.ErrInvalidPath,
		},
		{
			name:         "Path with special characters",
			path:         "/token_status_list/LV/PID/test@123",
			expectedCode: http.StatusNotFound,
			expectedErr:  errors.ErrListNotFound,
		},
		{
			name:         "Path with URL encoding",
			path:         "/token_status_list/LV/PID/test%20123",
			expectedCode: http.StatusNotFound,
			expectedErr:  errors.ErrListNotFound,
		},
	}

	for _, tt := range pathTests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			req.Header.Set("Accept", "application/statuslist+jwt")

			rr := httptest.NewRecorder()
			handler.ServeStatusList(rr, req)

			if rr.Code != tt.expectedCode {
				t.Errorf("Expected status %d, got %d", tt.expectedCode, rr.Code)
			}

			if tt.expectedErr != "" {
				var errorResponse errors.ErrorResponse
				err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
				if err != nil {
					t.Errorf("Failed to unmarshal error response: %v", err)
				}
				if errorResponse.Error.Code != tt.expectedErr {
					t.Errorf(expectedErrorCodeMsg, tt.expectedErr, errorResponse.Error.Code)
				}
			}
		})
	}
}

func TestServeStatusListContentNegotiation(t *testing.T) {
	cfg, tempDir, stor := setupTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	// Create both JWT and CWT files
	jwtContent := "jwt-content"
	cwtContent := "cwt-content"
	createTestStatusFile(t, tempDir, "LV", "PID", "test123", "token_status_list.jwt", jwtContent)
	createTestStatusFile(t, tempDir, "LV", "PID", "test123", "token_status_list.cwt", cwtContent)

	contentTests := []struct {
		name            string
		acceptHeader    string
		expectedType    string
		expectedContent string
		expectedStatus  int
	}{
		{
			name:            "Explicit JWT",
			acceptHeader:    "application/statuslist+jwt",
			expectedType:    "application/statuslist+jwt",
			expectedContent: jwtContent,
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "Explicit CWT",
			acceptHeader:    "application/statuslist+cwt",
			expectedType:    "application/statuslist+cwt",
			expectedContent: cwtContent,
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "Empty Accept defaults to JWT",
			acceptHeader:    "",
			expectedType:    "application/statuslist+jwt",
			expectedContent: jwtContent,
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "Wildcard Accept defaults to JWT",
			acceptHeader:    "*/*",
			expectedType:    "application/statuslist+jwt",
			expectedContent: jwtContent,
			expectedStatus:  http.StatusOK,
		},
		{
			name:           "Unsupported Accept type",
			acceptHeader:   "text/plain",
			expectedStatus: http.StatusNotAcceptable,
		},
		{
			name:           "Multiple Accept types with unsupported",
			acceptHeader:   "text/html, application/json",
			expectedStatus: http.StatusNotAcceptable,
		},
	}

	for _, tt := range contentTests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/token_status_list/LV/PID/test123", nil)
			if tt.acceptHeader != "" {
				req.Header.Set("Accept", tt.acceptHeader)
			}

			rr := httptest.NewRecorder()
			handler.ServeStatusList(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.expectedType != "" {
				contentType := rr.Header().Get("Content-Type")
				if contentType != tt.expectedType {
					t.Errorf("Expected Content-Type %s, got %s", tt.expectedType, contentType)
				}
			}

			if tt.expectedContent != "" {
				body := strings.TrimSpace(rr.Body.String())
				if body != tt.expectedContent {
					t.Errorf("Expected content %s, got %s", tt.expectedContent, body)
				}
			}
		})
	}
}

func TestServeStatusListMissingFiles(t *testing.T) {
	cfg, tempDir, stor := setupTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	// Only create JWT file, not CWT
	jwtContent := "jwt-content"
	createTestStatusFile(t, tempDir, "LV", "PID", "test123", "token_status_list.jwt", jwtContent)

	t.Run("Request CWT when only JWT exists", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/token_status_list/LV/PID/test123", nil)
		req.Header.Set("Accept", "application/statuslist+cwt")

		rr := httptest.NewRecorder()
		handler.ServeStatusList(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, rr.Code)
		}

		var errorResponse errors.ErrorResponse
		err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
		if err != nil {
			t.Errorf("Failed to unmarshal error response: %v", err)
		}
		if errorResponse.Error.Code != errors.ErrListNotFound {
			t.Errorf(expectedErrorCodeMsg, errors.ErrListNotFound, errorResponse.Error.Code)
		}
	})

	t.Run("Request JWT when it exists", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/token_status_list/LV/PID/test123", nil)
		req.Header.Set("Accept", "application/statuslist+jwt")

		rr := httptest.NewRecorder()
		handler.ServeStatusList(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}

		body := strings.TrimSpace(rr.Body.String())
		if body != jwtContent {
			t.Errorf("Expected content %s, got %s", jwtContent, body)
		}
	})
}

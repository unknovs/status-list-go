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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/errors"
	"github.com/unknovs/status-list-go/models"
	"github.com/unknovs/status-list-go/services/storage"
)

func setupStatusListTestConfig(t *testing.T) (*config.Config, string, storage.Storage) {
	tempDir, err := os.MkdirTemp("", "status_list_handler_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cfg := &config.Config{
		APIKey:              "test-api-key",
		ServiceURL:          "http://localhost:8080/",
		TokenStatusListSize: 100, // Smaller for testing
		StatusListDir:       tempDir,
		BackupDir:           filepath.Join(tempDir, "backup"),
		LogDir:              filepath.Join(tempDir, "logs"),
		PrivKeyPath:         "temp/private_key/decrypted_key.pem",
		CertPath:            "temp/certificate/PID-DS-0002.cert.der",
		CountryCode:         "LV",
		BackendType:         "local",
		AllowedDoctypes:     map[string]bool{"PID": true, "MDL": true},
	}

	// Create required directories
	os.MkdirAll(cfg.StatusListDir, 0755)
	os.MkdirAll(cfg.BackupDir, 0755)
	os.MkdirAll(cfg.LogDir, 0755)

	stor, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	return cfg, tempDir, stor
}

func createTestStatusList(t *testing.T, cfg *config.Config, stor storage.Storage, country, doctype, randID string) {
	// Create a test status list
	statusList := models.NewIssuerStatusList(1, 100, "random")
	identifierList := make(map[string]int)

	// Add some test identifiers
	identifierList["test-id-1"] = 0
	identifierList["test-id-2"] = 1

	statusListData := &models.StatusListData{
		TokenStatusList:   statusList,
		IdentifierList:    identifierList,
		Country:           country,
		Doctype:           doctype,
		Rand:              randID,
		StatusListURI:     fmt.Sprintf("http://localhost:8080/token_status_list/%s/%s/%s", country, doctype, randID),
		IdentifierListURI: fmt.Sprintf("http://localhost:8080/identifier_list/%s/%s/%s", country, doctype, randID),
	}

	// Save as JSON using storage interface
	jsonData, err := json.Marshal(statusListData)
	if err != nil {
		t.Fatalf("Failed to marshal status list data: %v", err)
	}

	jsonFilePath := filepath.Join("token_status_list", country, doctype, randID, "full_list.json")
	err = stor.Create(jsonFilePath, jsonData)
	if err != nil {
		t.Fatalf("Failed to create status list file via storage: %v", err)
	}
}

func TestNewStatusListHandler(t *testing.T) {
	cfg, tempDir, stor := setupStatusListTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	if handler == nil {
		t.Fatal("NewStatusListHandler returned nil")
	}

	if handler.config != cfg {
		t.Error("Handler config not properly set")
	}

	if handler.listManager == nil {
		t.Error("Handler listManager not initialized")
	}
}

func TestTakeIndex(t *testing.T) {
	cfg, tempDir, stor := setupStatusListTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	tests := []struct {
		name           string
		method         string
		headers        map[string]string
		formData       map[string]string
		expectedStatus int
		expectedError  errors.ErrorCode
	}{
		{
			name:   "Valid request",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"doctype":     "PID",
				"country":     "LV",
				"expiry_date": time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid method GET",
			method:         "GET",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  errors.ErrBadRequest,
		},
		{
			name:           "Invalid method PUT",
			method:         "PUT",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  errors.ErrBadRequest,
		},
		{
			name:   "Missing API key",
			method: "POST",
			formData: map[string]string{
				"doctype":     "PID",
				"country":     "LV",
				"expiry_date": time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  errors.ErrInvalidAPIKey,
		},
		{
			name:   "Invalid API key",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "wrong-key",
			},
			formData: map[string]string{
				"doctype":     "PID",
				"country":     "LV",
				"expiry_date": time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  errors.ErrInvalidAPIKey,
		},
		{
			name:   "Invalid doctype",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"doctype":     "INVALID",
				"country":     "LV",
				"expiry_date": time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidDoctype,
		},
		{
			name:   "Invalid country",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"doctype":     "PID",
				"country":     "US",
				"expiry_date": time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidCountry,
		},
		{
			name:   "Invalid expiry date format",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"doctype":     "PID",
				"country":     "LV",
				"expiry_date": "invalid-date",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidExpiryDate,
		},
		{
			name:   "Past expiry date",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"doctype":     "PID",
				"country":     "LV",
				"expiry_date": "2020-01-01",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidExpiryDate,
		},
		{
			name:   "Missing doctype",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"country":     "LV",
				"expiry_date": time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidDoctype,
		},
		{
			name:   "Missing country",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"doctype":     "PID",
				"expiry_date": time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidCountry,
		},
		{
			name:   "Missing expiry date",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"doctype": "PID",
				"country": "LV",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidExpiryDate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create form data
			formData := url.Values{}
			for k, v := range tt.formData {
				formData.Set(k, v)
			}

			req := httptest.NewRequest(tt.method, "/token_status_list/take", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Set headers
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rr := httptest.NewRecorder()
			handler.TakeIndex(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.expectedError != "" {
				var errorResponse errors.ErrorResponse
				err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
				if err != nil {
					t.Errorf("Failed to unmarshal error response: %v", err)
				}
				if errorResponse.Error.Code != tt.expectedError {
					t.Errorf("Expected error code %s, got %s", tt.expectedError, errorResponse.Error.Code)
				}
			}

			// For successful requests, verify we get a JSON response
			if tt.expectedStatus == http.StatusOK {
				contentType := rr.Header().Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("Expected Content-Type application/json, got %s", contentType)
				}

				var response map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				if err != nil {
					t.Errorf("Failed to unmarshal success response: %v", err)
				}

				// Verify response contains expected fields
				if response["status_list"] == nil {
					t.Error("Response missing status_list field")
				}
				if response["identifier_list"] == nil {
					t.Error("Response missing identifier_list field")
				}

				// Verify nested structure
				if statusList, ok := response["status_list"].(map[string]interface{}); ok {
					if statusList["uri"] == nil {
						t.Error("Response status_list missing uri field")
					}
					if statusList["idx"] == nil {
						t.Error("Response status_list missing idx field")
					}
				} else {
					t.Error("Response status_list is not a proper object")
				}

				if identifierList, ok := response["identifier_list"].(map[string]interface{}); ok {
					if identifierList["uri"] == nil {
						t.Error("Response identifier_list missing uri field")
					}
					if identifierList["id"] == nil {
						t.Error("Response identifier_list missing id field")
					}
				} else {
					t.Error("Response identifier_list is not a proper object")
				}
			}
		})
	}
}

func TestGetIndex(t *testing.T) {
	cfg, tempDir, stor := setupStatusListTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	// Create a test status list
	createTestStatusList(t, cfg, stor, "LV", "PID", "test-rand")

	tests := []struct {
		name           string
		method         string
		queryParams    map[string]string
		expectedStatus int
		expectedError  errors.ErrorCode
		expectedBody   string
	}{
		{
			name:   "Valid request with idx",
			method: "GET",
			queryParams: map[string]string{
				"uri": "http://localhost:8080/token_status_list/LV/PID/test-rand",
				"idx": "0",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "0", // Default status
		},
		{
			name:   "Valid request with id",
			method: "GET",
			queryParams: map[string]string{
				"uri": "http://localhost:8080/token_status_list/LV/PID/test-rand",
				"id":  "5",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "0",
		},
		{
			name:           "Invalid method POST",
			method:         "POST",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  errors.ErrBadRequest,
		},
		{
			name:   "Missing URI",
			method: "GET",
			queryParams: map[string]string{
				"idx": "0",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrBadRequest,
		},
		{
			name:   "Missing index and id",
			method: "GET",
			queryParams: map[string]string{
				"uri": "",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrBadRequest,
		},
		{
			name:   "Invalid index format",
			method: "GET",
			queryParams: map[string]string{
				"uri": "http://localhost:8080/token_status_list/LV/PID/test",
				"idx": "invalid",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidIndex,
		},
		{
			name:   "Invalid URI format",
			method: "GET",
			queryParams: map[string]string{
				"uri": "invalid%uri",
				"idx": "0",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidURI,
		},
		{
			name:   "Non-existent URI",
			method: "GET",
			queryParams: map[string]string{
				"uri": "http://localhost:8080/token_status_list/LV/PID/nonexistent",
				"idx": "0",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrListNotFound,
		},
		{
			name:   "Index within range but high",
			method: "GET",
			queryParams: map[string]string{
				"uri": "http://localhost:8080/token_status_list/LV/PID/test-rand",
				"idx": "99",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build query string
			queryParams := url.Values{}
			for k, v := range tt.queryParams {
				queryParams.Set(k, v)
			}

			url := "/token_status_list/get"
			if len(queryParams) > 0 {
				url += "?" + queryParams.Encode()
			}

			req := httptest.NewRequest(tt.method, url, nil)
			rr := httptest.NewRecorder()
			handler.GetIndex(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.expectedError != "" {
				var errorResponse errors.ErrorResponse
				err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
				if err != nil {
					t.Errorf("Failed to unmarshal error response: %v", err)
				}
				if errorResponse.Error.Code != tt.expectedError {
					t.Errorf("Expected error code %s, got %s", tt.expectedError, errorResponse.Error.Code)
				}
			}

			if tt.expectedBody != "" {
				body := strings.TrimSpace(rr.Body.String())
				if body != tt.expectedBody {
					t.Errorf("Expected body %s, got %s", tt.expectedBody, body)
				}

				contentType := rr.Header().Get("Content-Type")
				if contentType != "text/plain" {
					t.Errorf("Expected Content-Type text/plain, got %s", contentType)
				}
			}
		})
	}
}

func TestSetIndex(t *testing.T) {
	cfg, tempDir, stor := setupStatusListTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	tests := []struct {
		name           string
		method         string
		headers        map[string]string
		formData       map[string]string
		expectedStatus int
		expectedError  errors.ErrorCode
		expectedBody   string
		setupRandID    string
	}{
		{
			name:   "Valid request with idx",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"uri":    "", // Will be set in test
				"idx":    "0",
				"status": "1",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "Status Changed",
			setupRandID:    "test-rand-123",
		},
		{
			name:   "Valid request with id",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"uri":    "", // Will be set in test
				"id":     "1",
				"status": "1",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "Status Changed",
			setupRandID:    "test-rand-456",
		},
		{
			name:           "Invalid method GET",
			method:         "GET",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  errors.ErrBadRequest,
		},
		{
			name:   "Missing API key",
			method: "POST",
			formData: map[string]string{
				"uri":    "",
				"idx":    "0",
				"status": "1",
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  errors.ErrUnauthorizedAccess,
		},
		{
			name:   "Invalid API key",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "wrong-key",
			},
			formData: map[string]string{
				"uri":    "",
				"idx":    "0",
				"status": "1",
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  errors.ErrUnauthorizedAccess,
		},
		{
			name:   "Missing URI",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"idx":    "0",
				"status": "1",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrBadRequest,
		},
		{
			name:   "Missing index and id",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"uri":    "",
				"status": "1",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrBadRequest,
		},
		{
			name:   "Missing status",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"uri": "",
				"idx": "0",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrBadRequest,
		},
		{
			name:   "Invalid index format",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"uri":    "http://localhost:8080/token_status_list/LV/PID/test",
				"idx":    "invalid",
				"status": "1",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidIndex,
		},
		{
			name:   "Invalid status format",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"uri":    "http://localhost:8080/token_status_list/LV/PID/test",
				"idx":    "0",
				"status": "invalid",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidStatus,
		},
		{
			name:   "Invalid status value (not 1)",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"uri":    "http://localhost:8080/token_status_list/LV/PID/test",
				"idx":    "0",
				"status": "2",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidStatus,
		},
		{
			name:   "Invalid URI format",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"uri":    "invalid-uri",
				"idx":    "0",
				"status": "1",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidURI,
		},
		{
			name:   "URI with insufficient path parts",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"uri":    "http://localhost:8080/short",
				"idx":    "0",
				"status": "1",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidURI,
		},
		{
			name:   "URI with invalid country",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"uri":    "http://localhost:8080/token_status_list/US/PID/test",
				"idx":    "0",
				"status": "1",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidCountry,
		},
		{
			name:   "URI with invalid doctype",
			method: "POST",
			headers: map[string]string{
				"X-Api-Key": "test-api-key",
			},
			formData: map[string]string{
				"uri":    "http://localhost:8080/token_status_list/LV/INVALID/test",
				"idx":    "0",
				"status": "1",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidDoctype,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test data for valid requests
			if tt.setupRandID != "" {
				createTestStatusList(t, cfg, stor, "LV", "PID", tt.setupRandID)
				testURI := fmt.Sprintf("http://localhost:8080/token_status_list/LV/PID/%s", tt.setupRandID)
				tt.formData["uri"] = testURI
			}

			// Create form data
			formData := url.Values{}
			for k, v := range tt.formData {
				formData.Set(k, v)
			}

			req := httptest.NewRequest(tt.method, "/token_status_list/set", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Set headers
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rr := httptest.NewRecorder()
			handler.SetIndex(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.expectedError != "" {
				var errorResponse errors.ErrorResponse
				err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
				if err != nil {
					t.Errorf("Failed to unmarshal error response: %v", err)
				}
				if errorResponse.Error.Code != tt.expectedError {
					t.Errorf("Expected error code %s, got %s", tt.expectedError, errorResponse.Error.Code)
				}
			}

			if tt.expectedBody != "" {
				body := strings.TrimSpace(rr.Body.String())
				if !strings.Contains(body, tt.expectedBody) {
					t.Errorf("Expected body to contain %s, got %s", tt.expectedBody, body)
				}

				contentType := rr.Header().Get("Content-Type")
				if contentType != "text/plain" {
					t.Errorf("Expected Content-Type text/plain, got %s", contentType)
				}
			}
		})
	}
}

func TestValidateExpiryDate(t *testing.T) {
	cfg, tempDir, stor := setupStatusListTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	tests := []struct {
		name        string
		expiryDate  string
		expectError bool
	}{
		{
			name:        "Valid future date",
			expiryDate:  time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
			expectError: false,
		},
		{
			name:        "Valid tomorrow date",
			expiryDate:  time.Now().AddDate(0, 0, 1).Format("2006-01-02"),
			expectError: false,
		},
		{
			name:        "Invalid format",
			expiryDate:  "2024/12/31",
			expectError: true,
		},
		{
			name:        "Invalid format - no dashes",
			expiryDate:  "20241231",
			expectError: true,
		},
		{
			name:        "Invalid format - wrong order",
			expiryDate:  "31-12-2024",
			expectError: true,
		},
		{
			name:        "Past date",
			expiryDate:  "2020-01-01",
			expectError: true,
		},
		{
			name:        "Today's date (should be valid as it's not past)",
			expiryDate:  time.Now().Format("2006-01-02"),
			expectError: false,
		},
		{
			name:        "Empty string",
			expiryDate:  "",
			expectError: true,
		},
		{
			name:        "Invalid month",
			expiryDate:  "2025-13-01",
			expectError: true,
		},
		{
			name:        "Invalid day",
			expiryDate:  "2025-01-32",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateExpiryDate(tt.expiryDate)

			if tt.expectError && err == nil {
				t.Errorf("Expected error for expiry date %s, but got nil", tt.expiryDate)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for expiry date %s, but got: %v", tt.expiryDate, err)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	cfg, tempDir, stor := setupStatusListTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	tests := []struct {
		name           string
		statusCode     int
		data           interface{}
		expectedStatus int
	}{
		{
			name:           "Valid JSON response",
			statusCode:     http.StatusOK,
			data:           map[string]string{"test": "value"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Error status code",
			statusCode:     http.StatusBadRequest,
			data:           map[string]string{"error": "test error"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Complex data structure",
			statusCode:     http.StatusCreated,
			data:           map[string]interface{}{"number": 42, "bool": true, "array": []string{"a", "b"}},
			expectedStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			handler.writeJSON(rr, tt.statusCode, tt.data)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			contentType := rr.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", contentType)
			}

			// Verify the response can be unmarshaled back to the same structure
			var response interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to unmarshal response: %v", err)
			}
		})
	}
}

func TestTakeIndexFormParsingError(t *testing.T) {
	cfg, tempDir, stor := setupStatusListTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	// Create a request with invalid form data (malformed Content-Type)
	req := httptest.NewRequest("POST", "/token_status_list/take", strings.NewReader("invalid%form%data"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Api-Key", "test-api-key")

	rr := httptest.NewRecorder()
	handler.TakeIndex(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var errorResponse errors.ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	if err != nil {
		t.Errorf("Failed to unmarshal error response: %v", err)
	}
	if errorResponse.Error.Code != errors.ErrParseForm {
		t.Errorf("Expected error code %s, got %s", errors.ErrParseForm, errorResponse.Error.Code)
	}
}

func TestSetIndexFormParsingError(t *testing.T) {
	cfg, tempDir, stor := setupStatusListTestConfig(t)
	defer os.RemoveAll(tempDir)

	handler := NewStatusListHandler(cfg, stor)

	// Create a request with invalid form data
	req := httptest.NewRequest("POST", "/token_status_list/set", strings.NewReader("invalid%form%data"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Api-Key", "test-api-key")

	rr := httptest.NewRecorder()
	handler.SetIndex(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var errorResponse errors.ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	if err != nil {
		t.Errorf("Failed to unmarshal error response: %v", err)
	}
	if errorResponse.Error.Code != errors.ErrParseForm {
		t.Errorf("Expected error code %s, got %s", errors.ErrParseForm, errorResponse.Error.Code)
	}
}

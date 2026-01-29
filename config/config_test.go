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

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "environment variable set",
			key:          "TEST_ENV_VAR",
			defaultValue: "default",
			envValue:     "custom_value",
			expected:     "custom_value",
		},
		{
			name:         "environment variable not set",
			key:          "NONEXISTENT_ENV_VAR",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
		{
			name:         "environment variable empty",
			key:          "EMPTY_ENV_VAR",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
		{
			name:         "default value empty",
			key:          "TEST_ENV_VAR_2",
			defaultValue: "",
			envValue:     "value",
			expected:     "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up environment variable
			defer os.Unsetenv(tt.key)

			// Set environment variable if needed
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
			}

			result := getEnv(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getEnv(%s, %s) = %s, expected %s", tt.key, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		defaultValue bool
		expected     bool
	}{
		{"truthy true", "true", false, true},
		{"truthy one", "1", false, true},
		{"truthy yes", "YES", false, true},
		{"falsy false", "false", true, false},
		{"falsy zero", "0", true, false},
		{"invalid fallback", "maybe", true, true},
		{"empty fallback", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const key = "TEST_BOOL_ENV"
			if tt.value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, tt.value)
			}
			t.Cleanup(func() { os.Unsetenv(key) })

			result := getEnvBool(key, tt.defaultValue)
			if result != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		defaultValue int
		expected     int
	}{
		{"valid number", "5", 1, 5},
		{"negative number", "-1", 2, -1},
		{"invalid fallback", "abc", 3, 3},
		{"empty fallback", "", 4, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const key = "TEST_INT_ENV"
			if tt.value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, tt.value)
			}
			t.Cleanup(func() { os.Unsetenv(key) })

			result := getEnvInt(key, tt.defaultValue)
			if result != tt.expected {
				t.Fatalf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestNormalizeBasePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty", input: "", expected: ""},
		{name: "root", input: "/", expected: ""},
		{name: "no leading slash", input: "api", expected: "/api"},
		{name: "leading slash", input: "/api", expected: "/api"},
		{name: "trailing slash", input: "/api/", expected: "/api"},
		{name: "multiple trailing", input: "/api///", expected: "/api"},
		{name: "internal path", input: "api/v1", expected: "/api/v1"},
		{name: "whitespace", input: "  /api/v1/  ", expected: "/api/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeBasePath(tt.input)
			if result != tt.expected {
				t.Fatalf("NormalizeBasePath(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetEnvArray(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envValue string
		expected map[string]bool
	}{
		{
			name:     "single value",
			key:      "TEST_ARRAY_VAR",
			envValue: "value1",
			expected: map[string]bool{"value1": true},
		},
		{
			name:     "multiple values",
			key:      "TEST_ARRAY_VAR",
			envValue: "value1,value2,value3",
			expected: map[string]bool{"value1": true, "value2": true, "value3": true},
		},
		{
			name:     "values with spaces",
			key:      "TEST_ARRAY_VAR",
			envValue: " value1 , value2 , value3 ",
			expected: map[string]bool{"value1": true, "value2": true, "value3": true},
		},
		{
			name:     "empty value",
			key:      "TEST_ARRAY_VAR",
			envValue: "",
			expected: map[string]bool{},
		},
		{
			name:     "environment variable not set",
			key:      "NONEXISTENT_ARRAY_VAR",
			envValue: "",
			expected: map[string]bool{},
		},
		{
			name:     "values with empty entries",
			key:      "TEST_ARRAY_VAR",
			envValue: "value1,,value2,",
			expected: map[string]bool{"value1": true, "value2": true},
		},
		{
			name:     "only commas",
			key:      "TEST_ARRAY_VAR",
			envValue: ",,,",
			expected: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up environment variable
			defer os.Unsetenv(tt.key)

			// Set environment variable if needed
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
			}

			result := getEnvArray(tt.key)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("getEnvArray(%s) = %v, expected %v", tt.key, result, tt.expected)
			}
		})
	}
}

func TestGetAllowedDoctypes(t *testing.T) {
	// Clean up environment variable
	defer os.Unsetenv("ALLOWED_DOCTYPES")

	t.Run("default doctypes when env var not set", func(t *testing.T) {
		result := getAllowedDoctypes()

		expectedDoctypes := map[string]bool{
			"eu.europa.ec.eudi.ehic.1":    true,
			"eu.europa.ec.eudi.hiid.1":    true,
			"eu.europa.ec.eudi.pid.1":     true,
			"org.iso.18013.5.1.mDL":       true,
			"urn:eudi:pid:1":              true,
			"urn:eu.europa.ec.eudi:pid:1": true,
		}

		if !reflect.DeepEqual(result, expectedDoctypes) {
			t.Errorf("getAllowedDoctypes() = %v, expected %v", result, expectedDoctypes)
		}
	})

	t.Run("custom doctypes from environment variable", func(t *testing.T) {
		os.Setenv("ALLOWED_DOCTYPES", "custom1,custom2,custom3")
		defer os.Unsetenv("ALLOWED_DOCTYPES")

		result := getAllowedDoctypes()

		expectedDoctypes := map[string]bool{
			"custom1": true,
			"custom2": true,
			"custom3": true,
		}

		if !reflect.DeepEqual(result, expectedDoctypes) {
			t.Errorf("getAllowedDoctypes() = %v, expected %v", result, expectedDoctypes)
		}
	})

	t.Run("empty environment variable uses defaults", func(t *testing.T) {
		os.Setenv("ALLOWED_DOCTYPES", "")
		defer os.Unsetenv("ALLOWED_DOCTYPES")

		result := getAllowedDoctypes()

		expectedDoctypes := map[string]bool{
			"eu.europa.ec.eudi.ehic.1":    true,
			"eu.europa.ec.eudi.hiid.1":    true,
			"eu.europa.ec.eudi.pid.1":     true,
			"org.iso.18013.5.1.mDL":       true,
			"urn:eudi:pid:1":              true,
			"urn:eu.europa.ec.eudi:pid:1": true,
		}

		if !reflect.DeepEqual(result, expectedDoctypes) {
			t.Errorf("getAllowedDoctypes() = %v, expected %v", result, expectedDoctypes)
		}
	})
}

func TestEnsureDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("create new directory", func(t *testing.T) {
		testDir := filepath.Join(tempDir, "new_directory")

		err := ensureDir(testDir)
		if err != nil {
			t.Errorf("ensureDir() failed: %v", err)
		}

		// Check if directory was created
		if _, err := os.Stat(testDir); os.IsNotExist(err) {
			t.Error("Directory was not created")
		}
	})

	t.Run("create nested directories", func(t *testing.T) {
		testDir := filepath.Join(tempDir, "level1", "level2", "level3")

		err := ensureDir(testDir)
		if err != nil {
			t.Errorf("ensureDir() failed: %v", err)
		}

		// Check if nested directory was created
		if _, err := os.Stat(testDir); os.IsNotExist(err) {
			t.Error("Nested directory was not created")
		}
	})

	t.Run("directory already exists", func(t *testing.T) {
		testDir := filepath.Join(tempDir, "existing_directory")

		// Create directory first
		err := os.MkdirAll(testDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}

		// Should not fail when directory already exists
		err = ensureDir(testDir)
		if err != nil {
			t.Errorf("ensureDir() failed on existing directory: %v", err)
		}
	})
}

func TestConfigValidateDoctype(t *testing.T) {
	config := &Config{
		AllowedDoctypes: map[string]bool{
			"PID": true,
			"MDL": true,
			"DL":  true,
		},
	}

	tests := []struct {
		name     string
		doctype  string
		expected bool
	}{
		{
			name:     "valid doctype",
			doctype:  "PID",
			expected: true,
		},
		{
			name:     "another valid doctype",
			doctype:  "MDL",
			expected: true,
		},
		{
			name:     "invalid doctype",
			doctype:  "INVALID",
			expected: false,
		},
		{
			name:     "empty doctype",
			doctype:  "",
			expected: false,
		},
		{
			name:     "case sensitive - lowercase",
			doctype:  "pid",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ValidateDoctype(tt.doctype)
			if result != tt.expected {
				t.Errorf("ValidateDoctype(%s) = %v, expected %v", tt.doctype, result, tt.expected)
			}
		})
	}
}

func TestConfigValidateCountry(t *testing.T) {
	config := &Config{
		CountryCode: "LV",
	}

	tests := []struct {
		name     string
		country  string
		expected bool
	}{
		{
			name:     "valid country",
			country:  "LV",
			expected: true,
		},
		{
			name:     "invalid country",
			country:  "US",
			expected: false,
		},
		{
			name:     "empty country",
			country:  "",
			expected: false,
		},
		{
			name:     "case sensitive - lowercase",
			country:  "lv",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ValidateCountry(tt.country)
			if result != tt.expected {
				t.Errorf("ValidateCountry(%s) = %v, expected %v", tt.country, result, tt.expected)
			}
		})
	}
}

func TestConfigGetCertificatePaths(t *testing.T) {
	config := &Config{
		PrivKeyPath: "/path/to/private.key",
		CertPath:    "/path/to/certificate.crt",
	}

	privKeyPath, certPath := config.GetCertificatePaths()

	if privKeyPath != config.PrivKeyPath {
		t.Errorf("GetCertificatePaths() privKeyPath = %s, expected %s", privKeyPath, config.PrivKeyPath)
	}

	if certPath != config.CertPath {
		t.Errorf("GetCertificatePaths() certPath = %s, expected %s", certPath, config.CertPath)
	}
}

func TestLoad(t *testing.T) {
	// Save original environment variables
	originalEnvVars := make(map[string]string)
	envVars := []string{
		"API_KEY", "SERVICE_URL", "STATUS_LIST_DIR", "BACKUP_DIR", "LOG_DIR",
		"PRIVATE_KEY_PATH", "CERTIFICATE_PATH", "COUNTRY_CODE", "ALLOWED_DOCTYPES",
		"STATUS_LIST_CLEANUP_ENABLED", "STATUS_LIST_CLEANUP_HOUR", "STATUS_LIST_CLEANUP_MINUTE",
		"STATUS_LIST_RENEWAL_ENABLED", "STATUS_LIST_RENEWAL_HOUR", "STATUS_LIST_RENEWAL_MINUTE",
	}

	for _, envVar := range envVars {
		originalEnvVars[envVar] = os.Getenv(envVar)
		os.Unsetenv(envVar)
	}

	// Restore environment variables after test
	defer func() {
		for _, envVar := range envVars {
			if originalValue, exists := originalEnvVars[envVar]; exists && originalValue != "" {
				os.Setenv(envVar, originalValue)
			} else {
				os.Unsetenv(envVar)
			}
		}
	}()

	t.Run("load with default values", func(t *testing.T) {
		// Create a temporary directory for the test
		tempDir, err := os.MkdirTemp("", "config_load_test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Set temp directories for paths that will be created
		os.Setenv("STATUS_LIST_DIR", filepath.Join(tempDir, "status_lists"))
		os.Setenv("BACKUP_DIR", filepath.Join(tempDir, "backup"))
		os.Setenv("LOG_DIR", filepath.Join(tempDir, "logs"))

		config, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		// Test default values
		if config.APIKey != "test" {
			t.Errorf("Expected APIKey 'test', got '%s'", config.APIKey)
		}

		if config.ServiceURL != "http://localhost:8080/" {
			t.Errorf("Expected ServiceURL 'http://localhost:8080/', got '%s'", config.ServiceURL)
		}

		if config.TokenStatusListSize != 10000 {
			t.Errorf("Expected TokenStatusListSize 10000, got %d", config.TokenStatusListSize)
		}

		if config.PrivKeyPath != "temp/private_key/decrypted_key.pem" {
			t.Errorf("Expected PrivKeyPath 'temp/private_key/decrypted_key.pem', got '%s'", config.PrivKeyPath)
		}

		if config.CertPath != "temp/certificate/PID-DS-0002.cert.der" {
			t.Errorf("Expected CertPath 'temp/certificate/PID-DS-0002.cert.der', got '%s'", config.CertPath)
		}

		if config.CountryCode != "LV" {
			t.Errorf("Expected CountryCode 'LV', got '%s'", config.CountryCode)
		}

		if !config.CleanupEnabled {
			t.Error("Expected cleanup to be enabled by default")
		}

		if config.CleanupHour != 4 {
			t.Errorf("Expected CleanupHour 4, got %d", config.CleanupHour)
		}

		if config.CleanupMinute != 0 {
			t.Errorf("Expected CleanupMinute 0, got %d", config.CleanupMinute)
		}

		if !config.RenewalEnabled {
			t.Error("Expected renewal to be enabled by default")
		}

		if config.RenewalHour != 12 {
			t.Errorf("Expected RenewalHour 12, got %d", config.RenewalHour)
		}

		if config.RenewalMinute != 0 {
			t.Errorf("Expected RenewalMinute 0, got %d", config.RenewalMinute)
		}

		// Test that default doctypes are loaded
		expectedDoctypes := map[string]bool{
			"eu.europa.ec.eudi.ehic.1":    true,
			"eu.europa.ec.eudi.hiid.1":    true,
			"eu.europa.ec.eudi.pid.1":     true,
			"org.iso.18013.5.1.mDL":       true,
			"urn:eudi:pid:1":              true,
			"urn:eu.europa.ec.eudi:pid:1": true,
		}

		if !reflect.DeepEqual(config.AllowedDoctypes, expectedDoctypes) {
			t.Errorf("Expected default doctypes, got %v", config.AllowedDoctypes)
		}

		// Test that directories were created
		dirs := []string{config.StatusListDir, config.BackupDir, config.LogDir}
		for _, dir := range dirs {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				t.Errorf("Directory %s was not created", dir)
			}
		}
	})

	t.Run("load with custom environment variables", func(t *testing.T) {
		// Create a temporary directory for the test
		tempDir, err := os.MkdirTemp("", "config_load_custom_test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Set custom environment variables
		os.Setenv("API_KEY", "custom_api_key")
		os.Setenv("SERVICE_URL", "https://example.com:9000/")
		os.Setenv("BASE_PATH", "status-list/")
		os.Setenv("STATUS_LIST_DIR", filepath.Join(tempDir, "custom_status_lists"))
		os.Setenv("BACKUP_DIR", filepath.Join(tempDir, "custom_backup"))
		os.Setenv("LOG_DIR", filepath.Join(tempDir, "custom_logs"))
		os.Setenv("PRIVATE_KEY_PATH", "/custom/path/to/private.key")
		os.Setenv("CERTIFICATE_PATH", "/custom/path/to/certificate.crt")
		os.Setenv("COUNTRY_CODE", "EE")
		os.Setenv("ALLOWED_DOCTYPES", "CUSTOM1,CUSTOM2,CUSTOM3")
		os.Setenv("STATUS_LIST_CLEANUP_ENABLED", "false")
		os.Setenv("STATUS_LIST_CLEANUP_HOUR", "5")
		os.Setenv("STATUS_LIST_CLEANUP_MINUTE", "30")
		os.Setenv("STATUS_LIST_RENEWAL_ENABLED", "false")
		os.Setenv("STATUS_LIST_RENEWAL_HOUR", "14")
		os.Setenv("STATUS_LIST_RENEWAL_MINUTE", "45")

		config, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		// Test custom values
		if config.APIKey != "custom_api_key" {
			t.Errorf("Expected APIKey 'custom_api_key', got '%s'", config.APIKey)
		}

		if config.ServiceURL != "https://example.com:9000/" {
			t.Errorf("Expected ServiceURL 'https://example.com:9000/', got '%s'", config.ServiceURL)
		}

		if config.PrivKeyPath != "/custom/path/to/private.key" {
			t.Errorf("Expected PrivKeyPath '/custom/path/to/private.key', got '%s'", config.PrivKeyPath)
		}

		if config.CertPath != "/custom/path/to/certificate.crt" {
			t.Errorf("Expected CertPath '/custom/path/to/certificate.crt', got '%s'", config.CertPath)
		}

		if config.CountryCode != "EE" {
			t.Errorf("Expected CountryCode 'EE', got '%s'", config.CountryCode)
		}

		if config.BasePath != "/status-list" {
			t.Errorf("Expected BasePath '/status-list', got '%s'", config.BasePath)
		}

		if config.CleanupEnabled {
			t.Error("Expected cleanup to be disabled via env")
		}

		if config.CleanupHour != 5 {
			t.Errorf("Expected CleanupHour 5, got %d", config.CleanupHour)
		}

		if config.CleanupMinute != 30 {
			t.Errorf("Expected CleanupMinute 30, got %d", config.CleanupMinute)
		}

		if config.RenewalEnabled {
			t.Error("Expected renewal to be disabled via env")
		}

		if config.RenewalHour != 14 {
			t.Errorf("Expected RenewalHour 14, got %d", config.RenewalHour)
		}

		if config.RenewalMinute != 45 {
			t.Errorf("Expected RenewalMinute 45, got %d", config.RenewalMinute)
		}

		// Test custom doctypes
		expectedDoctypes := map[string]bool{
			"CUSTOM1": true,
			"CUSTOM2": true,
			"CUSTOM3": true,
		}

		if !reflect.DeepEqual(config.AllowedDoctypes, expectedDoctypes) {
			t.Errorf("Expected custom doctypes %v, got %v", expectedDoctypes, config.AllowedDoctypes)
		}

		// Test that custom directories were created
		dirs := []string{config.StatusListDir, config.BackupDir, config.LogDir}
		for _, dir := range dirs {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				t.Errorf("Directory %s was not created", dir)
			}
		}
	})

	t.Run("load with invalid directory path", func(t *testing.T) {
		// Skip on Windows as it's difficult to create a path that os.MkdirAll will reject
		if runtime.GOOS == "windows" {
			t.Skip("Skipping invalid directory path test on Windows")
		}

		// Set an invalid directory path that cannot be created
		invalidPath := "/invalid"
		os.Setenv("STATUS_LIST_DIR", invalidPath)

		_, err := Load()
		if err == nil {
			t.Error("Expected Load() to fail with invalid directory path, but it succeeded")
		}
	})
}

func TestConfigStruct(t *testing.T) {
	// Test the Config struct initialization
	config := &Config{
		APIKey:              "test_key",
		ServiceURL:          "http://test.com/",
		TokenStatusListSize: 5000,
		StatusListDir:       "/test/status",
		BackupDir:           "/test/backup",
		LogDir:              "/test/logs",
		PrivKeyPath:         "/test/private.key",
		CertPath:            "/test/certificate.crt",
		CountryCode:         "TEST",
		AllowedDoctypes:     map[string]bool{"TEST": true},
		CleanupEnabled:      true,
		CleanupHour:         3,
		CleanupMinute:       45,
		RenewalEnabled:      true,
		RenewalHour:         14,
		RenewalMinute:       30,
	}

	// Test all fields are set correctly
	if config.APIKey != "test_key" {
		t.Errorf("Expected APIKey 'test_key', got '%s'", config.APIKey)
	}

	if config.TokenStatusListSize != 5000 {
		t.Errorf("Expected TokenStatusListSize 5000, got %d", config.TokenStatusListSize)
	}

	if !config.AllowedDoctypes["TEST"] {
		t.Error("Expected AllowedDoctypes to contain 'TEST'")
	}

	// Test validation methods
	if !config.ValidateDoctype("TEST") {
		t.Error("Expected ValidateDoctype('TEST') to return true")
	}

	if !config.ValidateCountry("TEST") {
		t.Error("Expected ValidateCountry('TEST') to return true")
	}

	// Test certificate paths
	privKey, cert := config.GetCertificatePaths()
	if privKey != "/test/private.key" || cert != "/test/certificate.crt" {
		t.Errorf("Expected certificate paths '/test/private.key' and '/test/certificate.crt', got '%s' and '%s'", privKey, cert)
	}
}

// TestStorageConfigurationParsing tests that storage configuration fields are correctly loaded
func TestStorageConfigurationParsing(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(*testing.T, *Config)
	}{
		{
			name: "default local storage backend",
			envVars: map[string]string{
				"STATUS_LIST_DIR": "/tmp/test_status_lists",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.BackendType != "" && cfg.BackendType != "local" {
					t.Errorf("Expected BackendType to be empty or 'local', got '%s'", cfg.BackendType)
				}
				if cfg.StatusListDir != "/tmp/test_status_lists" {
					t.Errorf("Expected StatusListDir '/tmp/test_status_lists', got '%s'", cfg.StatusListDir)
				}
			},
		},
		{
			name: "explicit local storage backend",
			envVars: map[string]string{
				"STATUS_LIST_STORAGE": "local",
				"STATUS_LIST_DIR":     "/var/status_lists",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.BackendType != "local" {
					t.Errorf("Expected BackendType 'local', got '%s'", cfg.BackendType)
				}
			},
		},
		{
			name: "S3 storage backend with all required fields",
			envVars: map[string]string{
				"STATUS_LIST_STORAGE":  "s3",
				"S3_BUCKET":            "my-status-lists",
				"S3_REGION":            "us-west-2",
				"S3_ACCESS_KEY_ID":     "AKIAIOSFODNN7EXAMPLE",
				"S3_SECRET_ACCESS_KEY": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.BackendType != "s3" {
					t.Errorf("Expected BackendType 's3', got '%s'", cfg.BackendType)
				}
				if cfg.S3Bucket != "my-status-lists" {
					t.Errorf("Expected S3Bucket 'my-status-lists', got '%s'", cfg.S3Bucket)
				}
				if cfg.S3Region != "us-west-2" {
					t.Errorf("Expected S3Region 'us-west-2', got '%s'", cfg.S3Region)
				}
				if cfg.S3AccessKeyID != "AKIAIOSFODNN7EXAMPLE" {
					t.Errorf("Expected S3AccessKeyID 'AKIAIOSFODNN7EXAMPLE', got '%s'", cfg.S3AccessKeyID)
				}
				if cfg.S3SecretAccessKey != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
					t.Errorf("Expected S3SecretAccessKey to match, got '%s'", cfg.S3SecretAccessKey)
				}
			},
		},
		{
			name: "S3 storage with custom endpoint",
			envVars: map[string]string{
				"STATUS_LIST_STORAGE":  "s3",
				"S3_BUCKET":            "my-bucket",
				"S3_ACCESS_KEY_ID":     "minioadmin",
				"S3_SECRET_ACCESS_KEY": "minioadmin",
				"S3_ENDPOINT":          "http://localhost:9000",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.S3Endpoint != "http://localhost:9000" {
					t.Errorf("Expected S3Endpoint 'http://localhost:9000', got '%s'", cfg.S3Endpoint)
				}
			},
		},
		{
			name: "S3 storage with optional region",
			envVars: map[string]string{
				"STATUS_LIST_STORAGE":  "s3",
				"S3_BUCKET":            "my-bucket",
				"S3_ACCESS_KEY_ID":     "test",
				"S3_SECRET_ACCESS_KEY": "test",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.S3Region != "us-east-1" {
					t.Errorf("Expected default S3Region 'us-east-1', got '%s'", cfg.S3Region)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up environment
			defer func() {
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
			}()

			// Set environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Load configuration
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			// Run validation
			tt.validate(t, cfg)
		})
	}
}

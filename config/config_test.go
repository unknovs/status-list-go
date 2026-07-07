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
	"strings"
	"testing"

	"azugo.io/core/validation"
)

// TestValidateAPIKey ensures internal mode refuses to start with a missing or
// default API key, since the write endpoints are guarded only by that key.
func TestValidateAPIKey(t *testing.T) {
	const apiKeyErr = "api_key must be set"

	tests := []struct {
		name        string
		serviceMode string
		apiKey      string
		wantAPIErr  bool
	}{
		{name: "internal default key rejected", serviceMode: "internal", apiKey: "test", wantAPIErr: true},
		{name: "internal empty key rejected", serviceMode: "internal", apiKey: "", wantAPIErr: true},
		{name: "internal whitespace key rejected", serviceMode: "internal", apiKey: "   ", wantAPIErr: true},
		{name: "internal real key accepted", serviceMode: "internal", apiKey: "a-strong-secret", wantAPIErr: false},
		{name: "public empty key allowed", serviceMode: "public", apiKey: "", wantAPIErr: false},
		{name: "unset mode defaults to internal and is enforced", serviceMode: "", apiKey: "test", wantAPIErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				APIKey:      tt.apiKey,
				ServiceMode: tt.serviceMode,
				CountryCode: "LV",
			}

			err := cfg.Validate(validation.New())

			gotAPIErr := err != nil && strings.Contains(err.Error(), apiKeyErr)
			if gotAPIErr != tt.wantAPIErr {
				t.Fatalf("Validate() api_key error = %v (err=%v), want api_key error = %v", gotAPIErr, err, tt.wantAPIErr)
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
		{name: "empty string", input: "", expected: ""},
		{name: "root slash", input: "/", expected: ""},
		{name: "path without leading slash", input: "api", expected: "/api"},
		{name: "path with leading slash", input: "/api", expected: "/api"},
		{name: "path with trailing slash", input: "/api/", expected: "/api"},
		{name: "path with both slashes", input: "/api/", expected: "/api"},
		{name: "nested path", input: "/api/v1", expected: "/api/v1"},
		{name: "nested path with trailing slash", input: "/api/v1/", expected: "/api/v1"},
		{name: "whitespace only", input: "   ", expected: ""},
		{name: "path with whitespace", input: "  /api  ", expected: "/api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeBasePath(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeBasePath(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseAllowedDoctypes(t *testing.T) {
	t.Run("default doctypes when raw is empty", func(t *testing.T) {
		result := parseAllowedDoctypes("")
		expectedDoctypes := map[string]bool{
			"eu.europa.ec.eudi.ehic.1":    true,
			"eu.europa.ec.eudi.hiid.1":    true,
			"eu.europa.ec.eudi.pid.1":     true,
			"org.iso.18013.5.1.mDL":       true,
			"urn:eudi:pid:1":              true,
			"urn:eu.europa.ec.eudi:pid:1": true,
		}
		if !reflect.DeepEqual(result, expectedDoctypes) {
			t.Errorf("parseAllowedDoctypes(\"\") = %v, expected %v", result, expectedDoctypes)
		}
	})

	t.Run("custom doctypes from comma-separated string", func(t *testing.T) {
		result := parseAllowedDoctypes("custom1,custom2,custom3")
		expected := map[string]bool{"custom1": true, "custom2": true, "custom3": true}
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("parseAllowedDoctypes(%q) = %v, expected %v", "custom1,custom2,custom3", result, expected)
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		result := parseAllowedDoctypes(" PID , MDL ")
		expected := map[string]bool{"PID": true, "MDL": true}
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("parseAllowedDoctypes with spaces = %v, expected %v", result, expected)
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
		if err := ensureDir(testDir); err != nil {
			t.Errorf("ensureDir() failed: %v", err)
		}
		if _, err := os.Stat(testDir); os.IsNotExist(err) {
			t.Error("Directory was not created")
		}
	})

	t.Run("create nested directories", func(t *testing.T) {
		testDir := filepath.Join(tempDir, "level1", "level2", "level3")
		if err := ensureDir(testDir); err != nil {
			t.Errorf("ensureDir() failed: %v", err)
		}
		if _, err := os.Stat(testDir); os.IsNotExist(err) {
			t.Error("Nested directory was not created")
		}
	})

	t.Run("directory already exists", func(t *testing.T) {
		testDir := filepath.Join(tempDir, "existing")
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}
		if err := ensureDir(testDir); err != nil {
			t.Errorf("ensureDir() failed on existing directory: %v", err)
		}
	})
}

func TestConfigValidateDoctype(t *testing.T) {
	cfg := &Config{
		AllowedDoctypes: map[string]bool{"PID": true, "MDL": true},
	}

	tests := []struct {
		doctype  string
		expected bool
	}{
		{"PID", true},
		{"MDL", true},
		{"INVALID", false},
		{"", false},
		{"pid", false},
	}

	for _, tt := range tests {
		t.Run(tt.doctype, func(t *testing.T) {
			if got := cfg.ValidateDoctype(tt.doctype); got != tt.expected {
				t.Errorf("ValidateDoctype(%q) = %v, expected %v", tt.doctype, got, tt.expected)
			}
		})
	}
}

func TestConfigValidateCountry(t *testing.T) {
	cfg := &Config{CountryCode: "LV"}

	tests := []struct {
		country  string
		expected bool
	}{
		{"LV", true},
		{"US", false},
		{"", false},
		{"lv", false},
	}

	for _, tt := range tests {
		t.Run(tt.country, func(t *testing.T) {
			if got := cfg.ValidateCountry(tt.country); got != tt.expected {
				t.Errorf("ValidateCountry(%q) = %v, expected %v", tt.country, got, tt.expected)
			}
		})
	}
}

func TestConfigGetCertificatePaths(t *testing.T) {
	cfg := &Config{
		PrivKeyPath: "/path/to/private.key",
		CertPath:    "/path/to/certificate.crt",
	}

	privKey, cert := cfg.GetCertificatePaths()
	if privKey != cfg.PrivKeyPath {
		t.Errorf("privKeyPath = %q, expected %q", privKey, cfg.PrivKeyPath)
	}
	if cert != cfg.CertPath {
		t.Errorf("certPath = %q, expected %q", cert, cfg.CertPath)
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := &Config{
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

	if cfg.APIKey != "test_key" {
		t.Errorf("Expected APIKey 'test_key', got %q", cfg.APIKey)
	}
	if cfg.TokenStatusListSize != 5000 {
		t.Errorf("Expected TokenStatusListSize 5000, got %d", cfg.TokenStatusListSize)
	}
	if !cfg.AllowedDoctypes["TEST"] {
		t.Error("Expected AllowedDoctypes to contain 'TEST'")
	}
	if !cfg.ValidateDoctype("TEST") {
		t.Error("ValidateDoctype('TEST') should return true")
	}
	if !cfg.ValidateCountry("TEST") {
		t.Error("ValidateCountry('TEST') should return true")
	}
	privKey, cert := cfg.GetCertificatePaths()
	if privKey != "/test/private.key" || cert != "/test/certificate.crt" {
		t.Errorf("GetCertificatePaths() = (%q, %q)", privKey, cert)
	}
}

func TestStorageConfigurationParsing(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *Config
		validate func(*testing.T, *Config)
	}{
		{
			name: "default local storage backend",
			cfg: &Config{
				BackendType:   "local",
				StatusListDir: "/tmp/test_status_lists",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.BackendType != "local" {
					t.Errorf("Expected BackendType 'local', got %q", cfg.BackendType)
				}
				if cfg.StatusListDir != "/tmp/test_status_lists" {
					t.Errorf("Expected StatusListDir '/tmp/test_status_lists', got %q", cfg.StatusListDir)
				}
			},
		},
		{
			name: "S3 storage backend",
			cfg: &Config{
				BackendType:       "s3",
				S3Bucket:          "my-status-lists",
				S3Region:          "us-west-2",
				S3AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				S3SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.BackendType != "s3" {
					t.Errorf("Expected BackendType 's3', got %q", cfg.BackendType)
				}
				if cfg.S3Bucket != "my-status-lists" {
					t.Errorf("Expected S3Bucket 'my-status-lists', got %q", cfg.S3Bucket)
				}
				if cfg.S3Region != "us-west-2" {
					t.Errorf("Expected S3Region 'us-west-2', got %q", cfg.S3Region)
				}
				if cfg.S3AccessKeyID != "AKIAIOSFODNN7EXAMPLE" {
					t.Errorf("S3AccessKeyID mismatch: got %q", cfg.S3AccessKeyID)
				}
			},
		},
		{
			name: "S3 storage with custom endpoint",
			cfg: &Config{
				BackendType: "s3",
				S3Bucket:    "my-bucket",
				S3Endpoint:  "http://localhost:9000",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.S3Endpoint != "http://localhost:9000" {
					t.Errorf("Expected S3Endpoint 'http://localhost:9000', got %q", cfg.S3Endpoint)
				}
			},
		},
		{
			name: "S3 default region",
			cfg: &Config{
				BackendType: "s3",
				S3Region:    "us-east-1",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.S3Region != "us-east-1" {
					t.Errorf("Expected default S3Region 'us-east-1', got %q", cfg.S3Region)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.cfg)
		})
	}
}

// TestResolveSecret verifies the Docker/Kubernetes secret-file resolution restored
// after the viper migration dropped it (the cause of S3 SignatureDoesNotMatch when a
// credential env var points to a mounted secret file).
func TestResolveSecret(t *testing.T) {
	dir := t.TempDir()

	secretFile := filepath.Join(dir, "s3_secret")
	if err := os.WriteFile(secretFile, []byte("super-secret-value\n"), 0o600); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "absolute path to file yields trimmed contents", value: secretFile, want: "super-secret-value"},
		{name: "plain credential passes through", value: "AKIAIOSFODNN7EXAMPLE", want: "AKIAIOSFODNN7EXAMPLE"},
		{name: "non-existent absolute path passes through", value: "/no/such/secret/path", want: "/no/such/secret/path"},
		{name: "directory path passes through", value: dir, want: dir},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSecret(tt.value); got != tt.want {
				t.Errorf("resolveSecret(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestValidateResolvesFileBackedCredentials ensures Validate resolves S3 credentials
// (and the API key) supplied as secret-file paths before they are used, so a file-backed
// API key is validated on its real value and S3 credentials are the file contents.
func TestValidateResolvesFileBackedCredentials(t *testing.T) {
	dir := t.TempDir()

	writeSecret := func(name, contents string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(contents), 0o600); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}

		return p
	}

	cfg := &Config{
		ServiceMode:       "internal",
		APIKey:            writeSecret("api_key", "real-api-key\n"),
		S3AccessKeyID:     writeSecret("s3_access", "REALACCESSKEYID\n"),
		S3SecretAccessKey: writeSecret("s3_secret", "real-secret-access-key\n"),
	}

	if err := cfg.Validate(validation.New()); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}

	if cfg.APIKey != "real-api-key" {
		t.Errorf("APIKey = %q, want resolved file contents", cfg.APIKey)
	}
	if cfg.S3AccessKeyID != "REALACCESSKEYID" {
		t.Errorf("S3AccessKeyID = %q, want resolved file contents", cfg.S3AccessKeyID)
	}
	if cfg.S3SecretAccessKey != "real-secret-access-key" {
		t.Errorf("S3SecretAccessKey = %q, want resolved file contents", cfg.S3SecretAccessKey)
	}
}

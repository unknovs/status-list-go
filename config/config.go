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
	"log"
	"os"
	"strconv"
	"strings"
)

// Config holds the application configuration
type Config struct {
	APIKey              string
	ServiceURL          string
	SwaggerURLPrefix    string
	BasePath            string // Base path for all routes (e.g., "/api")
	ServiceMode         string // "public" or "internal" - controls which endpoints are registered
	TokenStatusListSize int
	StatusListDir       string
	BackupDir           string
	LogDir              string
	CleanupEnabled      bool
	CleanupHour         int
	CleanupMinute       int

	// Simple certificate configuration
	PrivKeyPath string
	CertPath    string
	CountryCode string

	// Storage configuration
	BackendType       string // "local" or "s3"
	S3Bucket          string
	S3Region          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Endpoint        string // Custom S3 endpoint for compatible services (optional)

	AllowedDoctypes map[string]bool
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	log.Println("Loading configuration from environment variables")

	config := &Config{
		APIKey:           getEnv("API_KEY", "test"),
		ServiceURL:       getEnv("SERVICE_URL", "http://localhost:8080/"), // from this value, Status List URL is derived
		SwaggerURLPrefix: getEnv("SWAGGER_URL_PREFIX", ""),                // Empty means use ServiceURL as base, e.g., "/api"
		BasePath:         NormalizeBasePath(getEnv("BASE_PATH", "")),      // Base path for all routes (e.g., "/api")
		ServiceMode:      getEnv("SERVICE_MODE", "internal"),              // "public" (read-only (GET methods), no Swagger) or "internal" (full API + Swagger)

		// Maximum number of entries (tokens) that a single status list can hold before a new list needs to be created.
		TokenStatusListSize: 10000,

		// STATUS_LIST_STORAGE=local configuration, local filesystem storage.
		// Backups are not implemented, shall be done externally, on infrastructure level if needed.
		StatusListDir: getEnv("STATUS_LIST_DIR", "/var/opt/status_lists"),
		BackupDir:     getEnv("BACKUP_DIR", "/var/opt/status_list_backup"),
		LogDir:        getEnv("LOG_DIR", "/tmp/status_lists"),

		// Expired status list cleanup service configuration
		CleanupEnabled: getEnvBool("STATUS_LIST_CLEANUP_ENABLED", true),
		CleanupHour:    normalizeHour(getEnvInt("STATUS_LIST_CLEANUP_HOUR", 2)),     // Default to 2 AM. Normalize to valid hour (value between 0-23)
		CleanupMinute:  normalizeMinute(getEnvInt("STATUS_LIST_CLEANUP_MINUTE", 0)), // Default to 0 minutes. Normalize to valid minute (value between 0-59)

		// Certificate configuration from environment or Docker secrets
		// PrivKeyPath: getEnv("PRIVATE_KEY_PATH", "/run/secrets/private_key"),
		// CertPath:    getEnv("CERTIFICATE_PATH", "/run/secrets/certificate"),
		PrivKeyPath: getEnvPath("PRIVATE_KEY_PATH", "temp/private_key/decrypted_key.pem"),
		CertPath:    getEnvPath("CERTIFICATE_PATH", "temp/certificate/PID-DS-0002.cert.der"),

		// Storage configuration
		BackendType:       getEnv("STATUS_LIST_STORAGE", "local"), // "local" or "s3", default to "local" for local filesystem storage
		S3Bucket:          getEnv("S3_BUCKET", ""),
		S3Region:          getEnv("S3_REGION", "us-east-1"),
		S3AccessKeyID:     getEnv("S3_ACCESS_KEY_ID", ""),
		S3SecretAccessKey: getEnv("S3_SECRET_ACCESS_KEY", ""),
		S3Endpoint:        getEnv("S3_ENDPOINT", ""),

		// Allowed document types (reads `,` separated values)
		// and country code (supports one country code, if needed can be extended to multiple using `getEnvArray`)
		AllowedDoctypes: getAllowedDoctypes(),
		CountryCode:     getEnv("COUNTRY_CODE", "LV"), // Default to Latvia (LV)
	}

	// Validate and normalize service mode
	config.ServiceMode = strings.ToLower(strings.TrimSpace(config.ServiceMode))
	if config.ServiceMode != "public" && config.ServiceMode != "internal" {
		log.Printf("Invalid SERVICE_MODE '%s', defaulting to 'internal'", config.ServiceMode)
		config.ServiceMode = "internal"
	}
	log.Printf("Service mode: %s", config.ServiceMode)

	// Ensure directories exist
	if err := ensureDir(config.StatusListDir); err != nil {
		return nil, err
	}
	if err := ensureDir(config.BackupDir); err != nil {
		return nil, err
	}
	if err := ensureDir(config.LogDir); err != nil {
		return nil, err
	}

	return config, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		// Check if it's a file path (Docker secrets, Kubernetes secrets, etc.)
		if strings.HasPrefix(value, "/") {
			if data, err := os.ReadFile(value); err == nil {
				return strings.TrimSpace(string(data))
			}
			// If file doesn't exist or can't be read, treat as regular value
		}
		return value
	}
	// Fall back to default value if environment variable is not set or empty
	return defaultValue
}

// NormalizeBasePath ensures configured base paths are canonicalized to a leading slash and no trailing slash.
func NormalizeBasePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed == "/" {
		return ""
	}

	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}

	trimmed = strings.TrimRight(trimmed, "/")
	if trimmed == "" {
		return ""
	}

	return trimmed
}

func getEnvPath(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvArray parses a comma-separated environment variable into a map with true values
// If the environment variable is not set or empty, returns an empty map
func getEnvArray(key string) map[string]bool {
	result := make(map[string]bool)

	value := os.Getenv(key)
	if value == "" {
		return result
	}

	// Split by comma and trim whitespace
	items := strings.Split(value, ",")
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result[trimmed] = true
		}
	}

	return result
}

func getEnvBool(key string, defaultValue bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	switch strings.ToLower(value) {
	case "true", "1", "yes", "y", "on":
		return true
	case "false", "0", "no", "n", "off":
		return false
	default:
		return defaultValue
	}
}

func getEnvInt(key string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}

	return parsed
}

func getAllowedDoctypes() map[string]bool {
	// Check if doctypes are specified via environment variable
	envDoctypes := getEnvArray("ALLOWED_DOCTYPES")
	if len(envDoctypes) > 0 {
		return envDoctypes
	}

	// Default hardcoded doctypes if environment variable is not set
	return map[string]bool{
		"eu.europa.ec.eudi.ehic.1":    true,
		"eu.europa.ec.eudi.hiid.1":    true,
		"eu.europa.ec.eudi.pid.1":     true,
		"org.iso.18013.5.1.mDL":       true,
		"urn:eudi:pid:1":              true,
		"urn:eu.europa.ec.eudi:pid:1": true,
	}
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func normalizeHour(value int) int {
	if value < 0 || value > 23 {
		return 2
	}
	return value
}

func normalizeMinute(value int) int {
	if value < 0 || value > 59 {
		return 0
	}
	return value
}

// ValidateDoctype validates if the doctype is allowed
func (c *Config) ValidateDoctype(doctype string) bool {
	return c.AllowedDoctypes[doctype]
}

// ValidateCountry validates if the country matches the configured country
func (c *Config) ValidateCountry(country string) bool {
	return country == c.CountryCode
}

// GetCertificatePaths returns the certificate paths for this instance
func (c *Config) GetCertificatePaths() (privKeyPath string, certPath string) {
	return c.PrivKeyPath, c.CertPath
}

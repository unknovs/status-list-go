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
	"fmt"
	"os"
	"strings"

	"azugo.io/core/validation"
	pkgconfig "github.com/gmb-lib/go-platform-kit/config"
	"github.com/spf13/viper"
)

// Configuration holds the application configuration.
type Configuration struct {
	*pkgconfig.BaseConfiguration `mapstructure:",squash"`

	APIKey           string `mapstructure:"api_key"`
	ServiceURL       string `mapstructure:"service_url"`
	SwaggerURLPrefix string `mapstructure:"swagger_url_prefix"`
	BasePath         string `mapstructure:"base_path"`
	ServiceMode      string `mapstructure:"service_mode"`

	TokenStatusListSize int `mapstructure:"token_status_list_size"`

	StatusListDir string `mapstructure:"status_list_dir"`
	BackupDir     string `mapstructure:"backup_dir"`
	LogDir        string `mapstructure:"log_dir"`

	CleanupEnabled bool `mapstructure:"status_list_cleanup_enabled"`
	CleanupHour    int  `mapstructure:"status_list_cleanup_hour"`
	CleanupMinute  int  `mapstructure:"status_list_cleanup_minute"`

	RenewalEnabled bool `mapstructure:"status_list_renewal_enabled"`
	RenewalHour    int  `mapstructure:"status_list_renewal_hour"`
	RenewalMinute  int  `mapstructure:"status_list_renewal_minute"`

	PrivKeyPath string `mapstructure:"private_key_path"`
	CertPath    string `mapstructure:"certificate_path"`
	CountryCode string `mapstructure:"country_code"`

	BackendType       string `mapstructure:"status_list_storage"`
	S3Bucket          string `mapstructure:"s3_bucket"`
	S3Region          string `mapstructure:"s3_region"`
	S3AccessKeyID     string `mapstructure:"s3_access_key_id"`
	S3SecretAccessKey string `mapstructure:"s3_secret_access_key"`
	S3Endpoint        string `mapstructure:"s3_endpoint"`

	AllowedDoctypesRaw string          `mapstructure:"allowed_doctypes"`
	AllowedDoctypes    map[string]bool `mapstructure:"-"`
}

// Config is an alias for Configuration for backward compatibility.
type Config = Configuration

// New returns a new Configuration with the embedded base initialized.
func New() *Configuration {
	return &Configuration{
		BaseConfiguration: pkgconfig.New(),
	}
}

// Bind registers environment-variable bindings and defaults with viper.
func (c *Configuration) Bind(_ string, v *viper.Viper) {
	c.BaseConfiguration.Bind("", v)

	v.SetDefault("service_name", "status-list")
	v.SetDefault("api_key", "test")
	v.SetDefault("service_url", "http://localhost:8080/")
	v.SetDefault("service_mode", "internal")
	v.SetDefault("token_status_list_size", 10000)
	v.SetDefault("status_list_dir", "/var/opt/status_lists")
	v.SetDefault("backup_dir", "/var/opt/status_list_backup")
	v.SetDefault("log_dir", "/tmp/status_lists")
	v.SetDefault("status_list_cleanup_enabled", true)
	v.SetDefault("status_list_cleanup_hour", 4)
	v.SetDefault("status_list_cleanup_minute", 0)
	v.SetDefault("status_list_renewal_enabled", true)
	v.SetDefault("status_list_renewal_hour", 12)
	v.SetDefault("status_list_renewal_minute", 0)
	v.SetDefault("private_key_path", "temp/private_key/decrypted_key.pem")
	v.SetDefault("certificate_path", "temp/certificate/PID-DS-0002.cert.der")
	v.SetDefault("country_code", "LV")
	v.SetDefault("status_list_storage", "local")
	v.SetDefault("s3_region", "us-east-1")

	_ = v.BindEnv("api_key", "API_KEY")
	_ = v.BindEnv("service_url", "SERVICE_URL")
	_ = v.BindEnv("swagger_url_prefix", "SWAGGER_URL_PREFIX")
	_ = v.BindEnv("base_path", "BASE_PATH")
	_ = v.BindEnv("service_mode", "SERVICE_MODE")
	_ = v.BindEnv("token_status_list_size", "TOKEN_STATUS_LIST_SIZE")
	_ = v.BindEnv("status_list_dir", "STATUS_LIST_DIR")
	_ = v.BindEnv("backup_dir", "BACKUP_DIR")
	_ = v.BindEnv("log_dir", "LOG_DIR")
	_ = v.BindEnv("status_list_cleanup_enabled", "STATUS_LIST_CLEANUP_ENABLED")
	_ = v.BindEnv("status_list_cleanup_hour", "STATUS_LIST_CLEANUP_HOUR")
	_ = v.BindEnv("status_list_cleanup_minute", "STATUS_LIST_CLEANUP_MINUTE")
	_ = v.BindEnv("status_list_renewal_enabled", "STATUS_LIST_RENEWAL_ENABLED")
	_ = v.BindEnv("status_list_renewal_hour", "STATUS_LIST_RENEWAL_HOUR")
	_ = v.BindEnv("status_list_renewal_minute", "STATUS_LIST_RENEWAL_MINUTE")
	_ = v.BindEnv("private_key_path", "PRIVATE_KEY_PATH")
	_ = v.BindEnv("certificate_path", "CERTIFICATE_PATH")
	_ = v.BindEnv("country_code", "COUNTRY_CODE")
	_ = v.BindEnv("status_list_storage", "STATUS_LIST_STORAGE")
	_ = v.BindEnv("s3_bucket", "S3_BUCKET")
	_ = v.BindEnv("s3_region", "S3_REGION")
	_ = v.BindEnv("s3_access_key_id", "S3_ACCESS_KEY_ID")
	_ = v.BindEnv("s3_secret_access_key", "S3_SECRET_ACCESS_KEY")
	_ = v.BindEnv("s3_endpoint", "S3_ENDPOINT")
	_ = v.BindEnv("allowed_doctypes", "ALLOWED_DOCTYPES")
}

// Validate normalizes fields and ensures all required directories exist.
func (c *Configuration) Validate(valid *validation.Validate) error {
	c.BasePath = NormalizeBasePath(c.BasePath)

	c.ServiceMode = strings.ToLower(strings.TrimSpace(c.ServiceMode))
	if c.ServiceMode != "public" && c.ServiceMode != "internal" {
		c.ServiceMode = "internal"
	}

	// In internal mode the write endpoints (/take, /set) are exposed and protected
	// only by the API key. Refuse to start with a missing or well-known default key
	// so the service never ships writable by anyone who knows the default.
	if c.ServiceMode == "internal" {
		if strings.TrimSpace(c.APIKey) == "" || c.APIKey == "test" {
			return fmt.Errorf("api_key must be set to a non-default value in internal mode (set the API_KEY environment variable)")
		}
	}

	c.CleanupHour = normalizeHour(c.CleanupHour)
	c.CleanupMinute = normalizeMinute(c.CleanupMinute)
	c.RenewalHour = normalizeHour(c.RenewalHour)
	c.RenewalMinute = normalizeMinute(c.RenewalMinute)

	c.AllowedDoctypes = parseAllowedDoctypes(c.AllowedDoctypesRaw)

	return valid.Struct(c)
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

// ValidateDoctype validates if the doctype is allowed.
func (c *Config) ValidateDoctype(doctype string) bool {
	return c.AllowedDoctypes[doctype]
}

// ValidateCountry validates if the country matches the configured country.
func (c *Config) ValidateCountry(country string) bool {
	return country == c.CountryCode
}

// GetCertificatePaths returns the certificate paths for this instance.
func (c *Config) GetCertificatePaths() (privKeyPath string, certPath string) {
	return c.PrivKeyPath, c.CertPath
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

func parseAllowedDoctypes(raw string) map[string]bool {
	result := make(map[string]bool)

	if raw != "" {
		for _, item := range strings.Split(raw, ",") {
			if t := strings.TrimSpace(item); t != "" {
				result[t] = true
			}
		}

		return result
	}

	return map[string]bool{
		"eu.europa.ec.eudi.ehic.1":    true,
		"eu.europa.ec.eudi.hiid.1":    true,
		"eu.europa.ec.eudi.pid.1":     true,
		"org.iso.18013.5.1.mDL":       true,
		"urn:eudi:pid:1":              true,
		"urn:eu.europa.ec.eudi:pid:1": true,
	}
}

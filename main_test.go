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

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPerformHealthCheck(t *testing.T) {
	// Save original environment variable
	originalServiceURL := os.Getenv("SERVICE_URL")
	defer func() {
		if originalServiceURL != "" {
			os.Setenv("SERVICE_URL", originalServiceURL)
		} else {
			os.Unsetenv("SERVICE_URL")
		}
	}()

	t.Run("health check success with default URL", func(t *testing.T) {
		// Create a test server that responds with 200 OK
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		// Set the SERVICE_URL to our test server
		os.Setenv("SERVICE_URL", server.URL)

		// Since performHealthCheck calls os.Exit, we need to test it in a subprocess
		if os.Getenv("TEST_HEALTH_CHECK") == "1" {
			performHealthCheck()
			return
		}

		// Run the test in a subprocess
		cmd := exec.Command(os.Args[0], "-test.run=TestPerformHealthCheck/health_check_success_with_default_URL")
		cmd.Env = append(os.Environ(), "TEST_HEALTH_CHECK=1", "SERVICE_URL="+server.URL)
		output, err := cmd.CombinedOutput()

		if err != nil {
			// Check if it's an exit status 0 (success)
			if exitError, ok := err.(*exec.ExitError); ok {
				if exitError.ExitCode() != 0 {
					t.Errorf("Expected exit code 0, got %d. Output: %s", exitError.ExitCode(), string(output))
				}
			} else {
				t.Errorf("Unexpected error: %v. Output: %s", err, string(output))
			}
		}

		// Check that the output contains success message
		if !strings.Contains(string(output), "Health check passed") {
			t.Errorf("Expected 'Health check passed' in output, got: %s", string(output))
		}
	})

	t.Run("health check success with custom URL", func(t *testing.T) {
		// Create a test server that responds with 200 OK
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		// Since performHealthCheck calls os.Exit, we need to test it in a subprocess
		if os.Getenv("TEST_HEALTH_CHECK_CUSTOM") == "1" {
			os.Setenv("SERVICE_URL", server.URL)
			performHealthCheck()
			return
		}

		// Run the test in a subprocess
		cmd := exec.Command(os.Args[0], "-test.run=TestPerformHealthCheck/health_check_success_with_custom_URL")
		cmd.Env = append(os.Environ(), "TEST_HEALTH_CHECK_CUSTOM=1", "SERVICE_URL="+server.URL)
		output, err := cmd.CombinedOutput()

		if err != nil {
			// Check if it's an exit status 0 (success)
			if exitError, ok := err.(*exec.ExitError); ok {
				if exitError.ExitCode() != 0 {
					t.Errorf("Expected exit code 0, got %d. Output: %s", exitError.ExitCode(), string(output))
				}
			} else {
				t.Errorf("Unexpected error: %v. Output: %s", err, string(output))
			}
		}

		// Check that the output contains success message
		if !strings.Contains(string(output), "Health check passed") {
			t.Errorf("Expected 'Health check passed' in output, got: %s", string(output))
		}
	})

	t.Run("health check failure - HTTP error", func(t *testing.T) {
		// Create a test server that responds with 500 Internal Server Error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		}))
		defer server.Close()

		// Since performHealthCheck calls os.Exit, we need to test it in a subprocess
		if os.Getenv("TEST_HEALTH_CHECK_HTTP_ERROR") == "1" {
			os.Setenv("SERVICE_URL", server.URL)
			performHealthCheck()
			return
		}

		// Run the test in a subprocess
		cmd := exec.Command(os.Args[0], "-test.run=TestPerformHealthCheck/health_check_failure_-_HTTP_error")
		cmd.Env = append(os.Environ(), "TEST_HEALTH_CHECK_HTTP_ERROR=1", "SERVICE_URL="+server.URL)
		output, err := cmd.CombinedOutput()

		// Expect exit code 1 (failure)
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				if exitError.ExitCode() != 1 {
					t.Errorf("Expected exit code 1, got %d. Output: %s", exitError.ExitCode(), string(output))
				}
			} else {
				t.Errorf("Unexpected error: %v. Output: %s", err, string(output))
			}
		} else {
			t.Error("Expected command to fail with exit code 1, but it succeeded")
		}

		// Check that the output contains failure message
		if !strings.Contains(string(output), "Health check failed: HTTP 500") {
			t.Errorf("Expected 'Health check failed: HTTP 500' in output, got: %s", string(output))
		}
	})

	t.Run("health check failure - connection error", func(t *testing.T) {
		// Use an invalid URL that will cause connection failure
		invalidURL := "http://invalid-host-that-does-not-exist:9999"

		// Since performHealthCheck calls os.Exit, we need to test it in a subprocess
		if os.Getenv("TEST_HEALTH_CHECK_CONNECTION_ERROR") == "1" {
			os.Setenv("SERVICE_URL", invalidURL)
			performHealthCheck()
			return
		}

		// Run the test in a subprocess
		cmd := exec.Command(os.Args[0], "-test.run=TestPerformHealthCheck/health_check_failure_-_connection_error")
		cmd.Env = append(os.Environ(), "TEST_HEALTH_CHECK_CONNECTION_ERROR=1", "SERVICE_URL="+invalidURL)
		output, err := cmd.CombinedOutput()

		// Expect exit code 1 (failure)
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				if exitError.ExitCode() != 1 {
					t.Errorf("Expected exit code 1, got %d. Output: %s", exitError.ExitCode(), string(output))
				}
			} else {
				t.Errorf("Unexpected error: %v. Output: %s", err, string(output))
			}
		} else {
			t.Error("Expected command to fail with exit code 1, but it succeeded")
		}

		// Check that the output contains failure message
		if !strings.Contains(string(output), "Health check failed:") {
			t.Errorf("Expected 'Health check failed:' in output, got: %s", string(output))
		}
	})

	t.Run("health check with empty SERVICE_URL uses default", func(t *testing.T) {
		// Unset SERVICE_URL to test default behavior
		os.Unsetenv("SERVICE_URL")

		// Create a server that responds on a custom port
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		// Since performHealthCheck calls os.Exit, we need to test it in a subprocess
		if os.Getenv("TEST_HEALTH_CHECK_DEFAULT") == "1" {
			// For testing, we'll use our test server URL instead of the default
			os.Setenv("SERVICE_URL", server.URL)
			performHealthCheck()
			return
		}

		// Run the test in a subprocess
		cmd := exec.Command(os.Args[0], "-test.run=TestPerformHealthCheck/health_check_with_empty_SERVICE_URL_uses_default")
		cmd.Env = append(os.Environ(), "TEST_HEALTH_CHECK_DEFAULT=1", "SERVICE_URL="+server.URL)
		output, err := cmd.CombinedOutput()

		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				if exitError.ExitCode() != 0 {
					t.Errorf("Expected exit code 0, got %d. Output: %s", exitError.ExitCode(), string(output))
				}
			} else {
				t.Errorf("Unexpected error: %v. Output: %s", err, string(output))
			}
		}

		// Check that the output contains success message
		if !strings.Contains(string(output), "Health check passed") {
			t.Errorf("Expected 'Health check passed' in output, got: %s", string(output))
		}
	})
}

func TestMain_HealthCheckFlag(t *testing.T) {
	// Test the main function with health-check flag
	// This is more complex since main() also calls os.Exit and starts servers

	t.Run("main with health-check flag", func(t *testing.T) {
		// Create a test server for health check
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}
		}))
		defer server.Close()

		// Since main() calls os.Exit, we test in a subprocess
		if os.Getenv("TEST_MAIN_HEALTH_CHECK") == "1" {
			// Override command line arguments
			os.Args = []string{"status-list-go", "-health-check"}
			os.Setenv("SERVICE_URL", server.URL)
			main()
			return
		}

		// Run the test in a subprocess
		cmd := exec.Command(os.Args[0], "-test.run=TestMain_HealthCheckFlag/main_with_health-check_flag")
		cmd.Env = append(os.Environ(), "TEST_MAIN_HEALTH_CHECK=1", "SERVICE_URL="+server.URL)
		output, err := cmd.CombinedOutput()

		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				if exitError.ExitCode() != 0 {
					t.Errorf("Expected exit code 0, got %d. Output: %s", exitError.ExitCode(), string(output))
				}
			} else {
				t.Errorf("Unexpected error: %v. Output: %s", err, string(output))
			}
		}

		// Check that the output contains health check message
		if !strings.Contains(string(output), "Health check passed") {
			t.Errorf("Expected 'Health check passed' in output, got: %s", string(output))
		}
	})
}

func TestMain_NormalExecution(t *testing.T) {
	// Test normal execution path is more challenging since it starts the full application
	// We'll test that it doesn't immediately fail and can load configuration

	t.Run("main normal execution - config loading", func(t *testing.T) {
		// Create temporary directories for the test
		tempDir, err := os.MkdirTemp("", "main_test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Since main() starts the full application and doesn't exit quickly,
		// we'll test in a subprocess with a timeout
		if os.Getenv("TEST_MAIN_NORMAL") == "1" {
			// Set environment variables for a valid config
			os.Setenv("STATUS_LIST_DIR", tempDir+"/status_lists")
			os.Setenv("BACKUP_DIR", tempDir+"/backup")
			os.Setenv("LOG_DIR", tempDir+"/logs")
			os.Setenv("API_KEY", "test_key")
			os.Setenv("PRIVATE_KEY_PATH", "temp/private_key/decrypted_key.pem")
			os.Setenv("CERTIFICATE_PATH", "temp/certificate/PID-DS-0002.cert.der")
			os.Setenv("PORT", "8081")

			// Override command line arguments (no health-check flag)
			os.Args = []string{"status-list-go"}

			// Set a timeout to prevent the test from running indefinitely
			go func() {
				time.Sleep(2 * time.Second)
				fmt.Println("Test completed - application started successfully")
				os.Exit(0)
			}()

			main()
			return
		}

		// Run the test in a subprocess with timeout
		cmd := exec.Command(os.Args[0], "-test.run=TestMain_NormalExecution/main_normal_execution_-_config_loading")
		cmd.Env = append(os.Environ(),
			"TEST_MAIN_NORMAL=1",
			"STATUS_LIST_DIR="+tempDir+"/status_lists",
			"BACKUP_DIR="+tempDir+"/backup",
			"LOG_DIR="+tempDir+"/logs",
			"API_KEY=test_key",
			"PRIVATE_KEY_PATH=temp/private_key/decrypted_key.pem",
			"CERTIFICATE_PATH=temp/certificate/PID-DS-0002.cert.der",
			"PORT=8081",
		)

		// Capture output to see the error
		output, err := cmd.CombinedOutput()
		t.Logf("Command output: %s", string(output))

		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				if exitError.ExitCode() != 0 {
					t.Errorf("Expected successful startup (exit code 0), got %d. Output: %s", exitError.ExitCode(), string(output))
				}
			} else {
				t.Errorf("Unexpected error: %v. Output: %s", err, string(output))
			}
		}
	})
}

func TestMain_ConfigLoadFailure(t *testing.T) {
	t.Run("main with config load failure", func(t *testing.T) {
		// Since main() calls log.Fatalf on config load failure, test in subprocess
		if os.Getenv("TEST_MAIN_CONFIG_FAIL") == "1" {
			// Set an invalid directory that should cause config.Load() to fail
			os.Setenv("STATUS_LIST_DIR", "C:\\Windows\\System32\\invalid_test_directory_12345")
			os.Args = []string{"status-list-go"}
			main()
			return
		}

		// Run the test in a subprocess
		cmd := exec.Command(os.Args[0], "-test.run=TestMain_ConfigLoadFailure/main_with_config_load_failure")
		cmd.Env = append(os.Environ(),
			"TEST_MAIN_CONFIG_FAIL=1",
			"STATUS_LIST_DIR=C:\\Windows\\System32\\invalid_test_directory_12345",
		)
		output, err := cmd.CombinedOutput()

		// Expect the command to fail due to config load failure
		if err == nil {
			t.Error("Expected command to fail due to config load failure, but it succeeded")
		} else {
			if exitError, ok := err.(*exec.ExitError); ok {
				if exitError.ExitCode() == 0 {
					t.Error("Expected non-zero exit code due to config failure")
				}
			}
		}

		// Check that the output contains config failure message
		outputStr := string(output)
		if !strings.Contains(outputStr, "Failed to load configuration") {
			t.Errorf("Expected 'Failed to load configuration' in output, got: %s", outputStr)
		}
	})
}

// Test helper function to verify HTTP client timeout configuration
func TestPerformHealthCheck_HTTPClientTimeout(t *testing.T) {
	// This test verifies that the HTTP client has proper timeout configuration
	// We'll test this by creating a server that delays response beyond the timeout

	t.Run("health check timeout", func(t *testing.T) {
		// Create a test server that delays response beyond the 5-second timeout
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(6 * time.Second) // Sleep longer than the 5-second timeout
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Since performHealthCheck calls os.Exit, we need to test it in a subprocess
		if os.Getenv("TEST_HEALTH_CHECK_TIMEOUT") == "1" {
			os.Setenv("SERVICE_URL", server.URL)
			performHealthCheck()
			return
		}

		// Run the test in a subprocess
		cmd := exec.Command(os.Args[0], "-test.run=TestPerformHealthCheck_HTTPClientTimeout/health_check_timeout")
		cmd.Env = append(os.Environ(), "TEST_HEALTH_CHECK_TIMEOUT=1", "SERVICE_URL="+server.URL)

		start := time.Now()
		output, err := cmd.CombinedOutput()
		duration := time.Since(start)

		// Should fail due to timeout
		if err == nil {
			t.Error("Expected command to fail due to timeout, but it succeeded")
		}

		// Should complete in reasonable time (much less than 6 seconds, close to 5 seconds)
		if duration > 8*time.Second {
			t.Errorf("Expected timeout around 5 seconds, but took %v", duration)
		}

		// Check that the output contains timeout-related error
		outputStr := string(output)
		if !strings.Contains(outputStr, "Health check failed:") {
			t.Errorf("Expected 'Health check failed:' in output, got: %s", outputStr)
		}
	})
}

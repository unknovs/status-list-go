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

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/valyala/fasthttp"

	"github.com/unknovs/status-list-go/services/storage"
)

func newTestApp(t *testing.T) *App {
	t.Helper()

	tempDir := t.TempDir()

	t.Setenv("METRICS_ENABLED", "false")
	t.Setenv("SERVICE_NAME", "test")
	t.Setenv("API_KEY", "test-api-key")
	t.Setenv("SERVICE_URL", "http://localhost:8080/")
	t.Setenv("STATUS_LIST_DIR", filepath.Join(tempDir, "status"))
	t.Setenv("BACKUP_DIR", filepath.Join(tempDir, "backup"))
	t.Setenv("LOG_DIR", filepath.Join(tempDir, "logs"))
	t.Setenv("PRIVATE_KEY_PATH", filepath.Join(tempDir, "key.pem"))
	t.Setenv("CERTIFICATE_PATH", filepath.Join(tempDir, "cert.pem"))
	t.Setenv("COUNTRY_CODE", "LV")
	t.Setenv("STATUS_LIST_STORAGE", "local")
	t.Setenv("ALLOWED_DOCTYPES", "PID")

	application, err := NewApp(nil, "")
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	return application
}

func TestNewAppInitializesAzugo(t *testing.T) {
	application := newTestApp(t)

	if application == nil {
		t.Fatal("expected NewApp to return a non-nil instance")
	}
	if application.Azugo() == nil {
		t.Fatal("expected Azugo app to be initialized")
	}
	if application.Storage() == nil {
		t.Fatal("expected storage backend to be initialized")
	}
	if _, ok := application.Storage().(*storage.LocalStorage); !ok {
		t.Fatalf("expected storage backend to be local storage, got %T", application.Storage())
	}
}

func TestHealthEndpoint(t *testing.T) {
	application := newTestApp(t)

	resp := executeRequest(t, application, fasthttp.MethodGet, "/health", nil)

	if got, want := resp.StatusCode(), fasthttp.StatusOK; got != want {
		t.Fatalf("health status code mismatch: got %d want %d", got, want)
	}

	if v := string(resp.Header.Peek("Content-Type")); v != "application/json" {
		t.Fatalf("unexpected content type: %s", v)
	}

	var payload map[string]string
	if err := json.Unmarshal(resp.Body(), &payload); err != nil {
		t.Fatalf("failed to decode health payload: %v", err)
	}
	if payload["status"] != "healthy" {
		t.Fatalf("unexpected health payload: %v", payload)
	}
}

func TestCORSMiddlewareAddsHeaders(t *testing.T) {
	application := newTestApp(t)

	// Azugo's built-in CORS only sets response headers when an Origin is present.
	resp := executeRequest(t, application, fasthttp.MethodGet, "/health", map[string]string{
		"Origin": "http://example.com",
	})

	if origin := string(resp.Header.Peek("Access-Control-Allow-Origin")); origin == "" {
		t.Fatal("expected Access-Control-Allow-Origin to be set")
	}
}

func TestOptionsPreflightShortCircuits(t *testing.T) {
	application := newTestApp(t)

	// Azugo's built-in CORS preflight returns 204 No Content (not 200).
	resp := executeRequest(t, application, fasthttp.MethodOptions, "/token_status_list/take", map[string]string{
		"Origin": "http://example.com",
	})

	if got, want := resp.StatusCode(), fasthttp.StatusNoContent; got != want {
		t.Fatalf("preflight status mismatch: got %d want %d", got, want)
	}
}

func TestSwaggerIndexServed(t *testing.T) {
	application := newTestApp(t)

	resp := executeRequest(t, application, fasthttp.MethodGet, "/token_status_list/swagger", nil)

	if got, want := resp.StatusCode(), fasthttp.StatusOK; got != want {
		t.Fatalf("swagger index status mismatch: got %d want %d", got, want)
	}
	if ct := string(resp.Header.Peek("Content-Type")); ct != "text/html" {
		t.Fatalf("unexpected swagger content type: %s", ct)
	}
	if body := string(resp.Body()); body == "" {
		t.Fatal("expected swagger index body to be present")
	}
}

func executeRequest(t *testing.T, application *App, method, path string, headers map[string]string) *fasthttp.Response {
	t.Helper()

	var req fasthttp.Request
	req.Header.SetMethod(method)
	req.SetRequestURI(path)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	var ctx fasthttp.RequestCtx
	ctx.Init(&req, nil, nil)
	application.Azugo().Handler(&ctx)

	var resp fasthttp.Response
	ctx.Response.CopyTo(&resp)

	return &resp
}

// TestMain_unused is here to ensure the os import is used in test setup.
var _ = os.Getenv

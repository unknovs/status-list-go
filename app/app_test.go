package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/valyala/fasthttp"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/services/storage"
)

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("METRICS_ENABLED", "false")

	tempDir, err := os.MkdirTemp("", "status-list-app-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	statusDir := filepath.Join(tempDir, "status")
	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		t.Fatalf("failed to create status dir: %v", err)
	}

	backupDir := filepath.Join(tempDir, "backup")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("failed to create backup dir: %v", err)
	}

	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}

	return &config.Config{
		APIKey:              "test-api-key",
		ServiceURL:          "http://localhost:8080/",
		SwaggerURLPrefix:    "",
		TokenStatusListSize: 10,
		StatusListDir:       statusDir,
		BackupDir:           backupDir,
		LogDir:              logDir,
		PrivKeyPath:         filepath.Join(tempDir, "key.pem"),
		CertPath:            filepath.Join(tempDir, "cert.pem"),
		CountryCode:         "LV",
		BackendType:         "local",
		AllowedDoctypes:     map[string]bool{"PID": true},
	}
}

func TestNewAppInitializesAzugo(t *testing.T) {
	cfg := newTestConfig(t)
	application := NewApp(cfg)

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
	application := NewApp(newTestConfig(t))

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
	application := NewApp(newTestConfig(t))

	resp := executeRequest(t, application, fasthttp.MethodGet, "/health", nil)

	if origin := string(resp.Header.Peek("Access-Control-Allow-Origin")); origin != "*" {
		t.Fatalf("unexpected CORS origin: %q", origin)
	}
	if methods := string(resp.Header.Peek("Access-Control-Allow-Methods")); methods == "" {
		t.Fatal("expected CORS methods header to be set")
	}
	if headers := string(resp.Header.Peek("Access-Control-Allow-Headers")); headers == "" {
		t.Fatal("expected CORS headers header to be set")
	}
}

func TestOptionsPreflightShortCircuits(t *testing.T) {
	application := NewApp(newTestConfig(t))

	resp := executeRequest(t, application, fasthttp.MethodOptions, "/token_status_list/take", nil)

	if got, want := resp.StatusCode(), fasthttp.StatusOK; got != want {
		t.Fatalf("preflight status mismatch: got %d want %d", got, want)
	}
	if len(resp.Body()) != 0 {
		t.Fatalf("expected preflight body to be empty, got %q", string(resp.Body()))
	}
}

func TestSwaggerIndexServed(t *testing.T) {
	cfg := newTestConfig(t)
	application := NewApp(cfg)

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

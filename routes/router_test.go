package routes

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/valyala/fasthttp"

	"azugo.io/azugo"
	azugoconfig "azugo.io/azugo/config"
	"azugo.io/azugo/server"
	appconfig "github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/handlers"
	"github.com/unknovs/status-list-go/services/storage"
)

func newTestConfig(t *testing.T) *appconfig.Config {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "routes-test")
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

	return &appconfig.Config{
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

func setupTestApp(t *testing.T) *azugo.App {
	t.Helper()

	t.Setenv("METRICS_ENABLED", "false")

	cfg := newTestConfig(t)
	stor, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	statusHandler := handlers.NewStatusListHandler(cfg, stor)

	srvCfg := azugoconfig.New()
	app, err := server.New(nil, server.Options{
		Configuration: srvCfg,
	})
	if err != nil {
		t.Fatalf("failed to create azugo app: %v", err)
	}

	// Allow all origins by default to preserve existing semantics.
	app.RouterOptions().CORS.SetOrigins("*")
	app.RouterOptions().CORS.SetHeaders("Origin", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization", "X-API-Key")

	if err := Init(app, cfg, statusHandler); err != nil {
		t.Fatalf("failed to initialize routes: %v", err)
	}

	// Add CORS middleware like in app.go
	app.Use(corsMiddleware)

	return app
}

func executeRequest(t *testing.T, app *azugo.App, method, path string, headers map[string]string) *fasthttp.Response {
	t.Helper()

	var req fasthttp.Request
	req.Header.SetMethod(method)
	req.SetRequestURI(path)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	var ctx fasthttp.RequestCtx
	ctx.Init(&req, nil, nil)
	app.Handler(&ctx)

	var resp fasthttp.Response
	ctx.Response.CopyTo(&resp)

	return &resp
}

func corsMiddleware(next azugo.RequestHandler) azugo.RequestHandler {
	return func(ctx *azugo.Context) {
		ctx.Header.Set("Access-Control-Allow-Origin", "*")
		ctx.Header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		ctx.Header.Set("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-API-Key")

		if ctx.Method() == fasthttp.MethodOptions {
			ctx.StatusCode(fasthttp.StatusOK)
			ctx.Response().ResetBody()

			return
		}

		next(ctx)
	}
}

func TestInitRoutes(t *testing.T) {
	app := setupTestApp(t)

	tests := []struct {
		name           string
		method         string
		path           string
		headers        map[string]string
		expectedStatus int
		checkBody      bool
		expectedBody   string
		description    string
	}{
		{
			name:           "Health endpoint",
			method:         fasthttp.MethodGet,
			path:           "/health",
			headers:        nil,
			expectedStatus: fasthttp.StatusOK,
			checkBody:      true,
			expectedBody:   `{"status":"healthy"}`,
			description:    "Health endpoint should return healthy status",
		},
		{
			name:           "Root endpoint",
			method:         fasthttp.MethodGet,
			path:           "/",
			headers:        nil,
			expectedStatus: fasthttp.StatusOK,
			checkBody:      true,
			expectedBody:   "OK",
			description:    "Root endpoint should return OK",
		},
		{
			name:           "Swagger UI endpoint",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/swagger",
			headers:        nil,
			expectedStatus: fasthttp.StatusOK,
			checkBody:      false,
			description:    "Swagger UI endpoint should serve HTML",
		},
		{
			name:           "Swagger JSON endpoint - not found",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/swagger/swagger.json",
			headers:        nil,
			expectedStatus: fasthttp.StatusNotFound,
			checkBody:      false,
			description:    "Swagger JSON should return 404 when file not found",
		},
		{
			name:           "Take endpoint - POST",
			method:         fasthttp.MethodPost,
			path:           "/token_status_list/take",
			headers:        map[string]string{"X-API-Key": "test-api-key"},
			expectedStatus: fasthttp.StatusBadRequest, // Will fail validation but route exists
			checkBody:      false,
			description:    "Take endpoint should be routed (returns 400 for invalid request)",
		},
		{
			name:           "Get endpoint - GET",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/get",
			headers:        map[string]string{"X-API-Key": "test-api-key"},
			expectedStatus: fasthttp.StatusBadRequest, // Will fail validation but route exists
			checkBody:      false,
			description:    "Get endpoint should be routed (returns 400 for invalid request)",
		},
		{
			name:           "Set endpoint - POST",
			method:         fasthttp.MethodPost,
			path:           "/token_status_list/set",
			headers:        map[string]string{"X-API-Key": "test-api-key"},
			expectedStatus: fasthttp.StatusBadRequest, // Will fail validation but route exists
			checkBody:      false,
			description:    "Set endpoint should be routed (returns 400 for invalid request)",
		},
		{
			name:           "Status list serving - GET",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/LV/PID/test123",
			headers:        nil,
			expectedStatus: fasthttp.StatusNotFound, // File not found but route exists
			checkBody:      false,
			description:    "Status list serving should be routed (returns 404 for missing file)",
		},
		{
			name:           "Static files - GET",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/static/test.txt",
			headers:        nil,
			expectedStatus: fasthttp.StatusNotFound, // Directory not found but route exists
			checkBody:      false,
			description:    "Static file serving should be routed",
		},
		{
			name:           "OPTIONS preflight",
			method:         fasthttp.MethodOptions,
			path:           "/token_status_list/take",
			headers:        nil,
			expectedStatus: fasthttp.StatusOK,
			checkBody:      true,
			expectedBody:   "",
			description:    "OPTIONS requests should be handled by CORS middleware",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := executeRequest(t, app, tt.method, tt.path, tt.headers)

			if got, want := resp.StatusCode(), tt.expectedStatus; got != want {
				t.Errorf("status code mismatch for %s: got %d want %d", tt.description, got, want)
			}

			if tt.checkBody {
				body := string(resp.Body())
				if tt.expectedBody != "" && body != tt.expectedBody {
					t.Errorf("body mismatch for %s: got %q want %q", tt.description, body, tt.expectedBody)
				}
				if tt.expectedBody == "" && len(body) != 0 {
					t.Errorf("expected empty body for %s, got %q", tt.description, body)
				}
			}
		})
	}
}

func TestSwaggerJSONWithFile(t *testing.T) {
	// Create a temporary swagger.json file
	tempDir, err := os.MkdirTemp("", "swagger-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	staticDir := filepath.Join(tempDir, "static")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatalf("failed to create static dir: %v", err)
	}

	swaggerContent := `{"swagger":"2.0","info":{"title":"Test API","version":"1.0"}}`
	swaggerPath := filepath.Join(staticDir, "swagger.json")
	if err := os.WriteFile(swaggerPath, []byte(swaggerContent), 0o644); err != nil {
		t.Fatalf("failed to write swagger.json: %v", err)
	}

	// Change to temp dir so resolveStaticDir finds our file
	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	app := setupTestApp(t)

	resp := executeRequest(t, app, fasthttp.MethodGet, "/token_status_list/swagger/swagger.json", nil)

	if got, want := resp.StatusCode(), fasthttp.StatusOK; got != want {
		t.Fatalf("swagger JSON status mismatch: got %d want %d", got, want)
	}

	if ct := string(resp.Header.Peek("Content-Type")); ct != "application/json" {
		t.Fatalf("unexpected content type: %s", ct)
	}

	// Parse the response to check if servers were added
	var swagger map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &swagger); err != nil {
		t.Fatalf("failed to parse swagger JSON: %v", err)
	}

	if _, exists := swagger["servers"]; !exists {
		t.Fatal("expected servers field to be added to swagger JSON")
	}

	// Check cache headers
	if cc := string(resp.Header.Peek("Cache-Control")); cc != "no-cache, no-store, must-revalidate" {
		t.Errorf("unexpected cache-control: %s", cc)
	}
}

func TestResolveStaticDir(t *testing.T) {
	// Test with no directories existing
	dir := resolveStaticDir()
	if dir == "" {
		t.Error("expected fallback directory even when none exist")
	}

	// Test with ./static existing
	tempDir, err := os.MkdirTemp("", "static-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	staticDir := filepath.Join(tempDir, "static")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatalf("failed to create static dir: %v", err)
	}

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	dir = resolveStaticDir()
	if dir != "./static" {
		t.Errorf("expected ./static, got %s", dir)
	}
}

func TestLocateSwaggerFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "swagger-locate-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	// No file exists
	path, paths := locateSwaggerFile()
	if path != "" {
		t.Errorf("expected empty path when no file exists, got %s", path)
	}
	if len(paths) == 0 {
		t.Error("expected paths to be returned even when file not found")
	}

	// Create file in ./static
	staticDir := filepath.Join(tempDir, "static")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatalf("failed to create static dir: %v", err)
	}

	swaggerPath := filepath.Join(staticDir, "swagger.json")
	if err := os.WriteFile(swaggerPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write swagger.json: %v", err)
	}

	path, _ = locateSwaggerFile()
	if path != "./static/swagger.json" {
		t.Errorf("expected ./static/swagger.json, got %s", path)
	}
}

func TestReadSwagger(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "swagger-read-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	swaggerPath := filepath.Join(tempDir, "swagger.json")
	swaggerContent := `{"swagger":"2.0","info":{"title":"Test","version":"1.0"}}`
	if err := os.WriteFile(swaggerPath, []byte(swaggerContent), 0o644); err != nil {
		t.Fatalf("failed to write swagger.json: %v", err)
	}

	doc, err := readSwagger(swaggerPath)
	if err != nil {
		t.Fatalf("failed to read swagger: %v", err)
	}

	if doc["swagger"] != "2.0" {
		t.Errorf("expected swagger version 2.0, got %v", doc["swagger"])
	}

	info, ok := doc["info"].(map[string]interface{})
	if !ok {
		t.Fatal("expected info to be map")
	}
	if info["title"] != "Test" {
		t.Errorf("expected title Test, got %v", info["title"])
	}
}

func TestBuildSwaggerServers(t *testing.T) {
	cfg := &appconfig.Config{
		ServiceURL:       "http://localhost:8080/",
		SwaggerURLPrefix: "",
	}

	r := httptest.NewRequest("GET", "http://localhost:8080/", nil)
	r.Host = "localhost:8080"

	servers := buildSwaggerServers(cfg, r)
	if len(servers) != 1 {
		t.Errorf("expected 1 server, got %d", len(servers))
	}
	if servers[0]["url"] != "http://localhost:8080/" {
		t.Errorf("expected server URL http://localhost:8080/, got %s", servers[0]["url"])
	}

	// Test with prefix
	cfg.SwaggerURLPrefix = "/api"
	servers = buildSwaggerServers(cfg, r)
	if len(servers) != 1 {
		t.Errorf("expected 1 server, got %d", len(servers))
	}
	expected := "http://localhost:8080/api/"
	if servers[0]["url"] != expected {
		t.Errorf("expected server URL %s, got %s", expected, servers[0]["url"])
	}

	// Test with forwarded headers (no prefix)
	cfg.SwaggerURLPrefix = ""
	r.Header.Set("X-Forwarded-Prefix", "/proxy")
	r.Header.Set("X-Forwarded-Host", "example.com")
	r.Header.Set("X-Forwarded-Proto", "https")
	servers = buildSwaggerServers(cfg, r)
	if len(servers) != 2 {
		t.Errorf("expected 2 servers with forwarded headers, got %d", len(servers))
	}
	if servers[1]["url"] != "https://example.com/proxy/" {
		t.Errorf("expected forwarded URL https://example.com/proxy/, got %s", servers[1]["url"])
	}
}

func TestForwardedURL(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com/", nil)
	r.Header.Set("X-Forwarded-Host", "example.com")
	r.Header.Set("X-Forwarded-Proto", "https")

	url := forwardedURL("/api", r)
	expected := "https://example.com/api/"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}

	// Test without proto header
	r.Header.Del("X-Forwarded-Proto")
	url = forwardedURL("/api", r)
	expected = "http://example.com/api/"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

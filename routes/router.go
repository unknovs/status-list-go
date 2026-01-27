package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"azugo.io/azugo"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/handlers"
)

// Init wires all HTTP routes into the provided Azugo application instance.
func Init(app *azugo.App, cfg *config.Config, handler *handlers.StatusListHandler) error {
	adapt := func(h http.Handler) azugo.RequestHandler {
		adapter := fasthttpadaptor.NewFastHTTPHandler(h)
		return func(ctx *azugo.Context) {
			adapter(ctx.Context())
		}
	}

	basePath := config.NormalizeBasePath(cfg.BasePath)
	if cfg.BasePath != basePath {
		cfg.BasePath = basePath
	}

	register := func(method, path string, h azugo.RequestHandler, includeFallback bool) {
		add := func(route string) {
			switch method {
			case fasthttp.MethodGet:
				app.Get(route, h)
			case fasthttp.MethodPost:
				app.Post(route, h)
			default:
				panic("unsupported HTTP method")
			}
		}

		add(path)

		if !includeFallback || basePath == "" {
			return
		}

		if !strings.HasPrefix(path, basePath) {
			return
		}

		trimmed := strings.TrimPrefix(path, basePath)
		if trimmed == "" {
			trimmed = "/"
		} else if !strings.HasPrefix(trimmed, "/") {
			trimmed = "/" + trimmed
		}

		if trimmed == path {
			return
		}

		add(trimmed)
	}

	// Helper function to add base path prefix
	prefix := func(path string) string {
		if basePath != "" && !strings.HasPrefix(path, basePath) {
			return basePath + path
		}
		return path
	}

	// Determine service mode ("public" or "internal")
	serviceMode := cfg.ServiceMode
	if serviceMode == "" {
		serviceMode = "internal" // Default to internal mode for backward compatibility
	}
	isPublicMode := serviceMode == "public"

	// REST API routes
	// Internal-only endpoints (require API key)
	if !isPublicMode {
		register(fasthttp.MethodPost, prefix("/token_status_list/take"), adapt(http.HandlerFunc(handler.TakeIndex)), true)
		register(fasthttp.MethodPost, prefix("/token_status_list/set"), adapt(http.HandlerFunc(handler.SetIndex)), true)
	}

	// Public endpoints (no API key required)
	register(fasthttp.MethodGet, prefix("/token_status_list/get"), adapt(http.HandlerFunc(handler.GetIndex)), true)
	register(fasthttp.MethodGet, prefix("/token_status_list/{country}/{doctype}/{id}"), adapt(http.HandlerFunc(handler.ServeStatusList)), true)

	// Static assets
	staticDir := http.Dir(resolveStaticDir())
	staticBasePrefix := prefix("/token_status_list/static/")
	staticHandler := http.StripPrefix(staticBasePrefix, http.FileServer(staticDir))
	register(fasthttp.MethodGet, prefix("/token_status_list/static/{path:*}"), adapt(staticHandler), false)
	if basePath != "" {
		fallbackStaticHandler := http.StripPrefix("/token_status_list/static/", http.FileServer(staticDir))
		register(fasthttp.MethodGet, "/token_status_list/static/{path:*}", adapt(fallbackStaticHandler), false)
	}

	// Swagger assets (internal mode only)
	if !isPublicMode {
		register(fasthttp.MethodGet, prefix("/token_status_list/swagger"), adapt(http.HandlerFunc(swaggerIndex)), true)
		register(fasthttp.MethodGet, prefix("/token_status_list/swagger/swagger.json"), adapt(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveSwaggerJSON(w, r, cfg)
		})), true)
	}

	// Health route using native Azugo handler to avoid adapter overhead.
	healthHandler := func(ctx *azugo.Context) {
		ctx.SkipRequestLog()
		ctx.StatusCode(fasthttp.StatusOK)
		ctx.JSON(map[string]string{"status": "healthy"})
	}
	register(fasthttp.MethodGet, prefix("/health"), healthHandler, true)

	// Root index mirrors old behavior.
	register(fasthttp.MethodGet, prefix("/"), func(ctx *azugo.Context) {
		ctx.StatusCode(fasthttp.StatusOK)
		ctx.Text("OK")
	}, true)

	return nil
}

func resolveStaticDir() string {
	candidates := []string{
		"./static",
		"/static",
	}

	if execDir, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(execDir), "static"))
	}

	for _, dir := range candidates {
		info, err := os.Stat(dir)
		if err == nil && info.IsDir() {
			return dir
		}
	}

	// Fallback to first candidate even if it does not exist so FileServer handles 404.
	return candidates[0]
}

func swaggerIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
    <title>Status List API</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui.css" />
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui-bundle.js"></script>
    <script>
        SwaggerUIBundle({
            url: './swagger/swagger.json',
            dom_id: '#swagger-ui',
            presets: [
                SwaggerUIBundle.presets.apis,
                SwaggerUIBundle.presets.standalone
            ]
        });
    </script>
</body>
</html>`))
}

func serveSwaggerJSON(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	swaggerPath, paths := locateSwaggerFile()
	if swaggerPath == "" {
		message := "swagger.json not found. Please ensure swagger.json exists in the static directory. Paths tried: " + strings.Join(paths, ", ")
		handlers.WriteCustomError(w, http.StatusNotFound, handlers.ErrListNotFound, message)

		return
	}

	swaggerDoc, err := readSwagger(swaggerPath)
	if err != nil {
		handlers.WriteCustomError(w, http.StatusInternalServerError, handlers.ErrListNotFound, err.Error())

		return
	}

	swaggerDoc["servers"] = buildSwaggerServers(cfg, r)

	payload, err := json.Marshal(swaggerDoc)
	if err != nil {
		handlers.WriteCustomError(w, http.StatusInternalServerError, handlers.ErrListNotFound,
			"Failed to serialize modified swagger.json: "+err.Error())

		return
	}

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(payload)
}

func locateSwaggerFile() (string, []string) {
	paths := []string{
		"./static/swagger.json",
		"/static/swagger.json",
	}

	if execDir, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(execDir), "static", "swagger.json"))
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, paths
		}
	}

	return "", paths
}

func readSwagger(path string) (map[string]interface{}, error) {
	swaggerData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read swagger.json: %w", err)
	}

	var swaggerDoc map[string]interface{}
	if err := json.Unmarshal(swaggerData, &swaggerDoc); err != nil {
		return nil, fmt.Errorf("failed to parse swagger.json: %w", err)
	}

	return swaggerDoc, nil
}

func buildSwaggerServers(cfg *config.Config, r *http.Request) []map[string]interface{} {
	servers := make([]map[string]interface{}, 0, 2)

	if prefix := cfg.SwaggerURLPrefix; prefix != "" {
		baseURL := strings.TrimSuffix(cfg.ServiceURL, "/")
		if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}
		servers = append(servers, map[string]interface{}{
			"url": baseURL + prefix + "/",
		})

		return servers
	}

	// Use BasePath if SwaggerURLPrefix is not set
	if basePath := cfg.BasePath; basePath != "" {
		baseURL := strings.TrimSuffix(cfg.ServiceURL, "/")
		if !strings.HasPrefix(basePath, "/") {
			basePath = "/" + basePath
		}
		servers = append(servers, map[string]interface{}{
			"url": baseURL + basePath + "/",
		})

		return servers
	}

	servers = append(servers, map[string]interface{}{
		"url": cfg.ServiceURL,
	})

	if forwarded := r.Header.Get("X-Forwarded-Prefix"); forwarded != "" {
		servers = append(servers, map[string]interface{}{
			"url": forwardedURL(forwarded, r),
		})
	}

	return servers
}

func forwardedURL(prefix string, r *http.Request) string {
	prefix = strings.TrimSuffix(prefix, "/")
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}

	return scheme + "://" + host + prefix + "/"
}

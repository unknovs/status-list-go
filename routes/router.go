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

package routes

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"azugo.io/azugo"
	pkerrors "github.com/gmb-lib/go-platform-kit/errors"
	"github.com/valyala/fasthttp"

	"github.com/unknovs/status-list-go/config"
	localerrors "github.com/unknovs/status-list-go/errors"
	"github.com/unknovs/status-list-go/handlers"
)

// Init wires all HTTP routes into the provided Azugo application instance.
func Init(app *azugo.App, cfg *config.Config, handler *handlers.StatusListHandler) error {
	basePath := config.NormalizeBasePath(cfg.BasePath)
	if cfg.BasePath != basePath {
		cfg.BasePath = basePath
	}

	prefix := createPrefixFunc(basePath)
	isPublicMode := determineServiceMode(cfg)

	if !isPublicMode {
		internal := app.Group(prefix("/token_status_list"))
		internal.Use(apiKeyMiddleware(cfg.APIKey))
		internal.Post("/take", handler.TakeIndex)
		internal.Post("/set", handler.SetIndex)
	}

	app.Get(prefix("/token_status_list/get"), handler.GetIndex)
	app.Get(prefix("/token_status_list/{country}/{doctype}/{id}"), handler.ServeStatusList)

	staticDir := resolveStaticDir()

	app.Get(prefix("/token_status_list/static/{path:*}"), func(ctx *azugo.Context) {
		serveStaticFile(ctx, staticDir, prefix("/token_status_list/static/"))
	})

	if !isPublicMode {
		app.Get(prefix("/token_status_list/swagger"), swaggerIndex)
		app.Get(prefix("/token_status_list/swagger/swagger.json"), func(ctx *azugo.Context) {
			serveSwaggerJSON(ctx, cfg)
		})
	}

	app.Get(prefix("/health"), func(ctx *azugo.Context) {
		ctx.SkipRequestLog()
		ctx.StatusCode(fasthttp.StatusOK)
		ctx.JSON(map[string]string{"status": "healthy"})
	})
	app.Get(prefix("/"), func(ctx *azugo.Context) {
		ctx.StatusCode(fasthttp.StatusOK)
		ctx.Text("OK")
	})

	// Register fallback routes without base path when basePath is set
	if basePath != "" {
		app.Get("/token_status_list/get", handler.GetIndex)
		app.Get("/token_status_list/{country}/{doctype}/{id}", handler.ServeStatusList)

		if !isPublicMode {
			fb := app.Group("/token_status_list")
			fb.Use(apiKeyMiddleware(cfg.APIKey))
			fb.Post("/take", handler.TakeIndex)
			fb.Post("/set", handler.SetIndex)
		}

		app.Get("/health", func(ctx *azugo.Context) {
			ctx.SkipRequestLog()
			ctx.StatusCode(fasthttp.StatusOK)
			ctx.JSON(map[string]string{"status": "healthy"})
		})
		app.Get("/", func(ctx *azugo.Context) {
			ctx.StatusCode(fasthttp.StatusOK)
			ctx.Text("OK")
		})
	}

	return nil
}

func determineServiceMode(cfg *config.Config) bool {
	mode := cfg.ServiceMode
	if mode == "" {
		mode = "internal"
	}

	return mode == "public"
}

func createPrefixFunc(basePath string) func(string) string {
	return func(path string) string {
		if basePath != "" && !strings.HasPrefix(path, basePath) {
			return basePath + path
		}

		return path
	}
}

func serveStaticFile(ctx *azugo.Context, staticDir, stripPrefix string) {
	rawPath := ctx.Path()
	filePath := strings.TrimPrefix(rawPath, stripPrefix)
	filePath = filepath.Join(staticDir, filepath.FromSlash(filePath))

	data, err := os.ReadFile(filePath)
	if err != nil {
		ctx.StatusCode(fasthttp.StatusNotFound)
		return
	}

	ctx.Header.Set("Content-Type", mime(filePath))
	ctx.Raw(data)
}

func mime(path string) string {
	switch filepath.Ext(path) {
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".html", ".htm":
		return "text/html"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

func swaggerIndex(ctx *azugo.Context) {
	ctx.Response().Header.SetContentType("text/html")
	ctx.Response().SetBodyString(`<!DOCTYPE html>
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
</html>`)
}

func serveSwaggerJSON(ctx *azugo.Context, cfg *config.Config) {
	swaggerPath, paths := locateSwaggerFile()
	if swaggerPath == "" {
		msg := "swagger.json not found. Paths tried: " + strings.Join(paths, ", ")

		ctx.StatusCode(fasthttp.StatusNotFound)
		ctx.Header.Set("Content-Type", "application/json")
		ctx.JSON(localerrors.ErrorResponse{
			Error: localerrors.ErrorDetail{Code: localerrors.ErrListNotFound, Message: msg},
		})

		return
	}

	swaggerDoc, err := readSwagger(swaggerPath)
	if err != nil {
		ctx.StatusCode(fasthttp.StatusInternalServerError)
		ctx.Header.Set("Content-Type", "application/json")
		ctx.JSON(localerrors.ErrorResponse{
			Error: localerrors.ErrorDetail{Code: localerrors.ErrInternalServer, Message: err.Error()},
		})

		return
	}

	swaggerDoc["servers"] = buildSwaggerServers(cfg, ctx)

	payload, err := json.Marshal(swaggerDoc)
	if err != nil {
		ctx.StatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	ctx.Header.Set("Pragma", "no-cache")
	ctx.Header.Set("Expires", "0")
	ctx.Header.Set("Content-Type", "application/json")
	ctx.Raw(payload)
}

func locateSwaggerFile() (string, []string) {
	paths := []string{"./static/swagger.json", "/static/swagger.json"}
	if execDir, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(execDir), "static", "swagger.json"))
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, paths
		}
	}

	return "", paths
}

func readSwagger(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read swagger.json: %w", err)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse swagger.json: %w", err)
	}

	return doc, nil
}

func buildSwaggerServers(cfg *config.Config, ctx *azugo.Context) []map[string]interface{} {
	return buildSwaggerServerValues(
		cfg,
		ctx.Header.Get("X-Forwarded-Prefix"),
		ctx.Header.Get("X-Forwarded-Proto"),
		ctx.Header.Get("X-Forwarded-Host"),
		ctx.Host(),
	)
}

func buildSwaggerServerValues(cfg *config.Config, forwardedPrefix, forwardedProto, forwardedHost, host string) []map[string]interface{} {
	if prefix := cfg.SwaggerURLPrefix; prefix != "" {
		baseURL := strings.TrimSuffix(cfg.ServiceURL, "/")

		if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}

		return []map[string]interface{}{{"url": baseURL + prefix + "/"}}
	}

	if basePath := cfg.BasePath; basePath != "" {
		baseURL := strings.TrimSuffix(cfg.ServiceURL, "/")

		if !strings.HasPrefix(basePath, "/") {
			basePath = "/" + basePath
		}

		return []map[string]interface{}{{"url": baseURL + basePath + "/"}}
	}

	servers := []map[string]interface{}{{"url": cfg.ServiceURL}}

	if forwardedPrefix != "" {
		servers = append(servers, map[string]interface{}{"url": buildForwardedURL(forwardedPrefix, forwardedProto, forwardedHost, host)})
	}

	return servers
}

func buildForwardedURL(prefix, forwardedProto, forwardedHost, host string) string {
	prefix = strings.TrimSuffix(prefix, "/")
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	scheme := "http"
	if forwardedProto == "https" {
		scheme = "https"
	}

	if forwardedHost != "" {
		host = forwardedHost
	}

	return scheme + "://" + host + prefix + "/"
}

func resolveStaticDir() string {
	candidates := []string{"./static", "/static"}
	if execDir, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(execDir), "static"))
	}

	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}

	return candidates[0]
}

// apiKeyMiddleware rejects requests without a valid X-Api-Key header.
func apiKeyMiddleware(apiKey string) azugo.RequestHandlerFunc {
	expected := []byte(apiKey)

	return func(next azugo.RequestHandler) azugo.RequestHandler {
		return func(ctx *azugo.Context) {
			provided := []byte(ctx.Header.Get(handlers.APIKeyHeader))
			// Constant-time comparison avoids a timing side-channel; a mismatched
			// length (including an empty header) yields 0, so no header never passes.
			if subtle.ConstantTimeCompare(provided, expected) != 1 {
				ctx.Error(pkerrors.HTTP("request", "unauthorized"))
				return
			}

			next(ctx)
		}
	}
}

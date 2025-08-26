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
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/handlers"
)

// App represents the application
type App struct {
	config *config.Config
	mux    *http.ServeMux
}

// NewApp creates a new application instance
func NewApp(cfg *config.Config) *App {
	mux := http.NewServeMux()

	app := &App{
		config: cfg,
		mux:    mux,
	}

	app.setupRoutes()
	return app
}

// setupRoutes configures all application routes
func (a *App) setupRoutes() {
	// Create handlers
	statusHandler := handlers.NewStatusListHandler(a.config)

	// API routes
	a.mux.HandleFunc("/token_status_list/take", statusHandler.TakeIndex)
	a.mux.HandleFunc("/token_status_list/get", statusHandler.GetIndex)
	a.mux.HandleFunc("/token_status_list/set", statusHandler.SetIndex)

	// RFC-compliant status list serving endpoint
	a.mux.HandleFunc("/token_status_list/", func(w http.ResponseWriter, r *http.Request) {
		// Only match /token_status_list/{country}/{doctype}/{id}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/token_status_list/"), "/")
		if len(parts) == 3 {
			statusHandler.ServeStatusList(w, r)
			return
		}
		http.NotFound(w, r)
	})

	// Static files
	staticDir := "./static/"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		// Try container path
		staticDir = "/static/"
		if _, err := os.Stat(staticDir); os.IsNotExist(err) {
			// Try absolute path relative to executable
			execDir, _ := os.Executable()
			staticDir = filepath.Join(filepath.Dir(execDir), "static")
		}
	}
	a.mux.Handle("/token_status_list/static/", http.StripPrefix("/token_status_list/static/", http.FileServer(http.Dir(staticDir))))

	// Swagger routes
	a.mux.HandleFunc("/token_status_list/swagger/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		// Try to find swagger.json in multiple locations
		swaggerPaths := []string{
			"./static/swagger.json", // Local development
			"/static/swagger.json",  // Container path
		}

		// Try absolute path relative to executable
		if execDir, err := os.Executable(); err == nil {
			swaggerPaths = append(swaggerPaths, filepath.Join(filepath.Dir(execDir), "static", "swagger.json"))
		}

		var swaggerPath string
		var found bool
		for _, path := range swaggerPaths {
			if _, err := os.Stat(path); err == nil {
				swaggerPath = path
				found = true
				break
			}
		}

		if !found {
			// If still not found, serve a basic error response
			handlers.WriteCustomError(w, http.StatusNotFound, handlers.ErrListNotFound,
				fmt.Sprintf("swagger.json not found. Please ensure swagger.json exists in the static directory. Paths tried: %v", swaggerPaths))
			return
		}

		// Read the original swagger.json
		swaggerData, err := os.ReadFile(swaggerPath)
		if err != nil {
			handlers.WriteCustomError(w, http.StatusInternalServerError, handlers.ErrListNotFound,
				fmt.Sprintf("Failed to read swagger.json: %v", err))
			return
		}

		// Parse the JSON to modify it dynamically
		var swaggerDoc map[string]interface{}
		if err := json.Unmarshal(swaggerData, &swaggerDoc); err != nil {
			handlers.WriteCustomError(w, http.StatusInternalServerError, handlers.ErrListNotFound,
				fmt.Sprintf("Failed to parse swagger.json: %v", err))
			return
		}

		// Build server URLs dynamically
		var servers []map[string]interface{}

		if a.config.SwaggerURLPrefix != "" {
			// Use ServiceURL + SwaggerURLPrefix if service is behind proxy and stripprefix is used
			baseURL := strings.TrimSuffix(a.config.ServiceURL, "/")
			prefix := a.config.SwaggerURLPrefix
			if !strings.HasPrefix(prefix, "/") {
				prefix = "/" + prefix
			}
			servers = append(servers, map[string]interface{}{
				"url": baseURL + prefix + "/",
			})
		} else {
			// Use ServiceURL as base and try to auto-detect from request headers
			servers = append(servers, map[string]interface{}{
				"url": a.config.ServiceURL,
			})

			// Also try to detect if we're behind a proxy with prefix
			if r.Header.Get("X-Forwarded-Prefix") != "" {
				scheme := "http"
				if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
					scheme = "https"
				}

				host := r.Host
				if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
					host = forwarded
				}

				pathPrefix := r.Header.Get("X-Forwarded-Prefix")
				pathPrefix = strings.TrimSuffix(pathPrefix, "/")
				if !strings.HasPrefix(pathPrefix, "/") {
					pathPrefix = "/" + pathPrefix
				}
				servers = append(servers, map[string]interface{}{
					"url": fmt.Sprintf("%s://%s%s/", scheme, host, pathPrefix),
				})
			}
		}

		swaggerDoc["servers"] = servers

		// Serialize back to JSON
		modifiedSwagger, err := json.Marshal(swaggerDoc)
		if err != nil {
			handlers.WriteCustomError(w, http.StatusInternalServerError, handlers.ErrListNotFound,
				fmt.Sprintf("Failed to serialize modified swagger.json: %v", err))
			return
		}

		// Add cache-busting headers to ensure swagger UI gets updates
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("Content-Type", "application/json")
		w.Write(modifiedSwagger)
	})

	a.mux.HandleFunc("/token_status_list/swagger", func(w http.ResponseWriter, r *http.Request) {
		swaggerHTML := `<!DOCTYPE html>
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
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(swaggerHTML))
	})

	// Health check
	a.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})
}

// corsMiddleware adds CORS headers to all responses
func (a *App) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-API-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Run starts the application server
func (a *App) Run() error {
	// Get port from environment variable, default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Starting Status List Service on :%s\n", port)

	// Wrap the mux with CORS middleware
	handler := a.corsMiddleware(a.mux)

	return http.ListenAndServe(":"+port, handler)
}

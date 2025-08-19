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
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{
				"error":       "swagger.json not found",
				"message":     "Please ensure swagger.json exists in the static directory",
				"paths_tried": fmt.Sprintf("%v", swaggerPaths),
			})
			return
		}

		// Add cache-busting headers to ensure swagger UI gets updates
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		http.ServeFile(w, r, swaggerPath)
	})

	a.mux.HandleFunc("/token_status_list/swagger", func(w http.ResponseWriter, r *http.Request) {
		swaggerHTML := `<!DOCTYPE html>
<html>
<head>
    <title>Revocation API</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui.css" />
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui-bundle.js"></script>
    <script>
        SwaggerUIBundle({
            url: '` + a.config.ServiceURL + `token_status_list/swagger/swagger.json',
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
	fmt.Println("Starting Status List Service on :8080")

	// Wrap the mux with CORS middleware
	handler := a.corsMiddleware(a.mux)

	return http.ListenAndServe(":8080", handler)
}

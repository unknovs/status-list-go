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
	"log"

	"azugo.io/azugo"
	azugoconfig "azugo.io/azugo/config"
	"azugo.io/azugo/server"
	"github.com/valyala/fasthttp"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/handlers"
	"github.com/unknovs/status-list-go/routes"
	"github.com/unknovs/status-list-go/services/storage"
)

// App represents the application backed by the Azugo framework.
type App struct {
	cfg           *config.Config
	storage       storage.Storage
	azApp         *azugo.App
	statusHandler *handlers.StatusListHandler
}

const (
	serviceName    = "Status List Service"
	defaultVersion = ""
)

// NewApp creates and configures a new application instance.
func NewApp(cfg *config.Config) *App {
	stor, err := storage.NewStorage(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize storage backend: %v", err)
	}
	log.Printf("Initialized storage backend: %s", cfg.BackendType)

	statusHandler := handlers.NewStatusListHandler(cfg, stor)

	azApp, err := server.New(nil, server.Options{
		AppName:       serviceName,
		AppVer:        defaultVersion,
		Configuration: azugoconfig.New(),
	})
	if err != nil {
		log.Fatalf("Failed to initialize Azugo application: %v", err)
	}

	// Allow all origins by default to preserve existing semantics.
	azApp.RouterOptions().CORS.SetOrigins("*")
	azApp.RouterOptions().CORS.SetHeaders("Origin", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization", "X-API-Key")

	azApp.Use(corsMiddleware)

	if err := routes.Init(azApp, cfg, statusHandler); err != nil {
		log.Fatalf("Failed to register routes: %v", err)
	}

	return &App{
		cfg:           cfg,
		storage:       stor,
		azApp:         azApp,
		statusHandler: statusHandler,
	}
}

// Run starts the Azugo HTTP server.
func (a *App) Run() error {
	return a.azApp.Start()
}

// Azugo returns the underlying Azugo application instance.
func (a *App) Azugo() *azugo.App {
	return a.azApp
}

// Storage exposes the configured storage backend for background tasks.
func (a *App) Storage() storage.Storage {
	return a.storage
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

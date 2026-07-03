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
	"fmt"
	"os"

	"azugo.io/azugo"
	"azugo.io/azugo/server"
	"github.com/gmb-lib/go-platform-kit/platform"
	pkerrors "github.com/gmb-lib/go-platform-kit/errors"
	"github.com/spf13/cobra"

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
func NewApp(cmd *cobra.Command, version string) (*App, error) {
	cfg := config.New()

	azApp, err := server.New(cmd, server.Options{
		AppName:       serviceName,
		AppVer:        version,
		Configuration: cfg,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize azugo: %w", err)
	}

	if err := platform.Setup(azApp, platform.Options{
		Config: cfg.BaseConfiguration,
	}); err != nil {
		return nil, fmt.Errorf("platform setup: %w", err)
	}

	pkerrors.RegisterReason("notAcceptable", pkerrors.ReasonSpec{Status: 406, Title: "Not acceptable"})

	if err := ensureDirs(cfg); err != nil {
		return nil, fmt.Errorf("ensure directories: %w", err)
	}

	stor, err := storage.NewStorage(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize storage backend: %w", err)
	}

	statusHandler := handlers.NewStatusListHandler(cfg, stor)

	azApp.RouterOptions().CORS.SetOrigins("*")
	azApp.RouterOptions().CORS.SetHeaders("Origin", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization", "X-API-Key")

	if err := routes.Init(azApp, cfg, statusHandler); err != nil {
		return nil, fmt.Errorf("register routes: %w", err)
	}

	return &App{
		cfg:           cfg,
		storage:       stor,
		azApp:         azApp,
		statusHandler: statusHandler,
	}, nil
}

func ensureDirs(cfg *config.Config) error {
	for _, dir := range []string{cfg.StatusListDir, cfg.BackupDir, cfg.LogDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return nil
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

// Config returns the application configuration.
func (a *App) Config() *config.Config {
	return a.cfg
}

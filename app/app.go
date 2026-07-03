package app

import (
	"fmt"

	"azugo.io/azugo"
	azugoconfig "azugo.io/azugo/config"
	"azugo.io/azugo/server"

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
func NewApp(cfg *config.Config) (*App, error) {
	stor, err := storage.NewStorage(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize storage backend: %w", err)
	}

	statusHandler := handlers.NewStatusListHandler(cfg, stor)

	azApp, err := server.New(nil, server.Options{
		AppName:       serviceName,
		AppVer:        defaultVersion,
		Configuration: azugoconfig.New(),
	})
	if err != nil {
		return nil, fmt.Errorf("initialize azugo: %w", err)
	}

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

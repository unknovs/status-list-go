package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/unknovs/status-list-go/app"
	"github.com/unknovs/status-list-go/cleanup"
	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/renewal"
)

var version string

func main() {
	rootCmd := newRootCommand()
	rootCmd.Version = version
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status-list",
		Short: "Status List Service",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServer()
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Run the HTTP server",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServer()
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "health",
		Short: "Perform a health check",
		Run: func(_ *cobra.Command, _ []string) {
			performHealthCheck()
		},
	})

	return cmd
}

func runServer() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	application, err := app.NewApp(cfg)
	if err != nil {
		return err
	}

	renewal.StartRenewalThread(cfg, application.Storage())
	cleanup.StartCleanupWorker(cfg, application.Storage())

	return application.Run()
}

func performHealthCheck() {
	serviceURL := os.Getenv("SERVICE_URL")
	if serviceURL == "" {
		serviceURL = "http://localhost:8080"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(serviceURL + "/health")
	if err != nil {
		fmt.Printf("Health check failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Health check failed: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}

	fmt.Println("Health check passed")
	os.Exit(0)
}

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

package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/unknovs/status-list-go/app"
	"github.com/unknovs/status-list-go/cleanup"
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServer(cmd)
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Run the HTTP server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServer(cmd)
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

func runServer(cmd *cobra.Command) error {
	application, err := app.NewApp(cmd, version)
	if err != nil {
		return err
	}

	renewal.StartRenewalThread(application.Config(), application.Storage(), application.Azugo().Log())
	cleanup.StartCleanupWorker(application.Config(), application.Storage(), application.Azugo().Log())

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
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Health check failed: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}

	fmt.Println("Health check passed")
	os.Exit(0)
}

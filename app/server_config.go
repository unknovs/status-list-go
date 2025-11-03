package app

import (
	azugoconfig "azugo.io/azugo/config"
	"azugo.io/core/validation"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// serverConfiguration wraps azugo configuration with project specific bindings.
// It keeps compatibility with legacy PORT environment overrides.
type serverConfiguration struct {
	*azugoconfig.Configuration `mapstructure:",squash"`
}

func newServerConfiguration() *serverConfiguration {
	return &serverConfiguration{Configuration: azugoconfig.New()}
}

// Bind wires configuration defaults and environment variables.
func (c *serverConfiguration) Bind(prefix string, v *viper.Viper) {
	c.Configuration.Bind(prefix, v)

	// Legacy PORT variable for HTTP listener compatibility.
	_ = v.BindEnv("server.http.port", "PORT")
	_ = v.BindEnv("server.http.address", "SERVER_HTTP_ADDRESS")
	_ = v.BindEnv("server.http.enabled", "SERVER_HTTP_ENABLED")
}

// BindCmd ensures CLI flags are propagated to configuration.
func (c *serverConfiguration) BindCmd(cmd *cobra.Command, v *viper.Viper) {
	c.Configuration.BindCmd(cmd, v)
}

// Validate forwards validation to the embedded configuration.
func (c *serverConfiguration) Validate(validate *validation.Validate) error {
	return c.Configuration.Validate(validate)
}

// ServerCore exposes the underlying Azugo configuration instance.
func (c *serverConfiguration) ServerCore() *azugoconfig.Configuration {
	return c.Configuration
}

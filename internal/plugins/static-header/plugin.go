// Package staticheader implements the static-header feature plugin.
// It injects a configurable HTTP header into requests to specified domains
// via the gateway's MITM proxy.
package staticheader

import (
	"fmt"

	"github.com/donbader/agent-sandbox/internal/resolve"
)

// Config defines the typed configuration for the static-header plugin.
type Config struct {
	Domains     []string `yaml:"domains" schema:"Domains to intercept" required:"true"`
	Header      string   `yaml:"header" schema:"Header name to inject" required:"true" examples:"Authorization,X-API-Key"`
	ValueFormat string   `yaml:"value_format" schema:"Header value format (use ${value} for env var substitution)" default:"${value}" examples:"Bearer ${value},token ${value}"`
	Secret      string   `yaml:"secret" schema:"Secret value (use ${VAR} reference)" required:"true" examples:"${MY_API_KEY}"`
}

func init() {
	resolve.Register("static-header", func(_ string, cfg Config) (*resolve.FeatureContributions, error) {
		if cfg.Secret == "" {
			return nil, fmt.Errorf("static-header: missing required option 'secret'")
		}
		envVar, ok := resolve.ExtractEnvVar(cfg.Secret)
		if !ok {
			return nil, fmt.Errorf("static-header: secret must be a ${VAR} reference, got %q", cfg.Secret)
		}

		valueFormat := cfg.ValueFormat
		if valueFormat == "" {
			valueFormat = "${value}"
		}
		return &resolve.FeatureContributions{
			Name:        "static-header",
			MITMDomains: cfg.Domains,
			Rewriters: []resolve.RewriterConfig{
				{
					Type:        "auth-header",
					Domains:     cfg.Domains,
					EnvVar:      envVar,
					Header:      cfg.Header,
					ValueFormat: valueFormat,
				},
			},
		}, nil
	})
}

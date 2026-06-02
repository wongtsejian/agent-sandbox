// Package pi implements the pi feature plugin.
// It installs the Pi CLI and configures MITM for the selected API providers.
package pi

import (
	"fmt"

	"github.com/donbader/agent-sandbox/internal/resolve"
)

// Config defines the typed configuration for the pi plugin.
type Config struct {
	Version      string            `yaml:"version" schema:"Pi version to install" default:"latest"`
	Providers    []string          `yaml:"providers" schema:"API providers to enable MITM for" examples:"anthropic,openai,google" default:"anthropic"`
	ProviderKeys map[string]string `yaml:"provider_keys" schema:"API key per provider (use ${VAR} references)" required:"true" examples:"anthropic: ${ANTHROPIC_API_KEY}"`
}

var domainMap = map[string]string{
	"anthropic": "api.anthropic.com",
	"openai":    "api.openai.com",
	"google":    "generativelanguage.googleapis.com",
}

func init() {
	resolve.Register("pi", func(_ string, cfg Config) (*resolve.FeatureContributions, error) {
		version := cfg.Version
		if version == "" {
			version = "latest"
		}

		providers := cfg.Providers
		if len(providers) == 0 {
			providers = []string{"anthropic"}
		}

		if len(cfg.ProviderKeys) == 0 {
			return nil, fmt.Errorf("pi: missing required option 'provider_keys'")
		}

		var domains []string
		for _, p := range providers {
			if d, ok := domainMap[p]; ok {
				domains = append(domains, d)
			}
			if _, ok := cfg.ProviderKeys[p]; !ok {
				return nil, fmt.Errorf("pi: provider %q listed but no key in provider_keys", p)
			}
			ref := cfg.ProviderKeys[p]
			if _, ok := resolve.ExtractEnvVar(ref); !ok {
				return nil, fmt.Errorf("pi: provider_keys[%s] must be a ${VAR} reference, got %q", p, ref)
			}
		}

		return &resolve.FeatureContributions{
			Name: "pi",
			Commands: []string{
				"apt-get update && apt-get install -y --no-install-recommends nodejs npm git",
				fmt.Sprintf("npm install -g @earendil-works/pi-coding-agent@%s pi-acp@latest", version),
			},
			MITMDomains: domains,
		}, nil
	})
}

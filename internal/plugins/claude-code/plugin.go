// Package claudecode implements the claude-code feature plugin.
// It installs the Claude Code CLI and configures MITM for api.anthropic.com.
package claudecode

import (
	"fmt"

	"github.com/donbader/agent-sandbox/internal/resolve"
)

// Config defines the typed configuration for the claude-code plugin.
type Config struct {
	APIKey  string `yaml:"api_key" schema:"Anthropic API key (use ${VAR} reference)" required:"true" examples:"${ANTHROPIC_API_KEY}"`
	Version string `yaml:"version" schema:"Claude Code version to install" default:"latest" examples:"latest,0.2.0"`
}

func init() {
	resolve.Register("claude-code", func(_ string, cfg Config) (*resolve.FeatureContributions, error) {
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("claude-code: missing required option 'api_key'")
		}
		if _, ok := resolve.ExtractEnvVar(cfg.APIKey); !ok {
			return nil, fmt.Errorf("claude-code: api_key must be a ${VAR} reference, got %q", cfg.APIKey)
		}

		version := cfg.Version
		if version == "" {
			version = "latest"
		}
		return &resolve.FeatureContributions{
			Name: "claude-code",
			Commands: []string{
				"apt-get update && apt-get install -y --no-install-recommends nodejs npm",
				fmt.Sprintf("npm install -g @anthropic-ai/claude-code@%s @agentclientprotocol/claude-agent-acp@latest", version),
			},
			MITMDomains: []string{"api.anthropic.com"},
		}, nil
	})
}

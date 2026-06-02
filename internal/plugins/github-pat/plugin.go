// Package githubpat implements the github-pat feature plugin.
// It injects a GitHub personal access token into requests to GitHub API domains
// via the gateway's MITM proxy.
package githubpat

import (
	"fmt"

	"github.com/donbader/agent-sandbox/internal/resolve"
)

// Config defines the typed configuration for the github-pat plugin.
type Config struct {
	Token   string   `yaml:"token" schema:"GitHub PAT (use ${VAR} reference)" required:"true" examples:"${GITHUB_PAT}"`
	Domains []string `yaml:"domains" schema:"GitHub API domains to intercept" default:"api.github.com,github.com"`
}

func init() {
	resolve.Register("github-pat", func(_ string, cfg Config) (*resolve.FeatureContributions, error) {
		if cfg.Token == "" {
			return nil, fmt.Errorf("github-pat: missing required option 'token'")
		}
		envVar, ok := resolve.ExtractEnvVar(cfg.Token)
		if !ok {
			return nil, fmt.Errorf("github-pat: token must be a ${VAR} reference, got %q", cfg.Token)
		}

		domains := cfg.Domains
		if len(domains) == 0 {
			domains = []string{"api.github.com", "github.com"}
		}
		return &resolve.FeatureContributions{
			Name:        "github-pat",
			MITMDomains: domains,
			AgentEnv:    []string{"GH_TOKEN=dummy", "GITHUB_TOKEN=dummy"},
			Rewriters: []resolve.RewriterConfig{
				{
					Type:        "auth-header",
					Domains:     domains,
					EnvVar:      envVar,
					Header:      "Authorization",
					ValueFormat: "Basic ${base64_basic}",
				},
			},
		}, nil
	})
}

// Package externalservices implements the external-services feature plugin.
// It exposes pre-existing Docker containers to the agent via the gateway's network.
package externalservices

import (
	"fmt"

	"github.com/donbader/agent-sandbox/internal/resolve"
)

// ServiceConfig defines a single external service to connect.
type ServiceConfig struct {
	Name    string `yaml:"name" schema:"Label and Docker DNS hostname for the service" required:"true"`
	Network string `yaml:"network" schema:"External Docker network the service is on" required:"true"`
}

// Config defines the typed configuration for the external-services plugin.
type Config struct {
	Services []ServiceConfig `yaml:"services" schema:"External services to make reachable" required:"true"`
}

func init() {
	resolve.Register("external-services", func(_ string, cfg Config) (*resolve.FeatureContributions, error) {
		if len(cfg.Services) == 0 {
			return nil, fmt.Errorf("external-services: at least one service is required")
		}

		var networks []string
		seen := map[string]bool{}
		for _, svc := range cfg.Services {
			if svc.Name == "" {
				return nil, fmt.Errorf("external-services: service name is required")
			}
			if svc.Network == "" {
				return nil, fmt.Errorf("external-services: network is required for service %q", svc.Name)
			}
			if !seen[svc.Network] {
				seen[svc.Network] = true
				networks = append(networks, svc.Network)
			}
		}

		return &resolve.FeatureContributions{
			ExternalNetworks: networks,
		}, nil
	})
}

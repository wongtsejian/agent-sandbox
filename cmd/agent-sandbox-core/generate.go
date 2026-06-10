package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/donbader/agent-sandbox/internal/config"
	"github.com/donbader/agent-sandbox/internal/dotenv"
	v1 "github.com/donbader/agent-sandbox/internal/generate/v1"
	"github.com/spf13/cobra"
)

func generateCmd(dir *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate build artifacts from agent.yaml or fleet.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, err := filepath.Abs(*dir)
			if err != nil {
				return fmt.Errorf("resolve dir: %w", err)
			}

			// Use auto-detected coreRoot from binary location.
			coreDir := coreRoot
			if coreDir == "." {
				fmt.Fprintf(os.Stderr, "Warning: could not detect core root from binary location.\n")
			}

			// Load .env file so secrets are available for auth-header baking.
			dotenv.Load(filepath.Join(projectDir, ".env"))

			// Try single-agent first, then fleet mode
			cfg, loadErr := config.Load(projectDir)
			if loadErr == nil {
				return generateSingleAgent(cfg, projectDir, coreDir)
			}

			// Try fleet mode
			_, agents, fleetErr := config.LoadFleetAgents(projectDir)
			if fleetErr != nil {
				return fmt.Errorf("cannot load agent.yaml or fleet.yaml:\n  agent: %w\n  fleet: %v", loadErr, fleetErr)
			}

			return generateFleet(agents, projectDir, coreDir)
		},
	}

	return cmd
}

func generateSingleAgent(cfg *config.Config, projectDir, coreDir string) error {
	g := v1.NewGeneratorWithCore(projectDir, coreDir)
	if err := g.RunWithConfig(cfg, projectDir); err != nil {
		return err
	}

	_ = ensureSchemaComment(filepath.Join(projectDir, "agent.yaml"), ".build/schema.json")
	fmt.Fprintf(os.Stderr, "Generated .build/ in %s\n", projectDir)
	return nil
}

func generateFleet(agents []config.FleetAgent, projectDir, coreDir string) error {
	g := v1.NewGeneratorWithCore(projectDir, coreDir)
	if err := g.RunFleet(agents); err != nil {
		return err
	}

	_ = ensureSchemaComment(filepath.Join(projectDir, "fleet.yaml"), ".build/fleet-schema.json")
	for _, agent := range agents {
		agentYAML := filepath.Join(agent.Dir, "agent.yaml")
		relSchema, err := filepath.Rel(agent.Dir, filepath.Join(projectDir, ".build", "schema.json"))
		if err != nil {
			relSchema = ".build/schema.json"
		}
		_ = ensureSchemaComment(agentYAML, relSchema)
	}

	fmt.Fprintf(os.Stderr, "Generated .build/ for %d agents in %s\n", len(agents), projectDir)
	return nil
}

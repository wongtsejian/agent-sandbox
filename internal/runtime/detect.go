// Package runtime detects the container runtime available on the host.
package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Runtime identifies a container runtime engine.
type Runtime string

const (
	Docker Runtime = "docker"
	Podman Runtime = "podman"
)

// Detected holds the result of container runtime detection.
type Detected struct {
	Runtime    Runtime
	Binary     string
	ComposeCmd []string
}

// Detect probes the system for an available container runtime.
// It checks the CONTAINER_RUNTIME env var first, then probes PATH
// (podman preferred over docker). Returns an error if no runtime is found.
func Detect() (*Detected, error) {
	if envVal := os.Getenv("CONTAINER_RUNTIME"); envVal != "" {
		return detectFromEnv(envVal)
	}
	return detectFromPath()
}

// DetectOrDefault returns the detected runtime, falling back to Docker
// defaults if detection fails. Useful for non-critical paths like tests.
func DetectOrDefault() *Detected {
	d, err := Detect()
	if err != nil {
		return &Detected{
			Runtime:    Docker,
			Binary:     "docker",
			ComposeCmd: []string{"docker", "compose"},
		}
	}
	return d
}

func detectFromEnv(val string) (*Detected, error) {
	switch Runtime(val) {
	case Docker:
		if _, err := exec.LookPath("docker"); err != nil {
			return nil, fmt.Errorf("CONTAINER_RUNTIME set to %q but binary not found on PATH", val)
		}
		return buildDetected(Docker)
	case Podman:
		if _, err := exec.LookPath("podman"); err != nil {
			return nil, fmt.Errorf("CONTAINER_RUNTIME set to %q but binary not found on PATH", val)
		}
		return buildDetected(Podman)
	default:
		return nil, fmt.Errorf("unsupported CONTAINER_RUNTIME value %q: must be \"docker\" or \"podman\"", val)
	}
}

func detectFromPath() (*Detected, error) {
	if _, err := exec.LookPath("podman"); err == nil {
		return buildDetected(Podman)
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return buildDetected(Docker)
	}
	return nil, fmt.Errorf("no container runtime found: install docker or podman and ensure it is on PATH")
}

func buildDetected(rt Runtime) (*Detected, error) {
	binary := string(rt)
	composeCmd := []string{binary, "compose"}

	if rt == Podman {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "podman", "compose", "version").Run(); err != nil {
			if _, err2 := exec.LookPath("podman-compose"); err2 == nil {
				composeCmd = []string{"podman-compose"}
			} else {
				return nil, fmt.Errorf("podman found but no compose capability: install podman-compose or the podman compose plugin")
			}
		}
	}

	return &Detected{
		Runtime:    rt,
		Binary:     binary,
		ComposeCmd: composeCmd,
	}, nil
}

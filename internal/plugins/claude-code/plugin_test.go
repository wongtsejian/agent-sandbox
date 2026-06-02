package claudecode

import (
	"strings"
	"testing"

	"github.com/donbader/agent-sandbox/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	t.Run("defaults version to latest", func(t *testing.T) {
		contrib, err := resolve.ResolveFeature("/project", "claude-code", "claude-code", map[string]any{
			"api_key": "${ANTHROPIC_API_KEY}",
		})
		require.NoError(t, err)
		assert.True(t, containsInstallCmd(contrib.Commands, "@anthropic-ai/claude-code@latest"),
			"expected install command with @latest, got: %v", contrib.Commands)
	})

	t.Run("uses specified version", func(t *testing.T) {
		contrib, err := resolve.ResolveFeature("/project", "claude-code", "claude-code", map[string]any{
			"api_key": "${ANTHROPIC_API_KEY}",
			"version": "0.2.0",
		})
		require.NoError(t, err)
		assert.True(t, containsInstallCmd(contrib.Commands, "@anthropic-ai/claude-code@0.2.0"),
			"expected install command with @0.2.0, got: %v", contrib.Commands)
	})

	t.Run("sets MITM domain", func(t *testing.T) {
		contrib, err := resolve.ResolveFeature("/project", "claude-code", "claude-code", map[string]any{
			"api_key": "${ANTHROPIC_API_KEY}",
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"api.anthropic.com"}, contrib.MITMDomains)
	})

	t.Run("errors without api_key", func(t *testing.T) {
		_, err := resolve.ResolveFeature("/project", "claude-code", "claude-code", map[string]any{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required option 'api_key'")
	})

	t.Run("errors with literal api_key", func(t *testing.T) {
		_, err := resolve.ResolveFeature("/project", "claude-code", "claude-code", map[string]any{
			"api_key": "sk-ant-1234",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a ${VAR} reference")
	})
}

// containsInstallCmd reports whether any command in cmds contains substr.
func containsInstallCmd(cmds []string, substr string) bool {
	for _, c := range cmds {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

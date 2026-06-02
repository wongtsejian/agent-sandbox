package pi

import (
	"strings"
	"testing"

	"github.com/donbader/agent-sandbox/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	t.Run("defaults to anthropic provider", func(t *testing.T) {
		contrib, err := resolve.ResolveFeature("/project", "pi", "pi", map[string]any{
			"provider_keys": map[string]any{
				"anthropic": "${ANTHROPIC_API_KEY}",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"api.anthropic.com"}, contrib.MITMDomains)
	})

	t.Run("single custom provider", func(t *testing.T) {
		contrib, err := resolve.ResolveFeature("/project", "pi", "pi", map[string]any{
			"providers": []any{"openai"},
			"provider_keys": map[string]any{
				"openai": "${OPENAI_API_KEY}",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"api.openai.com"}, contrib.MITMDomains)
	})

	t.Run("multiple providers", func(t *testing.T) {
		contrib, err := resolve.ResolveFeature("/project", "pi", "pi", map[string]any{
			"providers": []any{"anthropic", "openai", "google"},
			"provider_keys": map[string]any{
				"anthropic": "${ANTHROPIC_API_KEY}",
				"openai":    "${OPENAI_API_KEY}",
				"google":    "${GOOGLE_API_KEY}",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"api.anthropic.com", "api.openai.com", "generativelanguage.googleapis.com"}, contrib.MITMDomains)
	})

	t.Run("errors when provider missing from provider_keys", func(t *testing.T) {
		_, err := resolve.ResolveFeature("/project", "pi", "pi", map[string]any{
			"providers": []any{"anthropic", "openai"},
			"provider_keys": map[string]any{
				"anthropic": "${ANTHROPIC_API_KEY}",
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no key in provider_keys")
	})

	t.Run("errors without provider_keys", func(t *testing.T) {
		_, err := resolve.ResolveFeature("/project", "pi", "pi", map[string]any{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required option 'provider_keys'")
	})

	t.Run("errors with literal key value", func(t *testing.T) {
		_, err := resolve.ResolveFeature("/project", "pi", "pi", map[string]any{
			"provider_keys": map[string]any{
				"anthropic": "sk-ant-literal",
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a ${VAR} reference")
	})

	t.Run("defaults version to latest in install command", func(t *testing.T) {
		contrib, err := resolve.ResolveFeature("/project", "pi", "pi", map[string]any{
			"provider_keys": map[string]any{
				"anthropic": "${ANTHROPIC_API_KEY}",
			},
		})
		require.NoError(t, err)
		assert.True(t, containsInstallCmd(contrib.Commands, "@earendil-works/pi-coding-agent@latest"),
			"expected install command with @latest, got: %v", contrib.Commands)
	})

	t.Run("uses specified version in install command", func(t *testing.T) {
		contrib, err := resolve.ResolveFeature("/project", "pi", "pi", map[string]any{
			"version": "1.2.3",
			"provider_keys": map[string]any{
				"anthropic": "${ANTHROPIC_API_KEY}",
			},
		})
		require.NoError(t, err)
		assert.True(t, containsInstallCmd(contrib.Commands, "@earendil-works/pi-coding-agent@1.2.3"),
			"expected install command with @1.2.3, got: %v", contrib.Commands)
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

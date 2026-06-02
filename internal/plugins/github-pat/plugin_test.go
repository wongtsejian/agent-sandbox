package githubpat

import (
	"testing"

	"github.com/donbader/agent-sandbox/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubPATPlugin_DefaultDomains(t *testing.T) {
	config := map[string]any{
		"token": "${GITHUB_PAT}",
	}

	plugin := resolve.RegisteredPlugins()["github-pat"]
	require.NotNil(t, plugin, "github-pat plugin not registered")

	contrib, err := plugin.Resolve("", config)
	require.NoError(t, err)

	assert.Equal(t, []string{"api.github.com", "github.com"}, contrib.MITMDomains)
	assert.Equal(t, []string{"GH_TOKEN=dummy", "GITHUB_TOKEN=dummy"}, contrib.AgentEnv)

	require.Len(t, contrib.Rewriters, 1)
	rw := contrib.Rewriters[0]
	assert.Equal(t, "auth-header", rw.Type)
	assert.Equal(t, "Authorization", rw.Header)
	assert.Equal(t, "Basic ${base64_basic}", rw.ValueFormat)
	assert.Equal(t, "GITHUB_PAT", rw.EnvVar)
}

func TestGitHubPATPlugin_CustomDomains(t *testing.T) {
	config := map[string]any{
		"token":   "${GITHUB_PAT}",
		"domains": []any{"api.github.com"},
	}

	plugin := resolve.RegisteredPlugins()["github-pat"]
	require.NotNil(t, plugin, "github-pat plugin not registered")

	contrib, err := plugin.Resolve("", config)
	require.NoError(t, err)

	assert.Equal(t, []string{"api.github.com"}, contrib.MITMDomains)
	require.Len(t, contrib.Rewriters, 1)
	assert.Equal(t, []string{"api.github.com"}, contrib.Rewriters[0].Domains)
}

func TestGitHubPATPlugin_ErrorsWithoutToken(t *testing.T) {
	plugin := resolve.RegisteredPlugins()["github-pat"]
	require.NotNil(t, plugin, "github-pat plugin not registered")

	_, err := plugin.Resolve("", map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required option 'token'")
}

func TestGitHubPATPlugin_ErrorsWithLiteralToken(t *testing.T) {
	plugin := resolve.RegisteredPlugins()["github-pat"]
	require.NotNil(t, plugin, "github-pat plugin not registered")

	_, err := plugin.Resolve("", map[string]any{
		"token": "ghp_1234567890",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a ${VAR} reference")
}

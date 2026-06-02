package staticheader

import (
	"testing"

	"github.com/donbader/agent-sandbox/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticHeaderPlugin_Resolve(t *testing.T) {
	plugin := resolve.RegisteredPlugins()["static-header"]
	require.NotNil(t, plugin, "static-header plugin not registered")

	contrib, err := plugin.Resolve("", map[string]any{
		"domains":      []any{"api.example.com"},
		"header":       "X-API-Key",
		"value_format": "Bearer ${value}",
		"secret":       "${EXAMPLE_API_KEY}",
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"api.example.com"}, contrib.MITMDomains)

	require.Len(t, contrib.Rewriters, 1)
	rw := contrib.Rewriters[0]
	assert.Equal(t, "auth-header", rw.Type)
	assert.Equal(t, "X-API-Key", rw.Header)
	assert.Equal(t, "Bearer ${value}", rw.ValueFormat)
	assert.Equal(t, "EXAMPLE_API_KEY", rw.EnvVar)
}

func TestStaticHeaderPlugin_DefaultValueFormat(t *testing.T) {
	plugin := resolve.RegisteredPlugins()["static-header"]
	require.NotNil(t, plugin, "static-header plugin not registered")

	contrib, err := plugin.Resolve("", map[string]any{
		"domains": []any{"api.example.com"},
		"header":  "X-API-Key",
		"secret":  "${EXAMPLE_API_KEY}",
	})
	require.NoError(t, err)

	require.Len(t, contrib.Rewriters, 1)
	assert.Equal(t, "${value}", contrib.Rewriters[0].ValueFormat)
}

func TestStaticHeaderPlugin_ErrorsWithoutSecret(t *testing.T) {
	plugin := resolve.RegisteredPlugins()["static-header"]
	require.NotNil(t, plugin, "static-header plugin not registered")

	_, err := plugin.Resolve("", map[string]any{
		"domains": []any{"api.example.com"},
		"header":  "X-API-Key",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required option 'secret'")
}

func TestStaticHeaderPlugin_ErrorsWithLiteralSecret(t *testing.T) {
	plugin := resolve.RegisteredPlugins()["static-header"]
	require.NotNil(t, plugin, "static-header plugin not registered")

	_, err := plugin.Resolve("", map[string]any{
		"domains": []any{"api.example.com"},
		"header":  "X-API-Key",
		"secret":  "literal-secret-value",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a ${VAR} reference")
}

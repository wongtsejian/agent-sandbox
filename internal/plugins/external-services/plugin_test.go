package externalservices

import (
	"testing"

	"github.com/donbader/agent-sandbox/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalServices_ValidConfig(t *testing.T) {
	contrib, err := resolve.ResolveFeature(".", "external-services", "external-services", map[string]any{
		"services": []any{
			map[string]any{"name": "rkgw", "network": "rkgw-external"},
			map[string]any{"name": "postgres", "network": "my-db-net"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"rkgw-external", "my-db-net"}, contrib.ExternalNetworks)
}

func TestExternalServices_DeduplicatesNetworks(t *testing.T) {
	contrib, err := resolve.ResolveFeature(".", "external-services", "external-services", map[string]any{
		"services": []any{
			map[string]any{"name": "svc1", "network": "shared-net"},
			map[string]any{"name": "svc2", "network": "shared-net"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared-net"}, contrib.ExternalNetworks)
}

func TestExternalServices_EmptyServicesError(t *testing.T) {
	_, err := resolve.ResolveFeature(".", "external-services", "external-services", map[string]any{
		"services": []any{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one service")
}

func TestExternalServices_MissingNameError(t *testing.T) {
	_, err := resolve.ResolveFeature(".", "external-services", "external-services", map[string]any{
		"services": []any{
			map[string]any{"network": "some-net"},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestExternalServices_MissingNetworkError(t *testing.T) {
	_, err := resolve.ResolveFeature(".", "external-services", "external-services", map[string]any{
		"services": []any{
			map[string]any{"name": "svc"},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "network is required")
}

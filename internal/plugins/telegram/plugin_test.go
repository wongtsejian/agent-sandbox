package telegram

import (
	"testing"

	"github.com/donbader/agent-sandbox/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	t.Run("returns expected contributions", func(t *testing.T) {
		config := map[string]any{
			"bot_token": "${TELEGRAM_BOT_TOKEN}",
			"access_control": map[string]any{
				"allowed_users":   []any{"@coreyortea"},
				"require_mention": false,
			},
		}

		contrib, err := resolve.ResolveFeature("/project", "telegram", "telegram", config)
		require.NoError(t, err)
		assert.Equal(t, []string{"api.telegram.org"}, contrib.MITMDomains)
		assert.Equal(t, "telegram", contrib.ChannelName)
		assert.Equal(t, "TELEGRAM_BOT_TOKEN", contrib.Rewriters[0].EnvVar)
	})

	t.Run("errors without bot_token", func(t *testing.T) {
		config := map[string]any{
			"access_control": map[string]any{
				"allowed_users": []any{"@coreyortea"},
			},
		}

		_, err := resolve.ResolveFeature("/project", "telegram", "telegram", config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required option 'bot_token'")
	})

	t.Run("errors with non-ref bot_token", func(t *testing.T) {
		config := map[string]any{
			"bot_token": "literal-token-value",
			"access_control": map[string]any{
				"allowed_users": []any{"@coreyortea"},
			},
		}

		_, err := resolve.ResolveFeature("/project", "telegram", "telegram", config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a ${VAR} reference")
	})
}

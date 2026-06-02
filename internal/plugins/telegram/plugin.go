// Package telegram implements the Telegram feature plugin.
// It provides a Telegram bot channel via the channel manager, with MITM token injection
// through the gateway.
package telegram

import (
	"fmt"

	"github.com/donbader/agent-sandbox/internal/resolve"
)

// Config defines the typed configuration for the telegram plugin.
type Config struct {
	BotToken      string       `yaml:"bot_token" schema:"Telegram bot token (use ${VAR} reference)" required:"true" examples:"${TELEGRAM_BOT_TOKEN}"`
	AccessControl AccessControl `yaml:"access_control" schema:"Access control settings for the Telegram bot" required:"true"`
	AckEmoji      *string       `yaml:"ack_emoji" schema:"Emoji reaction to acknowledge received messages" default:"👀"`
}

// AccessControl defines who can interact with the bot.
type AccessControl struct {
	AllowedUsers   []string            `yaml:"allowed_users" schema:"Telegram usernames allowed to interact" examples:"@user1,@user2" pattern:"^@"`
	RequireMention bool                `yaml:"require_mention" schema:"Whether the bot requires @mention in group chats" default:"false"`
	Groups         map[string]GroupACL `yaml:"groups" schema:"Per-group access control overrides (key: chat ID)"`
}

// GroupACL defines access control for a specific group chat.
type GroupACL struct {
	AllowedUsers   []string `yaml:"allowed_users" schema:"Users allowed in this group (overrides top-level)" examples:"@user1,@user2"`
	RequireMention *bool    `yaml:"require_mention" schema:"Override require_mention for this group"`
}

func init() {
	resolve.Register("telegram", func(_ string, cfg Config) (*resolve.FeatureContributions, error) {
		if cfg.BotToken == "" {
			return nil, fmt.Errorf("telegram: missing required option 'bot_token'")
		}
		envVar, ok := resolve.ExtractEnvVar(cfg.BotToken)
		if !ok {
			return nil, fmt.Errorf("telegram: bot_token must be a ${VAR} reference, got %q", cfg.BotToken)
		}

		channelConfig := map[string]any{"access_control": cfg.AccessControl}
		if cfg.AckEmoji != nil {
			channelConfig["ack_emoji"] = *cfg.AckEmoji
		}
		return &resolve.FeatureContributions{
			Name:        "telegram",
			MITMDomains: []string{"api.telegram.org"},
			ChannelName: "telegram",
			ChannelConfig: channelConfig,
			Rewriters: []resolve.RewriterConfig{
				{
					Type:    "telegram-url",
					Domains: []string{"api.telegram.org"},
					EnvVar:  envVar,
				},
			},
		}, nil
	})
}

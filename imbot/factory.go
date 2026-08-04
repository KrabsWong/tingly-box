package imbot

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform"
)

// CreateBot creates a bot instance based on the configuration
func CreateBot(config *core.Config) (core.Bot, error) {
	// Validate config
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Expand environment variables
	config.ExpandEnvVars()

	// Create bot using platform registry
	return platform.Create(config)
}

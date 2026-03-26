package config

import "github.com/kelseyhightower/envconfig"

// Config holds application-wide settings loaded from environment variables.
// Env vars use the PENSA_ prefix for discoverability.
type Config struct {
	ConcurrentDownloads int    `envconfig:"PENSA_CONCURRENT_DOWNLOADS" default:"50"`
	Verbose             bool   `envconfig:"PENSA_VERBOSE"`
	Quiet               bool   `envconfig:"PENSA_QUIET"`
	Color               string `envconfig:"PENSA_COLOR" default:"auto"` // auto, always, never
}

// New loads config from environment variables with defaults.
func New() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

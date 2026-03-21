// Package config loads dep-health runtime configuration from environment variables.
package config

import (
	"os"
	"strconv"
)

// Config holds all runtime settings for dep-health.
type Config struct {
	AnthropicAPIKey string
	GitHubToken     string
	OrgName         string
	MaxConcurrency  int
	DBPath          string
}

// Load reads configuration from environment variables, applying sensible defaults.
func Load() *Config {
	maxConc := 10
	if raw := os.Getenv("DEP_HEALTH_MAX_CONCURRENCY"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			maxConc = n
		}
	}
	dbPath := os.Getenv("DEP_HEALTH_DB")
	if dbPath == "" {
		dbPath = "dep-health.db"
	}
	return &Config{
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		GitHubToken:     os.Getenv("GITHUB_TOKEN"),
		OrgName:         os.Getenv("DEP_HEALTH_ORG"),
		MaxConcurrency:  maxConc,
		DBPath:          dbPath,
	}
}

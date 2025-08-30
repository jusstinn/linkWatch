package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL    string
	CheckInterval  time.Duration
	MaxConcurrency int
	HTTPTimeout    time.Duration
	ShutdownGrace  time.Duration
}

// Default values in one place
const (
	defaultDBURL          = "file:linkwatch.db?_pragma=busy_timeout(5000)"
	defaultCheckInterval  = 15 * time.Second
	defaultMaxConcurrency = 8
	defaultHTTPTimeout    = 5 * time.Second
	defaultShutdownGrace  = 10 * time.Second
)

// Load reads config values from environment with fallbacks.
func Load() (*Config, error) {
	cfg := &Config{}

	cfg.DatabaseURL = getEnvString("DATABASE_URL", defaultDBURL)

	var err error
	if cfg.CheckInterval, err = getEnvDuration("CHECK_INTERVAL", defaultCheckInterval); err != nil {
		return nil, fmt.Errorf("invalid CHECK_INTERVAL: %w", err)
	}

	if cfg.MaxConcurrency, err = getEnvInt("MAX_CONCURRENCY", defaultMaxConcurrency); err != nil {
		return nil, fmt.Errorf("invalid MAX_CONCURRENCY: %w", err)
	}

	if cfg.HTTPTimeout, err = getEnvDuration("HTTP_TIMEOUT", defaultHTTPTimeout); err != nil {
		return nil, fmt.Errorf("invalid HTTP_TIMEOUT: %w", err)
	}

	if cfg.ShutdownGrace, err = getEnvDuration("SHUTDOWN_GRACE", defaultShutdownGrace); err != nil {
		return nil, fmt.Errorf("invalid SHUTDOWN_GRACE: %w", err)
	}

	return cfg, nil
}

// --- Helper functions ---

func getEnvString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	if v := os.Getenv(key); v != "" {
		return time.ParseDuration(v)
	}
	return fallback, nil
}

func getEnvInt(key string, fallback int) (int, error) {
	if v := os.Getenv(key); v != "" {
		i, err := strconv.Atoi(v)
		if err != nil || i <= 0 {
			return 0, fmt.Errorf("must be positive integer")
		}
		return i, nil
	}
	return fallback, nil
}

func (c *Config) String() string {
	return fmt.Sprintf(
		"Config{DatabaseURL: %s, CheckInterval: %v, MaxConcurrency: %d, HTTPTimeout: %v, ShutdownGrace: %v}",
		c.DatabaseURL, c.CheckInterval, c.MaxConcurrency, c.HTTPTimeout, c.ShutdownGrace,
	)
}

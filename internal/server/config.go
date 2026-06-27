package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for Cerebro.
type Config struct {
	CerebrasKeys           []string       `yaml:"cerebras_keys"`
	Server                 ServerConfig   `yaml:"server"`
	DefaultCooldownSeconds int            `yaml:"default_cooldown_seconds"`
	Tenants                []TenantConfig `yaml:"tenants"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port     int    `yaml:"port"`
	Upstream string `yaml:"upstream"`
}

// TenantConfig defines a single tenant with a name and API key (bearer token).
type TenantConfig struct {
	Name   string `yaml:"name"`
	APIKey string `yaml:"api_key"`
}

// LoadConfig reads configuration from a YAML file and applies environment
// variable overrides. Environment variables take precedence over YAML values.
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:     8080,
			Upstream: "https://api.cerebras.ai",
		},
		DefaultCooldownSeconds: 60,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		// Config file is optional if env vars provide everything.
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
	}

	// Environment variable overrides.
	if keys := os.Getenv("CEREBRAS_API_KEYS"); keys != "" {
		cfg.CerebrasKeys = splitAndTrim(keys, ",")
	}

	if port := os.Getenv("CEREBRO_PORT"); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid CEREBRO_PORT: %w", err)
		}
		cfg.Server.Port = p
	}

	if upstream := os.Getenv("CEREBRO_UPSTREAM"); upstream != "" {
		cfg.Server.Upstream = upstream
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// Validate checks the configuration for errors and enforces invariants.
func (c *Config) Validate() error {
	if len(c.CerebrasKeys) == 0 {
		return fmt.Errorf("at least one Cerebras API key is required (set cerebras_keys in config or CEREBRAS_API_KEYS env var)")
	}

	if len(c.Tenants) == 0 {
		return fmt.Errorf("at least one tenant is required")
	}

	seen := make(map[string]string) // api_key -> tenant name
	for _, t := range c.Tenants {
		if t.Name == "" {
			return fmt.Errorf("tenant name cannot be empty")
		}
		if t.APIKey == "" {
			return fmt.Errorf("tenant %q has an empty api_key", t.Name)
		}
		if existing, ok := seen[t.APIKey]; ok {
			return fmt.Errorf("duplicate tenant api_key: tenants %q and %q share the same key", existing, t.Name)
		}
		seen[t.APIKey] = t.Name
	}

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535, got %d", c.Server.Port)
	}

	if c.Server.Upstream == "" {
		return fmt.Errorf("server upstream URL cannot be empty")
	}

	if c.DefaultCooldownSeconds < 1 {
		return fmt.Errorf("default_cooldown_seconds must be >= 1, got %d", c.DefaultCooldownSeconds)
	}

	return nil
}

// splitAndTrim splits a string by a separator and trims whitespace from each part,
// filtering out empty strings.
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

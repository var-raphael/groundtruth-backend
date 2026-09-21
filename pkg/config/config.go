package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all environment-driven settings for the service.
type Config struct {
	DatabaseURL string

	// MistralAPIKeys holds one or more Mistral API keys, parsed from a
	// comma-separated MISTRAL_API_KEY env var. Having multiple keys lets
	// the worker pin separate keys to separate concurrent LLM workers
	// rather than funneling all scoring requests through a single key.
	MistralAPIKeys []string

	GithubToken string
	Port        string
}

// Load reads a .env file if present (ignored if missing, e.g. in production
// where env vars are injected directly) and returns a populated Config.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	_ = godotenv.Overload()

	cfg := &Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		MistralAPIKeys: parseCommaSeparated(os.Getenv("MISTRAL_API_KEY")),
		GithubToken:    os.Getenv("GITHUB_TOKEN"),
		Port:           os.Getenv("PORT"),
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if len(cfg.MistralAPIKeys) == 0 {
		missing = append(missing, "MISTRAL_API_KEY")
	}
	if cfg.GithubToken == "" {
		missing = append(missing, "GITHUB_TOKEN")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %v", missing)
	}

	return cfg, nil
}

func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

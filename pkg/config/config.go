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

	GithubToken     string
	Port            string
	DashboardSecret string

	TokenEncryptionKey string
	SupabaseURL        string
	AdminEmails        []string

	PaystackSecretKey   string
	PaystackProPlanCode string
	AppURL              string
}

// Load reads a .env file if present (ignored if missing, e.g. in production
// where env vars are injected directly) and returns a populated Config.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	_ = godotenv.Overload()

	cfg := &Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		MistralAPIKeys: parseCommaSeparated(os.Getenv("MISTRAL_API_KEY")),
		GithubToken:     os.Getenv("GITHUB_TOKEN"),
		Port:            os.Getenv("PORT"),
		DashboardSecret: os.Getenv("DASHBOARD_SECRET"),

		TokenEncryptionKey: os.Getenv("TOKEN_ENCRYPTION_KEY"),
		SupabaseURL:        os.Getenv("SUPABASE_URL"),
		AdminEmails:        splitList(os.Getenv("ADMIN_EMAILS")),

		PaystackSecretKey:   os.Getenv("PAYSTACK_SECRET_KEY"),
		PaystackProPlanCode: os.Getenv("PAYSTACK_PRO_PLAN_CODE"),
		AppURL:              os.Getenv("APP_URL"),
	}
	if cfg.AppURL == "" {
		cfg.AppURL = "http://localhost:3000"
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
	if cfg.DashboardSecret == "" {
		missing = append(missing, "DASHBOARD_SECRET")
	}
	if cfg.TokenEncryptionKey == "" {
		missing = append(missing, "TOKEN_ENCRYPTION_KEY")
	}
	if cfg.SupabaseURL == "" {
		missing = append(missing, "SUPABASE_URL")
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

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

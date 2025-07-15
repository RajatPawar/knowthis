package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port              string
	DatabaseURL       string
	SlackBotToken     string
	SlackAppToken     string
	SlabWebhookSecret string
	OpenAIAPIKey      string
	LogLevel          string
	LogFormat         string
	Environment       string
}

func Load() *Config {
	// Determine default database URL based on environment
	var defaultDatabaseURL string
	env := getEnvOrDefault("ENVIRONMENT", "development")

	if env == "production" {
		// For production environments like Railway, try SSL first, fall back to disable if needed
		defaultDatabaseURL = "postgres://localhost/knowthis?sslmode=require"
	} else {
		// For development, disable SSL by default
		defaultDatabaseURL = "postgres://localhost/knowthis?sslmode=disable"
	}

	return &Config{
		Port:              getEnvOrDefault("PORT", "8080"),
		DatabaseURL:       getEnvOrDefault("DATABASE_URL", defaultDatabaseURL),
		SlackBotToken:     os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken:     os.Getenv("SLACK_APP_TOKEN"),
		SlabWebhookSecret: os.Getenv("SLAB_WEBHOOK_SECRET"),
		OpenAIAPIKey:      os.Getenv("OPENAI_API_KEY"),
		LogLevel:          getEnvOrDefault("LOG_LEVEL", "INFO"),
		LogFormat:         getEnvOrDefault("LOG_FORMAT", "text"),
		Environment:       env,
	}
}

func (c *Config) Validate() error {
	var errors []string

	if c.SlackBotToken == "" {
		errors = append(errors, "SLACK_BOT_TOKEN is required")
	}

	if c.SlackAppToken == "" {
		errors = append(errors, "SLACK_APP_TOKEN is required")
	}

	if c.OpenAIAPIKey == "" {
		errors = append(errors, "OPENAI_API_KEY is required")
	}

	if c.DatabaseURL == "" {
		errors = append(errors, "DATABASE_URL is required")
	}

	// Optional validations
	if c.SlackBotToken != "" && !strings.HasPrefix(c.SlackBotToken, "xoxb-") {
		errors = append(errors, "SLACK_BOT_TOKEN must start with 'xoxb-'")
	}

	if c.SlackAppToken != "" && !strings.HasPrefix(c.SlackAppToken, "xapp-") {
		errors = append(errors, "SLACK_APP_TOKEN must start with 'xapp-'")
	}

	validLogLevels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	if !contains(validLogLevels, strings.ToUpper(c.LogLevel)) {
		errors = append(errors, "LOG_LEVEL must be one of: DEBUG, INFO, WARN, ERROR")
	}

	validLogFormats := []string{"text", "json"}
	if !contains(validLogFormats, strings.ToLower(c.LogFormat)) {
		errors = append(errors, "LOG_FORMAT must be one of: text, json")
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", errors[0])
	}

	return nil
}

func (c *Config) IsProduction() bool {
	return strings.ToLower(c.Environment) == "production"
}

func (c *Config) IsDevelopment() bool {
	return strings.ToLower(c.Environment) == "development"
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

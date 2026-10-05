package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Server
	ServerPort string
	AppEnv     string

	// Database
	DatabaseURL string

	// GitHub OAuth
	GitHubClientID     string
	GitHubClientSecret string
	GitHubCallbackURL  string

	// Telegram
	TelegramBotToken string

	// Security
	JWTSecret     string
	EncryptionKey string // must be exactly 32 bytes for AES-256
}

// Load reads the .env file (if present) and then reads all required
// environment variables. Returns an error if any required variable is missing.
func Load() (*Config, error) {
	// Load .env file in development; ignore error in production (vars set externally)
	_ = godotenv.Load()

	cfg := &Config{
		ServerPort:         getEnvOrDefault("SERVER_PORT", "8080"),
		AppEnv:             getEnvOrDefault("APP_ENV", "development"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		GitHubCallbackURL:  os.Getenv("GITHUB_CALLBACK_URL"),
		TelegramBotToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		EncryptionKey:      os.Getenv("ENCRYPTION_KEY"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate checks that all required fields are populated.
func (c *Config) validate() error {
	required := map[string]string{
		"DATABASE_URL":          c.DatabaseURL,
		"GITHUB_CLIENT_ID":      c.GitHubClientID,
		"GITHUB_CLIENT_SECRET":  c.GitHubClientSecret,
		"GITHUB_CALLBACK_URL":   c.GitHubCallbackURL,
		"TELEGRAM_BOT_TOKEN":    c.TelegramBotToken,
		"JWT_SECRET":            c.JWTSecret,
		"ENCRYPTION_KEY":        c.EncryptionKey,
	}

	for name, val := range required {
		if val == "" {
			return fmt.Errorf("missing required environment variable: %s", name)
		}
	}

	if len(c.EncryptionKey) != 32 {
		return fmt.Errorf("ENCRYPTION_KEY must be exactly 32 characters (got %d)", len(c.EncryptionKey))
	}

	return nil
}

// IsDevelopment returns true when running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.AppEnv == "development"
}

// Port returns the server port as an integer.
func (c *Config) Port() int {
	p, err := strconv.Atoi(c.ServerPort)
	if err != nil {
		return 8080
	}
	return p
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// LoadEnv loads environment variables from the .env file.
// Existing environment variables are not overwritten.
func LoadEnv() error {
	if err := godotenv.Load(); err != nil {
		return fmt.Errorf("failed to load .env file: %w", err)
	}

	return nil
}

// GetEnv returns the value of an environment variable.
// If the variable does not exist, the fallback value is returned.
func GetEnv(key, fallback string) string {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	return value
}

package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL            string
	LogLevel               string
	WorkerName             string
	ObjectStorageLocalRoot string
}

func Load() (Config, error) {
	config := Config{
		DatabaseURL:            strings.TrimSpace(os.Getenv("DATABASE_URL")),
		LogLevel:               valueOrDefault("LOG_LEVEL", "info"),
		WorkerName:             valueOrDefault("WORKER_NAME", "aerosight-worker"),
		ObjectStorageLocalRoot: strings.TrimSpace(os.Getenv("OBJECT_STORAGE_LOCAL_ROOT")),
	}

	var problems []error
	if config.DatabaseURL == "" {
		problems = append(problems, errors.New("DATABASE_URL is required"))
	}
	if !isLogLevel(config.LogLevel) {
		problems = append(problems, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error, got %q", config.LogLevel))
	}
	if config.WorkerName == "" {
		problems = append(problems, errors.New("WORKER_NAME must not be empty"))
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid AeroSight worker configuration: %w", errors.Join(problems...))
	}
	return config, nil
}

func valueOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func isLogLevel(value string) bool {
	switch value {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}

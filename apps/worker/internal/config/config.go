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
	CallbackListenAddress  string
	CallbackPublicBaseURL  string
	AssetURLSigningSecret  string
	MediaAPIBaseURL        string
	MediaAPIUser           string
	MediaAPIPassword       string
}

func Load() (Config, error) {
	config := Config{
		DatabaseURL:            strings.TrimSpace(os.Getenv("DATABASE_URL")),
		LogLevel:               valueOrDefault("LOG_LEVEL", "info"),
		WorkerName:             valueOrDefault("WORKER_NAME", "aerosight-worker"),
		ObjectStorageLocalRoot: strings.TrimSpace(os.Getenv("OBJECT_STORAGE_LOCAL_ROOT")),
		CallbackListenAddress:  valueOrDefault("CALLBACK_LISTEN_ADDRESS", "127.0.0.1:8081"),
		CallbackPublicBaseURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("CALLBACK_PUBLIC_BASE_URL")), "/"),
		AssetURLSigningSecret:  strings.TrimSpace(os.Getenv("AUTH_SECRET")),
		MediaAPIBaseURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("MEDIA_API_BASE_URL")), "/"),
		MediaAPIUser:           strings.TrimSpace(os.Getenv("MEDIA_ADMIN_USER")),
		MediaAPIPassword:       strings.TrimSpace(os.Getenv("MEDIA_ADMIN_PASSWORD")),
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
	if config.CallbackListenAddress == "" || !strings.Contains(config.CallbackListenAddress, ":") {
		problems = append(problems, errors.New("CALLBACK_LISTEN_ADDRESS must be a host:port address"))
	}
	if config.CallbackPublicBaseURL != "" && !strings.HasPrefix(config.CallbackPublicBaseURL, "https://") {
		problems = append(problems, errors.New("CALLBACK_PUBLIC_BASE_URL must use HTTPS"))
	}
	mediaValues := 0
	for _, value := range []string{config.MediaAPIBaseURL, config.MediaAPIUser, config.MediaAPIPassword} {
		if value != "" {
			mediaValues++
		}
	}
	if mediaValues != 0 && mediaValues != 3 {
		problems = append(problems, errors.New("MEDIA_API_BASE_URL, MEDIA_ADMIN_USER, and MEDIA_ADMIN_PASSWORD must be configured together"))
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

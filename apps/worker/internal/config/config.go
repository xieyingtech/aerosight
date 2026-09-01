package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL               string
	LogLevel                  string
	WorkerName                string
	ObjectStorageLocalRoot    string
	CallbackListenAddress     string
	CallbackPublicBaseURL     string
	AssetURLSigningSecret     string
	AuthSecret                string
	MediaAPIBaseURL           string
	MediaAPIUser              string
	MediaAPIPassword          string
	FlightHubEnabled          bool
	FlightHubAPIBaseURL       string
	FlightHubHTTPTimeout      time.Duration
	FlightHubMaxRetries       int
	FlightHubPollInterval     time.Duration
	FlightHubReconcileEvery   time.Duration
	FlightHubMaxResponseBytes int64
	FlightHubAllowedLinkHosts []string
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
		AuthSecret:             strings.TrimSpace(os.Getenv("AUTH_SECRET")),
		MediaAPIBaseURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("MEDIA_API_BASE_URL")), "/"),
		MediaAPIUser:           strings.TrimSpace(os.Getenv("MEDIA_ADMIN_USER")),
		MediaAPIPassword:       strings.TrimSpace(os.Getenv("MEDIA_ADMIN_PASSWORD")),
	}
	var problems []error
	config.FlightHubEnabled, problems = booleanValue("DJI_FLIGHTHUB_ENABLED", false)
	config.FlightHubAPIBaseURL = valueOrDefault("DJI_FLIGHTHUB_API_BASE_URL", "https://es-flight-api-cn.djigate.com")
	config.FlightHubHTTPTimeout, problems = durationMilliseconds("DJI_FLIGHTHUB_HTTP_TIMEOUT_MS", 8*time.Second, problems)
	config.FlightHubMaxRetries, problems = integerValue("DJI_FLIGHTHUB_MAX_RETRIES", 2, 0, 3, problems)
	config.FlightHubPollInterval, problems = durationSeconds("DJI_FLIGHTHUB_POLL_INTERVAL_SECONDS", 5*time.Minute, problems)
	config.FlightHubReconcileEvery, problems = durationSeconds("DJI_FLIGHTHUB_RECONCILE_INTERVAL_SECONDS", 15*time.Second, problems)
	responseBytes, problems := integerValue("DJI_FLIGHTHUB_MAX_RESPONSE_BYTES", 4<<20, 1024, 16<<20, problems)
	config.FlightHubMaxResponseBytes = int64(responseBytes)
	config.FlightHubAllowedLinkHosts, problems = hostnameList(
		"DJI_FLIGHTHUB_ALLOWED_LINK_HOSTS",
		"es-flight-api-cn.djigate.com,test-file-storage.djicdn.com,files-cdn.dbeta.me",
		problems,
	)

	if config.DatabaseURL == "" {
		problems = append(problems, errors.New("DATABASE_URL is required"))
	}
	if config.FlightHubAPIBaseURL != "https://es-flight-api-cn.djigate.com" {
		problems = append(problems, errors.New("DJI_FLIGHTHUB_API_BASE_URL must use the official China HTTPS origin"))
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

func booleanValue(name string, fallback bool) (bool, []error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, []error{fmt.Errorf("%s must be true or false", name)}
	}
	return parsed, nil
}

func integerValue(name string, fallback, minimum, maximum int, problems []error) (int, []error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, problems
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, append(problems, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum))
	}
	return parsed, problems
}

func durationMilliseconds(name string, fallback time.Duration, problems []error) (time.Duration, []error) {
	value, problems := integerValue(name, int(fallback/time.Millisecond), 500, 30_000, problems)
	return time.Duration(value) * time.Millisecond, problems
}

func durationSeconds(name string, fallback time.Duration, problems []error) (time.Duration, []error) {
	value, problems := integerValue(name, int(fallback/time.Second), 1, 86_400, problems)
	return time.Duration(value) * time.Second, problems
}

func valueOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func hostnameList(name, fallback string, problems []error) ([]string, []error) {
	raw := valueOrDefault(name, fallback)
	items := strings.Split(raw, ",")
	result := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" || strings.ContainsAny(item, "/:@?#*") || strings.Contains(item, " ") {
			return nil, append(problems, fmt.Errorf("%s must contain comma-separated exact hostnames", name))
		}
		if _, duplicate := seen[item]; duplicate {
			return nil, append(problems, fmt.Errorf("%s must not contain duplicate hostnames", name))
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result, problems
}

func isLogLevel(value string) bool {
	switch value {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}

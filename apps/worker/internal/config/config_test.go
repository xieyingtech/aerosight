package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("expected clear DATABASE_URL error, got %v", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://database.example/aerosight")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("WORKER_NAME", "")
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.LogLevel != "info" || config.WorkerName != "aerosight-worker" || config.CallbackListenAddress != "127.0.0.1:8081" ||
		config.FlightHubAPIBaseURL != "https://es-flight-api-cn.djigate.com" || config.FlightHubHTTPTimeout != 8*time.Second || config.FlightHubMaxRetries != 2 ||
		config.FlightHubPollInterval != 5*time.Minute || config.FlightHubReconcileEvery != 15*time.Second || config.FlightHubMaxResponseBytes != 4<<20 ||
		len(config.FlightHubAllowedLinkHosts) != 3 {
		t.Fatalf("unexpected defaults: %#v", config)
	}
}

func TestLoadFlightHubConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://database.example/aerosight")
	t.Setenv("DJI_FLIGHTHUB_HTTP_TIMEOUT_MS", "12000")
	t.Setenv("DJI_FLIGHTHUB_MAX_RETRIES", "3")
	t.Setenv("DJI_FLIGHTHUB_POLL_INTERVAL_SECONDS", "600")
	t.Setenv("DJI_FLIGHTHUB_RECONCILE_INTERVAL_SECONDS", "20")
	t.Setenv("DJI_FLIGHTHUB_MAX_RESPONSE_BYTES", "8388608")
	t.Setenv("DJI_FLIGHTHUB_ALLOWED_LINK_HOSTS", "objects.vendor.example,media.vendor.example")
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.FlightHubHTTPTimeout != 12*time.Second || config.FlightHubMaxRetries != 3 ||
		config.FlightHubPollInterval != 10*time.Minute || config.FlightHubReconcileEvery != 20*time.Second || config.FlightHubMaxResponseBytes != 8<<20 ||
		len(config.FlightHubAllowedLinkHosts) != 2 || config.FlightHubAllowedLinkHosts[0] != "objects.vendor.example" {
		t.Fatalf("unexpected FlightHub configuration: %#v", config)
	}
}

func TestLoadRejectsInvalidFlightHubConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://database.example/aerosight")
	t.Setenv("DJI_FLIGHTHUB_MAX_RETRIES", "9")
	t.Setenv("DJI_FLIGHTHUB_API_BASE_URL", "https://example.test")
	t.Setenv("DJI_FLIGHTHUB_ALLOWED_LINK_HOSTS", "*.vendor.example")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DJI_FLIGHTHUB_MAX_RETRIES") ||
		!strings.Contains(err.Error(), "DJI_FLIGHTHUB_API_BASE_URL") || !strings.Contains(err.Error(), "DJI_FLIGHTHUB_ALLOWED_LINK_HOSTS") {
		t.Fatalf("expected FlightHub configuration errors, got %v", err)
	}
}

func TestLoadRejectsInvalidCallbackListenAddress(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://database.example/aerosight")
	t.Setenv("CALLBACK_LISTEN_ADDRESS", "missing-port")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "CALLBACK_LISTEN_ADDRESS") {
		t.Fatalf("expected callback address error, got %v", err)
	}
}

func TestLoadRejectsInsecureCallbackPublicURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://database.example/aerosight")
	t.Setenv("CALLBACK_PUBLIC_BASE_URL", "http://callbacks.example.test")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "CALLBACK_PUBLIC_BASE_URL") {
		t.Fatalf("expected callback public URL error, got %v", err)
	}
}

func TestLoadRequiresCompleteMediaGatewayConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://database.example/aerosight")
	t.Setenv("MEDIA_API_BASE_URL", "http://media:9997")
	t.Setenv("MEDIA_ADMIN_USER", "")
	t.Setenv("MEDIA_ADMIN_PASSWORD", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "must be configured together") {
		t.Fatalf("expected complete media gateway error, got %v", err)
	}
}

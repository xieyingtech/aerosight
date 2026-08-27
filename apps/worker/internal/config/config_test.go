package config

import (
	"strings"
	"testing"
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
	if config.LogLevel != "info" || config.WorkerName != "aerosight-worker" || config.CallbackListenAddress != "127.0.0.1:8081" {
		t.Fatalf("unexpected defaults: %#v", config)
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

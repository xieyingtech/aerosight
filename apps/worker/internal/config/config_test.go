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
	if config.LogLevel != "info" || config.WorkerName != "aerosight-worker" {
		t.Fatalf("unexpected defaults: %#v", config)
	}
}

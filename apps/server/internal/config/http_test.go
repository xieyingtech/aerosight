package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestCSPOriginConfiguration(t *testing.T) {
	for _, raw := range []string{"*", "https:", "https://*.example", "https://user:pass@example.test", "https://example.test/path", "https://example.test?", "https://example.test?q=1", "https://example.test#fragment", "https://example.test;script-src", "https://example.test\nscript-src 'unsafe-inline'", "data:", "http://media.example"} {
		if _, err := cspOrigins(raw, false); err == nil {
			t.Fatalf("accepted production origin %q", raw)
		}
	}
	origins, err := cspOrigins(" https://maps.example/,https://maps.example,https://media.example:8889 ", false)
	if err != nil || len(origins) != 2 || origins[0] != "https://maps.example" || origins[1] != "https://media.example:8889" {
		t.Fatalf("origins: %+v %v", origins, err)
	}
	if _, err := cspOrigins("http://localhost:8889", true); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"PUBLIC_ORIGIN": "https://aerosight.example", "CSRF_AUTH_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32))}
	cfg, err := LoadHTTP(func(k string) string { return env[k] })
	if err != nil || len(cfg.CSPMapOrigins) != 2 || cfg.CSPMapOrigins[0] != "https://tiles.openfreemap.org" || cfg.CSPMapOrigins[1] != "https://demotiles.maplibre.org" || len(cfg.CSPMediaOrigins) != 0 {
		t.Fatalf("default CSP: %+v %v", cfg.CSPMapOrigins, err)
	}
}

func TestHTTPConfigRequiresKeysAndOrigin(t *testing.T) {
	env := map[string]string{"PUBLIC_ORIGIN": "https://aerosight.example", "CSRF_AUTH_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32))}
	get := func(k string) string { return env[k] }
	if cfg, err := LoadHTTP(get); err != nil || cfg.AIRequestTimeout != 120*time.Second {
		t.Fatalf("AI default %v %v", cfg.AIRequestTimeout, err)
	}
	env["AI_REQUEST_TIMEOUT"] = "45s"
	if cfg, err := LoadHTTP(get); err != nil || cfg.AIRequestTimeout != 45*time.Second {
		t.Fatalf("AI override %v %v", cfg.AIRequestTimeout, err)
	}
	env["AI_REQUEST_TIMEOUT"] = "0s"
	if _, err := LoadHTTP(get); err == nil {
		t.Fatal("zero AI timeout accepted")
	}
	delete(env, "AI_REQUEST_TIMEOUT")
	env["ALGORITHM_ALLOWED_HOSTS"] = " algorithm.example, , *.trusted.example "
	if cfg, err := LoadHTTP(get); err != nil || len(cfg.AlgorithmAllowedHosts) != 2 || cfg.AlgorithmAllowedHosts[0] != "algorithm.example" || cfg.AlgorithmAllowedHosts[1] != "*.trusted.example" {
		t.Fatalf("algorithm allowlist %+v %v", cfg.AlgorithmAllowedHosts, err)
	}
	if _, err := LoadHTTP(get); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"CSRF_AUTH_KEY", "PUBLIC_ORIGIN"} {
		old := env[key]
		delete(env, key)
		if _, err := LoadHTTP(get); err == nil {
			t.Fatalf("accepted missing %s", key)
		}
		env[key] = old
	}
	env["PUBLIC_ORIGIN"] = "http://localhost:3000"
	env["AEROSIGHT_ENV"] = "development"
	if _, err := LoadHTTP(get); err != nil {
		t.Fatal(err)
	}
	env["TRUSTED_PROXIES"] = "*"
	if _, err := LoadHTTP(get); err == nil {
		t.Fatal("trusted arbitrary proxy")
	}
}

func TestHTTPConfigBudgetsAndValidation(t *testing.T) {
	base := map[string]string{"AEROSIGHT_ENV": "development", "CSRF_AUTH_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32))}
	get := func(k string) string { return base[k] }
	cfg, err := LoadHTTP(get)
	if err != nil || cfg.PublicOrigin != "http://localhost:3000" || cfg.HTTPPool != 20 || cfg.WorkerPool != 10 || cfg.SSELimit != 30 || cfg.RequestTimeout != 30*time.Second || cfg.ShutdownTimeout != 30*time.Second || len(cfg.TrustedProxies) != 0 {
		t.Fatalf("defaults %+v %v", cfg, err)
	}
	if cfg.SessionLifetime != 7*24*time.Hour || cfg.SessionIdle != 24*time.Hour {
		t.Fatalf("session defaults %+v", cfg)
	}
	base["SESSION_LIFETIME"], base["SESSION_IDLE_TIMEOUT"] = "48h", "2h"
	if cfg, err := LoadHTTP(get); err != nil || cfg.SessionLifetime != 48*time.Hour || cfg.SessionIdle != 2*time.Hour {
		t.Fatalf("session overrides %+v %v", cfg, err)
	}
	delete(base, "SESSION_LIFETIME")
	delete(base, "SESSION_IDLE_TIMEOUT")
	for _, key := range []string{"HTTP_DB_MAX_CONNECTIONS", "WORKER_DB_MAX_CONNECTIONS", "LOGIN_RATE_LIMIT", "WRITE_RATE_LIMIT", "SSE_RATE_LIMIT"} {
		for _, bad := range []string{"0", "-1", "10001", "1.5", "abc"} {
			base[key] = bad
			if _, err := LoadHTTP(get); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("%s=%s accepted: %v", key, bad, err)
			}
		}
		delete(base, key)
	}
	base["HTTP_DB_MAX_CONNECTIONS"] = "7"
	base["WORKER_DB_MAX_CONNECTIONS"] = "3"
	base["SSE_RATE_LIMIT"] = "9"
	if cfg, err := LoadHTTP(get); err != nil || cfg.HTTPPool != 7 || cfg.WorkerPool != 3 || cfg.SSELimit != 9 {
		t.Fatalf("budgets %+v %v", cfg, err)
	}
	for _, bad := range []string{"localhost", "localhost:", "localhost:nope", "localhost:65536", "localhost:-1"} {
		base["HTTP_LISTEN_ADDRESS"] = bad
		if _, err := LoadHTTP(get); err == nil {
			t.Fatalf("invalid address accepted: %s", bad)
		}
	}
	delete(base, "HTTP_LISTEN_ADDRESS")
	for _, bad := range []string{"not-base64", base64.StdEncoding.EncodeToString(make([]byte, 31)), base64.StdEncoding.EncodeToString(make([]byte, 33))} {
		base["CSRF_AUTH_KEY"] = bad
		if _, err := LoadHTTP(get); err == nil {
			t.Fatal("invalid CSRF key accepted")
		}
	}
}

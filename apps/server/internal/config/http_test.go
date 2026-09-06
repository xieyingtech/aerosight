package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

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

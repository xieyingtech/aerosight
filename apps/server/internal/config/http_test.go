package config

import (
	"encoding/base64"
	"testing"
)

func TestHTTPConfigRequiresKeysAndOrigin(t *testing.T) {
	env := map[string]string{"PUBLIC_ORIGIN": "https://aerosight.example", "CSRF_AUTH_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32))}
	get := func(k string) string { return env[k] }
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

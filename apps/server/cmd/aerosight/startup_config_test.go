//go:build dev

package main

import (
	"encoding/base64"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"aerosight/server/internal/webassets"
)

func TestDevelopmentStartupRejectsInvalidConfigurationBeforeDatabase(t *testing.T) {
	if pages, err := webassets.Embedded(); err != nil || pages != nil {
		t.Fatalf("dev unexpectedly requires frontend: %v", err)
	}
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	for _, tc := range []struct{ key, value, want string }{
		{"CSRF_AUTH_KEY", "", "CSRF_AUTH_KEY"},
		{"AUTH_SECRET", "short", "AUTH_SECRET"},
		{"HTTP_DB_MAX_CONNECTIONS", "0", "HTTP_DB_MAX_CONNECTIONS"},
		{"HTTP_LISTEN_ADDRESS", "127.0.0.1:99999", "HTTP_LISTEN_ADDRESS"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			os.Args = []string{"aerosight", "serve"}
			for key, value := range map[string]string{
				"DATABASE_URL": "postgresql://invalid.invalid/not-contacted", "AEROSIGHT_ENV": "development", "PUBLIC_ORIGIN": "http://localhost:3000", "AUTH_SECRET": strings.Repeat("a", 32), "CSRF_AUTH_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)), "HTTP_DB_MAX_CONNECTIONS": "20", "HTTP_LISTEN_ADDRESS": "127.0.0.1:8080", "CALLBACK_PUBLIC_BASE_URL": "", "MEDIA_API_BASE_URL": "", "MEDIA_ADMIN_USER": "", "MEDIA_ADMIN_PASSWORD": "",
			} {
				t.Setenv(key, value)
			}
			t.Setenv(tc.key, tc.value)
			if err := run(slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("startup boundary %v", err)
			}
		})
	}
}

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
		{"CSRF_SECRET", "", "CSRF_SECRET"},
		{"APP_SECRET", "short", "APP_SECRET"},
		{"HTTP_DB_MAX_CONNECTIONS", "0", "HTTP_DB_MAX_CONNECTIONS"},
		{"PORT", "99999", "PORT"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			os.Args = []string{"aerosight", "serve"}
			for key, value := range map[string]string{
				"DATABASE_URL": "postgresql://invalid.invalid/not-contacted", "AEROSIGHT_ENV": "development", "PUBLIC_ORIGIN": "http://localhost:3000", "APP_SECRET": strings.Repeat("a", 32), "CSRF_SECRET": base64.StdEncoding.EncodeToString(make([]byte, 32)), "HTTP_DB_MAX_CONNECTIONS": "20", "HOST": "127.0.0.1", "PORT": "8080", "CALLBACK_PUBLIC_BASE_URL": "", "MEDIA_API_BASE_URL": "", "MEDIA_ADMIN_USER": "", "MEDIA_ADMIN_PASSWORD": "",
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

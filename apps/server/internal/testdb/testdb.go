package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	_ "github.com/jackc/pgx/v5/stdlib"
	"net/url"
	"os"
	"strings"
	"testing"
)

// New creates and only removes a disposable database on an explicitly configured test server.
func New(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("AEROSIGHT_MIGRATION_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("AEROSIGHT_MIGRATION_TEST_DATABASE_URL is not configured")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatal(err)
	}
	suffix := make([]byte, 8)
	if _, err = rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "aerosight_test_" + hex.EncodeToString(suffix)
	if _, err = admin.ExecContext(context.Background(), "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	db, err := sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		if !strings.HasPrefix(name, "aerosight_test_") {
			panic("invalid test database name")
		}
		_, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		if err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	return db
}

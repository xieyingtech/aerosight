package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"time"
)

const lockKey = "aerosight.db.migrations"

var filePattern = regexp.MustCompile(`^\d{4}_[a-z0-9_]+\.sql$`)

type Migration struct{ Name, SQL, Checksum string }
type Applied struct {
	Name        string
	Adopted     bool
	ExecutionMS int64
}

func Read(source fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, err
	}
	var result []Migration
	for _, e := range entries {
		if e.IsDir() || !filePattern.MatchString(e.Name()) {
			continue
		}
		b, err := fs.ReadFile(source, e.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, Migration{e.Name(), string(b), fmt.Sprintf("%x", sha256.Sum256(b))})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	if len(result) == 0 {
		return nil, fmt.Errorf("no migrations found")
	}
	return result, nil
}

func Embedded(ctx context.Context, db *sql.DB) ([]Applied, error) {
	source, err := fs.Sub(Files, "sql")
	if err != nil {
		return nil, err
	}
	return Run(ctx, db, source)
}

// Run holds a dedicated connection for the same advisory lock used by the old Node runner.
func Run(ctx context.Context, db *sql.DB, source fs.FS) ([]Applied, error) {
	migrations, err := Read(source)
	if err != nil {
		return nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "select pg_advisory_lock(hashtext($1))", lockKey); err != nil {
		return nil, err
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(release, "select pg_advisory_unlock(hashtext($1))", lockKey)
	}()
	if _, err = conn.ExecContext(ctx, `create table if not exists schema_migrations (
 name text primary key, checksum text not null, adopted boolean not null default false,
 execution_ms integer not null default 0, applied_at timestamptz not null default now())`); err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, "select name, checksum from schema_migrations order by name")
	if err != nil {
		return nil, err
	}
	recorded := map[string]string{}
	for rows.Next() {
		var name, checksum string
		if err = rows.Scan(&name, &checksum); err != nil {
			rows.Close()
			return nil, err
		}
		recorded[name] = checksum
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Validate all historical checksums before applying any new change.
	for _, m := range migrations {
		if old, ok := recorded[m.Name]; ok && old != m.Checksum {
			return nil, fmt.Errorf("migration %s changed after it was applied; restore its original contents", m.Name)
		}
	}
	applied := []Applied{}
	for _, m := range migrations {
		if _, ok := recorded[m.Name]; ok {
			continue
		}
		adopted := false
		if m.Name == "0001_baseline.sql" && len(recorded) == 0 {
			if err = conn.QueryRowContext(ctx, `select to_regclass('public.users') is not null and to_regclass('public.projects') is not null and to_regclass('public.devices') is not null`).Scan(&adopted); err != nil {
				return applied, err
			}
		}
		if m.Name == "0002_enable_postgis.sql" {
			var available bool
			if err = conn.QueryRowContext(ctx, "select exists(select 1 from pg_available_extensions where name='postgis')").Scan(&available); err != nil {
				return applied, err
			}
			if !available {
				return applied, fmt.Errorf("PostGIS extension is required but is not available")
			}
		}
		started := time.Now()
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return applied, err
		}
		if !adopted {
			_, err = tx.ExecContext(ctx, m.SQL)
		}
		elapsed := time.Since(started).Milliseconds()
		if err == nil {
			_, err = tx.ExecContext(ctx, "insert into schema_migrations(name,checksum,adopted,execution_ms) values($1,$2,$3,$4)", m.Name, m.Checksum, adopted, elapsed)
		}
		if err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("migration %s failed: %w", m.Name, err)
		}
		if err = tx.Commit(); err != nil {
			return applied, err
		}
		recorded[m.Name] = m.Checksum
		applied = append(applied, Applied{m.Name, adopted, elapsed})
	}
	return applied, nil
}

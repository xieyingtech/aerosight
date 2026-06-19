package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `
		create table if not exists schema_migrations (
			version text primary key,
			applied_at timestamp not null default now()
		)
	`); err != nil {
		log.Fatalf("ensure migrations table: %v", err)
	}

	files, err := filepath.Glob(filepath.Join("db", "migrations", "*.up.sql"))
	if err != nil {
		log.Fatalf("list migrations: %v", err)
	}
	sort.Strings(files)

	for _, file := range files {
		version := strings.TrimSuffix(filepath.Base(file), ".up.sql")
		var exists bool
		if err := conn.QueryRow(ctx, `select exists(select 1 from schema_migrations where version = $1)`, version).Scan(&exists); err != nil {
			log.Fatalf("check migration %s: %v", version, err)
		}
		if exists {
			continue
		}
		if version == "000001_initial" {
			var hasExistingSchema bool
			if err := conn.QueryRow(ctx, `
				select exists (
					select 1
					from information_schema.tables
					where table_schema = 'public' and table_name = 'users'
				)
			`).Scan(&hasExistingSchema); err != nil {
				log.Fatalf("check existing schema: %v", err)
			}
			if hasExistingSchema {
				if _, err := conn.Exec(ctx, `insert into schema_migrations (version) values ($1)`, version); err != nil {
					log.Fatalf("baseline migration %s: %v", version, err)
				}
				fmt.Printf("baselined %s\n", version)
				continue
			}
		}

		sql, err := os.ReadFile(file)
		if err != nil {
			log.Fatalf("read migration %s: %v", file, err)
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			log.Fatalf("begin migration %s: %v", version, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatalf("apply migration %s: %v", version, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations (version) values ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatalf("record migration %s: %v", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			log.Fatalf("commit migration %s: %v", version, err)
		}
		fmt.Printf("applied %s\n", version)
	}
}

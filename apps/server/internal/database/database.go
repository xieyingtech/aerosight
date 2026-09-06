package database

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
	"time"
)

func Open(ctx context.Context, url string, maxOpen int) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxOpen / 2)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func InTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx, *sqlcgen.Queries) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx, sqlcgen.New(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func Bootstrap(ctx context.Context, db *sql.DB) error {
	return InTx(ctx, db, func(tx *sql.Tx, q *sqlcgen.Queries) error {
		if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtext($1))", "aerosight.bootstrap.default-admin"); err != nil {
			return err
		}
		exists, err := q.HasUsers(ctx)
		if err != nil || exists {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte("admin"), 12)
		if err != nil {
			return err
		}
		return q.CreateDefaultAdmin(ctx, sql.NullString{String: string(hash), Valid: true})
	})
}

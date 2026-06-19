package store

import (
	"context"
	"errors"
	"time"

	"aerosight/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	config.MaxConns = 10
	config.MinConns = 1
	config.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

func (s *Store) EnsureDefaultAdmin(ctx context.Context) error {
	var id int32
	err := s.pool.QueryRow(ctx, `select id from users limit 1`).Scan(&id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	password, err := auth.HashPassword("admin")
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		insert into users (name, email, password, role)
		values ($1, $2, $3, $4)
	`, "admin", "admin@example.com", password, "admin")
	return err
}

package database

import (
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestBootstrapAndSharedTransactionRollback(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(ctx, db); err != nil {
		t.Fatal(err)
	}
	before, err := sqlcgen.New(db).FindLoginUser(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(ctx, db); err != nil {
		t.Fatal(err)
	}
	after, _ := sqlcgen.New(db).FindLoginUser(ctx, "admin@example.com")
	if before.Password != after.Password {
		t.Fatal("bootstrap reset password")
	}
	expected := errors.New("rollback")
	err = InTx(ctx, db, func(tx *sql.Tx, q *sqlcgen.Queries) error {
		if _, err := tx.ExecContext(ctx, "delete from users"); err != nil {
			return err
		}
		if err := q.CreateDefaultAdmin(ctx, sql.NullString{String: "temporary", Valid: true}); err != nil {
			return err
		}
		return expected
	})
	if !errors.Is(err, expected) {
		t.Fatal(err)
	}
	after, err = sqlcgen.New(db).FindLoginUser(ctx, "admin@example.com")
	if err != nil || after.Password != before.Password {
		t.Fatal("transaction was not atomic")
	}
}

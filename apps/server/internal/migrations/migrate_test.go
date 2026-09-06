package migrations

import (
	"aerosight/server/internal/testdb"
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestReadPreservesBytesAndSorts(t *testing.T) {
	source := fstest.MapFS{"0002_next.sql": {Data: []byte("select 2;\r\n")}, "0001_first.sql": {Data: []byte("select 1;\n")}, "ignore.txt": {Data: []byte("ignored")}}
	got, err := Read(source)
	if err != nil || len(got) != 2 || got[0].Name != "0001_first.sql" || got[1].SQL != "select 2;\r\n" {
		t.Fatalf("%+v %v", got, err)
	}
	other, _ := Read(fstest.MapFS{"0002_next.sql": {Data: []byte("select 2;\n")}})
	if got[1].Checksum == other[0].Checksum {
		t.Fatal("checksum normalized SQL bytes")
	}
}

func TestFreshRepeatConcurrentAndHistoricalChecksum(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	source, _ := fs.Sub(Files, "sql")
	list, err := Read(source)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := Run(ctx, db, source); done <- err }()
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow("select count(*) from schema_migrations").Scan(&count); err != nil || count != len(list) {
		t.Fatalf("ledger %d %v", count, err)
	}
	applied, err := Run(ctx, db, source)
	if err != nil || len(applied) != 0 {
		t.Fatalf("repeated %+v %v", applied, err)
	}
	changed := fstest.MapFS{}
	for _, m := range list {
		changed[m.Name] = &fstest.MapFile{Data: []byte(m.SQL)}
	}
	changed[list[0].Name] = &fstest.MapFile{Data: []byte(list[0].SQL + "\n-- modified")}
	changed["9999_pending.sql"] = &fstest.MapFile{Data: []byte("create table should_not_exist(id int)")}
	if _, err = Run(ctx, db, changed); err == nil || !strings.Contains(err.Error(), "changed after") {
		t.Fatalf("expected checksum error: %v", err)
	}
	var present bool
	if err = db.QueryRow("select to_regclass('should_not_exist') is not null").Scan(&present); err != nil || present {
		t.Fatalf("pending ran before validation: %v", err)
	}
}

func TestLegacyBaselineAndFailedMigrationRollback(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	source, _ := fs.Sub(Files, "sql")
	baseline, _ := fs.ReadFile(source, "0001_baseline.sql")
	if _, err := db.Exec(string(baseline)); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, db, source); err != nil {
		t.Fatal(err)
	}
	var adopted bool
	if err := db.QueryRow("select adopted from schema_migrations where name='0001_baseline.sql'").Scan(&adopted); err != nil || !adopted {
		t.Fatalf("baseline not adopted: %v", err)
	}
	broken := fstest.MapFS{"9999_failed.sql": {Data: []byte("create table rolled_back(id int); select unknown_migration_function();")}}
	if _, err := Run(ctx, db, broken); err == nil {
		t.Fatal("failed migration accepted")
	}
	var exists bool
	if err := db.QueryRow("select to_regclass('rolled_back') is not null or exists(select 1 from schema_migrations where name='9999_failed.sql')").Scan(&exists); err != nil || exists {
		t.Fatalf("partial migration persisted: %v", err)
	}
}

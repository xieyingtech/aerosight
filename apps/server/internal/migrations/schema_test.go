package migrations

import (
	"aerosight/server/internal/testdb"
	"context"
	"database/sql"
	"os"
	"sort"
	"strings"
	"testing"
)

// Compare database-normalized definitions, not the formatting of migration SQL.
// Extension-owned objects and the migration runner's ledger are not app schema.
const schemaCatalog = `
WITH app_relations AS (
 SELECT c.oid,c.relname,c.relkind,c.relrowsecurity,c.relforcerowsecurity
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='public' AND c.relname<>'schema_migrations'
 AND c.relkind IN ('r','p','v','m')
 AND NOT EXISTS(SELECT 1 FROM pg_depend d WHERE d.classid='pg_class'::regclass AND d.objid=c.oid AND d.deptype='e')
)
SELECT 'relation/'||relname, jsonb_build_array(relkind,relrowsecurity,relforcerowsecurity)::text FROM app_relations
UNION ALL
SELECT 'column/'||c.relname||'/'||a.attname,
 jsonb_build_array(format_type(a.atttypid,a.atttypmod),a.attnotnull,a.attidentity,a.attgenerated,pg_get_expr(d.adbin,d.adrelid))::text
FROM app_relations c JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped
LEFT JOIN pg_attrdef d ON d.adrelid=c.oid AND d.adnum=a.attnum
UNION ALL
SELECT 'constraint/'||c.relname||'/'||k.conname,pg_get_constraintdef(k.oid,true)
FROM app_relations c JOIN pg_constraint k ON k.conrelid=c.oid
-- PostgreSQL 18 catalogs NOT NULL constraints with inherited/generated names.
-- Column attnotnull above already compares their actual behavior.
WHERE k.contype <> 'n'
UNION ALL
SELECT 'index/'||c.relname||'/'||i.relname,pg_get_indexdef(i.oid)
FROM app_relations c JOIN pg_index x ON x.indrelid=c.oid JOIN pg_class i ON i.oid=x.indexrelid
UNION ALL
SELECT 'trigger/'||c.relname||'/'||t.tgname,pg_get_triggerdef(t.oid,true)
FROM app_relations c JOIN pg_trigger t ON t.tgrelid=c.oid WHERE NOT t.tgisinternal
UNION ALL
SELECT 'view/'||c.relname,pg_get_viewdef(c.oid,true) FROM app_relations c WHERE c.relkind IN ('v','m')
UNION ALL
SELECT 'function/'||p.oid::regprocedure::text,pg_get_functiondef(p.oid)
FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
WHERE n.nspname='public' AND p.prokind IN ('f','p')
AND NOT EXISTS(SELECT 1 FROM pg_depend d WHERE d.classid='pg_proc'::regclass AND d.objid=p.oid AND d.deptype='e')
UNION ALL
SELECT 'sequence/'||c.relname,jsonb_build_array(format_type(s.seqtypid,NULL),s.seqstart,s.seqincrement,s.seqmax,s.seqmin,s.seqcache,s.seqcycle)::text
FROM pg_sequence s JOIN pg_class c ON c.oid=s.seqrelid JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='public'
AND NOT EXISTS(SELECT 1 FROM pg_depend d WHERE d.classid='pg_class'::regclass AND d.objid=c.oid AND d.deptype='e')
UNION ALL
SELECT 'policy/'||c.relname||'/'||p.polname,jsonb_build_array(p.polcmd,p.polpermissive,pg_get_expr(p.polqual,p.polrelid),pg_get_expr(p.polwithcheck,p.polrelid),
 (SELECT array_agg(CASE WHEN r=0 THEN 'public' ELSE pg_get_userbyid(r)::text END ORDER BY r) FROM unnest(p.polroles) r))::text
FROM app_relations c JOIN pg_policy p ON p.polrelid=c.oid
UNION ALL
SELECT 'enum/'||t.typname,jsonb_agg(e.enumlabel ORDER BY e.enumsortorder)::text
FROM pg_type t JOIN pg_namespace n ON n.oid=t.typnamespace JOIN pg_enum e ON e.enumtypid=t.oid
WHERE n.nspname='public'
AND NOT EXISTS(SELECT 1 FROM pg_depend d WHERE d.classid='pg_type'::regclass AND d.objid=t.oid AND d.deptype='e')
GROUP BY t.typname
`

func catalog(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(schemaCatalog)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatal(err)
		}
		// Git checkout line endings may differ between embedded migrations and
		// the snapshot; retain all other function body text for comparison.
		result[key] = strings.ReplaceAll(value, "\r\n", "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSchemaSnapshotMatchesFullMigration(t *testing.T) {
	migrated := testdb.New(t)
	if _, err := Embedded(context.Background(), migrated); err != nil {
		t.Fatal(err)
	}
	snapshot := testdb.New(t)
	data, err := os.ReadFile("../../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = snapshot.Exec(string(data)); err != nil {
		t.Fatal(err)
	}
	actual, expected := catalog(t, migrated), catalog(t, snapshot)
	keys := map[string]bool{}
	for key := range actual {
		keys[key] = true
	}
	for key := range expected {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		if actual[key] != expected[key] {
			t.Errorf("schema drift %s\nmigrated: %s\nsnapshot: %s", key, actual[key], expected[key])
		}
	}
}

package migrations

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"

	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/media"
	"aerosight/server/internal/testdb"
	"github.com/google/uuid"
)

func TestUsageCleanupUpgradePreservesHistoryAndAccess(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	source, _ := fs.Sub(Files, "sql")
	list, err := Read(source)
	if err != nil {
		t.Fatal(err)
	}
	previous, compact := fstest.MapFS{}, fstest.MapFS{}
	for _, m := range list {
		if m.Name < "0090_" {
			previous[m.Name] = &fstest.MapFile{Data: []byte(m.SQL)}
		}
		if m.Name < "0091_" {
			compact[m.Name] = &fstest.MapFile{Data: []byte(m.SQL)}
		}
	}
	if _, err = Run(ctx, db, previous); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(compactUpgradeFixture); err != nil {
		t.Fatal(err)
	}
	if _, err = Run(ctx, db, compact); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`
update assets set status='available',metadata_json='{"name":"display-name.jpg"}',mime_type='image/jpeg' where id=1001;
insert into assets(id,project_id,team_id,kind,storage_key,logical_key) values(1101,1001,1001,'thumbnail','thumbnail.jpg','thumbnail');
insert into assets(id,project_id,team_id,kind,storage_key,logical_key) values(1102,1001,1001,'image','second-source.jpg','second-source');
insert into asset_upload_intents(id,project_id,team_id,logical_key,object_key,file_name,kind,mime_type,expected_size_bytes,expected_checksum_sha256,asset_id,status,expires_at,completed_at)
values('00000000-0000-4000-8000-000000003001',1001,1001,'image','image-upload','original-name.jpg','image','image/jpeg',10,repeat('a',64),1001,'completed',now(),now());
insert into asset_upload_intents(id,project_id,team_id,logical_key,object_key,file_name,kind,mime_type,expected_size_bytes,expected_checksum_sha256,status,expires_at)
values('00000000-0000-4000-8000-000000003002',1001,1001,'unused','pending-upload','pending.jpg','image','image/jpeg',10,repeat('b',64),'pending',now());
insert into evidence_links(project_id,team_id,target_type,target_id,asset_id,asset_version,asset_checksum_sha256,is_published,start_offset_ms,end_offset_ms)
values(1001,1001,'report','legacy-report',1001,1,repeat('a',64),true,20,150);
insert into event_rules(id,project_id,team_id,name) values(1101,1001,1001,'legacy rule');
insert into event_rule_versions(id,project_id,team_id,event_rule_id,version,label,minimum_confidence,severity)
values(1101,1001,1001,1101,1,'person',0.5,'low');
insert into perception_events(id,project_id,team_id,event_rule_version_id,detection_group_id,deduplication_key,severity,first_detected_at,last_detected_at)
values('00000000-0000-4000-8000-000000003003',1001,1001,1101,1001,'legacy event','low',now(),now());
insert into event_feedback(id,project_id,team_id,perception_event_id,action,value_json,reason,actor_user_id,created_at)
values(1101,1001,1001,'00000000-0000-4000-8000-000000003003','confirm','{"category":"person"}','original reason',1001,'2026-10-08T10:01:00Z');
insert into event_feedback(id,project_id,team_id,perception_event_id,action,value_json,reason,actor_user_id,created_at)
values(1102,1001,1001,'00000000-0000-4000-8000-000000003003','false_positive','{}','earlier feedback',1001,'2026-10-08T10:00:00Z');
insert into asset_derivatives(project_id,team_id,source_asset_id,derived_asset_id,derivative_type,generator,generator_version,parameters_json)
values(1001,1001,1001,1101,'thumbnail','fixture','fixture-v1','{"width":160}');
insert into asset_derivatives(project_id,team_id,source_asset_id,derived_asset_id,derivative_type,generator,generator_version,parameters_json)
values(1001,1001,1102,1101,'alternate-thumbnail','fixture','fixture-v2','{"width":320}');
`); err != nil {
		t.Fatal("0090 fixture", err)
	}
	if _, err = Run(ctx, db, source); err != nil {
		t.Fatal("0091 upgrade", err)
	}
	q := sqlcgen.New(db)
	asset, err := q.ReadMediaAccessAsset(ctx, sqlcgen.ReadMediaAccessAssetParams{ProjectID: 1001, ID: 1001})
	if err != nil || asset.FileName != "original-name.jpg" || !asset.Sensitive {
		t.Fatalf("media access changed: %+v %v", asset, err)
	}
	checks := []string{
		`select count(*)=7 from audit_events where request_id='schema-0091' and action='schema.archive'`,
		`select count(*)=2 and bool_or(details_json->>'source_asset_id'='1102' and details_json->>'generator_version'='fixture-v2') from audit_events where request_id='schema-0091' and resource_type='asset_derivatives'`,
		`select details_json->>'file_name'='pending.jpg' from audit_events where request_id='schema-0091' and resource_type='asset_upload_intents' and resource_id='00000000-0000-4000-8000-000000003002'`,
		`select details_json->>'start_offset_ms'='20' and details_json->>'end_offset_ms'='150' and details_json->>'asset_checksum_sha256'=repeat('a',64) from audit_events where request_id='schema-0091' and resource_type='evidence_links'`,
		`select metadata_json->>'name'='display-name.jpg' from assets where id=1001`,
		`select derivative_source_asset_id=1001 and derivative_type='thumbnail' and metadata_json->>'generatorVersion'='fixture-v1' and metadata_json->'derivativeParameters'->>'width'='160' from assets where id=1101`,
		`select to_regclass('asset_upload_intents') is null and to_regclass('evidence_links') is null and to_regclass('event_feedback') is null and to_regclass('asset_derivatives') is null`,
	}
	for _, query := range checks {
		var ok bool
		if err = db.QueryRow(query).Scan(&ok); err != nil || !ok {
			t.Fatalf("%s: %v (%v)", query, ok, err)
		}
	}
	for _, query := range []string{
		`delete from assets where id=1001`,
		`update assets set metadata_json=metadata_json-'legacyPublishedEvidence' where id=1001`,
		`update assets set derivative_source_asset_id=1002 where id=1101`,
	} {
		if _, err = db.Exec(query); err == nil {
			t.Fatalf("expected refusal: %s", query)
		}
	}
	if _, err = db.Exec(`update users set name='renamed operator' where id=1001; delete from team_members where team_id=1001 and user_id=1001`); err != nil {
		t.Fatal(err)
	}
	params := sqlcgen.GetLegacyPerceptionFeedbackParams{ProjectID: 1001, ID: uuid.MustParse("00000000-0000-4000-8000-000000003003")}
	rows, err := q.GetLegacyPerceptionFeedback(ctx, params)
	if err != nil || len(rows) != 2 {
		t.Fatalf("feedback lost: %s %v", rows, err)
	}
	var feedback map[string]any
	if err = json.Unmarshal(rows[0], &feedback); err != nil {
		t.Fatal(err)
	}
	if feedback["id"] != "1102" || feedback["reason"] != "earlier feedback" {
		t.Fatalf("feedback is not ordered by occurrence time: %s", rows)
	}
	if err = json.Unmarshal(rows[1], &feedback); err != nil {
		t.Fatal(err)
	}
	if feedback["id"] != "1101" || feedback["reason"] != "original reason" || feedback["actorName"] != "renamed operator" || feedback["createdAt"] != "2026-10-08T10:01:00+00:00" {
		t.Fatalf("feedback changed: %s", rows[1])
	}
	if feedback["value"].(map[string]any)["category"] != "person" {
		t.Fatalf("feedback value changed: %s", rows[1])
	}
	params.ProjectID = 1002
	rows, err = q.GetLegacyPerceptionFeedback(ctx, params)
	if err != nil || len(rows) != 0 {
		t.Fatalf("cross-project feedback leaked: %s %v", rows, err)
	}

	// Exercise the registered derivative writer against the final schema, twice.
	d := media.Derivative{SourceAssetID: 1001, ProjectID: 1001, TeamID: 1001, LogicalKey: "new-thumbnail", Version: 1, Kind: "thumbnail", MimeType: "image/jpeg", StorageKey: "new-thumbnail.jpg", ChecksumSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", DerivativeType: "thumbnail", Generator: media.ThumbnailGeneratorVersion, Width: 160, Height: 90}
	for range 2 {
		tx, e := db.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		e = media.NewSQLRepository().SaveDerivative(ctx, tx, d)
		if e != nil {
			tx.Rollback()
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
	}
	var n int
	if err = db.QueryRow(`select count(*) from assets where project_id=1001 and logical_key='new-thumbnail' and derivative_source_asset_id=1001 and metadata_json->>'width'='160'`).Scan(&n); err != nil || n != 1 {
		t.Fatal("derivative retry lost provenance or duplicated", n, err)
	}
	d.SourceAssetID = 1002
	d.LogicalKey = "forged-thumbnail"
	d.StorageKey = "forged-thumbnail.jpg"
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = media.NewSQLRepository().SaveDerivative(ctx, tx, d)
	tx.Rollback()
	if err == nil {
		t.Fatal("cross-project derivative accepted")
	}
	if applied, e := Run(ctx, db, source); e != nil || len(applied) != 0 {
		t.Fatal("second migration was not a no-op", e)
	}
}

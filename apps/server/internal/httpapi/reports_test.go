package httpapi

import (
	"fmt"
	"strings"
	"testing"
)

func TestReportPublicationRetentionAndExport(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	var report, version string
	var asset int
	if err := f.db.QueryRow("insert into assets(project_id,team_id,kind,storage_key,logical_key) values($1,$2,'image','test/report.jpg','report.jpg') returning id", pid, team).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into generated_reports(project_id,team_id,source_type,source_id,title) values($1,$2,'task_run','1','Inspection') returning id", pid, team).Scan(&report); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`insert into generated_report_versions(project_id,team_id,generated_report_id,version,completeness,content_json) values($1,$2,$3,1,'incomplete',jsonb_build_object('evidence',jsonb_build_array(jsonb_build_object('assetId',$4::int),jsonb_build_object('assetId',$4::int)))) returning id`, pid, team, report, asset).Scan(&version); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/reports/%s", pid, report)
	res := f.request(t, "GET", path+"/export", "")
	data := decodedResponse(t, res)
	if res.StatusCode != 403 || data["error"] != "PUBLISHED_REPORT_NOT_FOUND" {
		t.Fatalf("draft export %d %+v", res.StatusCode, data)
	}
	res = f.request(t, "POST", path+"/publish", `{}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "REPORT_INCOMPLETE_CONFIRMATION_REQUIRED" {
		t.Fatalf("incomplete %d %+v", res.StatusCode, data)
	}
	// Failure in retention must roll back both publication pointers and audit.
	if _, err := f.db.Exec(`create function reject_retention() returns trigger language plpgsql as $$ begin raise exception 'private retention detail'; end $$;create trigger reject_retention before update on assets for each row execute function reject_retention()`); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path+"/publish", `{"allowIncomplete":true}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "REPORT_PUBLISH_FAILED" {
		t.Fatalf("retention failure %d %+v", res.StatusCode, data)
	}
	var count int
	if err := f.db.QueryRow("select count(*) from generated_reports where id=$1 and current_published_version_id is not null", report).Scan(&count); err != nil || count != 0 {
		t.Fatalf("pointer rollback %d %v", count, err)
	}
	if err := f.db.QueryRow("select count(*) from audit_events where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
		t.Fatalf("audit rollback %d %v", count, err)
	}
	if _, err := f.db.Exec("drop trigger reject_retention on assets"); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path+"/publish", `{"allowIncomplete":true}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || data["versionId"] != version || len(data["retainedAssetIds"].([]any)) != 1 {
		t.Fatalf("publish %d %+v", res.StatusCode, data)
	}
	var held bool
	var reason string
	if err := f.db.QueryRow("select legal_hold,retention_reason from assets where id=$1", asset).Scan(&held, &reason); err != nil || !held || reason != "published-report:"+report {
		t.Fatalf("retention %v %s %v", held, reason, err)
	}
	res = f.request(t, "GET", path+"/export", "")
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || data["id"] != report || data["version"] != float64(1) || data["publishedAt"] == nil || !strings.Contains(res.Header.Get("Content-Disposition"), report+".json") || res.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("export %d %+v", res.StatusCode, data)
	}
	res = f.request(t, "POST", path+"/publish", `{"allowIncomplete":true}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "REPORT_DRAFT_NOT_FOUND" {
		t.Fatalf("repeat %d %+v", res.StatusCode, data)
	}
	var second string
	if err := f.db.QueryRow("insert into generated_report_versions(project_id,team_id,generated_report_id,version,completeness) values($1,$2,$3,2,'failed') returning id", pid, team, report).Scan(&second); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path+"/publish", `{"allowIncomplete":true}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "REPORT_FAILED_NOT_PUBLISHABLE" {
		t.Fatalf("failed %d %+v", res.StatusCode, data)
	}
	if _, err := f.db.Exec("update generated_report_versions set completeness='complete' where id=$1", second); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path+"/publish", `{}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || data["versionId"] != second || len(data["retainedAssetIds"].([]any)) != 0 {
		t.Fatalf("replacement %d %+v", res.StatusCode, data)
	}
	var oldStatus string
	if err := f.db.QueryRow("select status from generated_report_versions where id=$1", version).Scan(&oldStatus); err != nil || oldStatus != "retired" {
		t.Fatalf("retired %s %v", oldStatus, err)
	}
	res = f.request(t, "GET", path+"/export", "")
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || data["version"] != float64(2) {
		t.Fatalf("latest export %d %+v", res.StatusCode, data)
	}
	_, other := f.project(t)
	res = f.request(t, "GET", fmt.Sprintf("/api/projects/%d/reports/%s/export", other, report), "")
	data = decodedResponse(t, res)
	if res.StatusCode != 403 {
		t.Fatalf("scope %d %+v", res.StatusCode, data)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) select $1,$2,id,'mission:operate' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", path+"/export", "")
	data = decodedResponse(t, res)
	if res.StatusCode != 403 || data["error"] != "PROJECT_ACCESS_DENIED" {
		t.Fatalf("export permission %d %+v", res.StatusCode, data)
	}
	if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) select $1,$2,id,'report:export' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", path+"/export", "")
	data = decodedResponse(t, res)
	if res.StatusCode != 200 {
		t.Fatalf("export allowed %d %+v", res.StatusCode, data)
	}
}

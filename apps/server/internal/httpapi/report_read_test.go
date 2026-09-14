package httpapi

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestReportReadLatestScopedAndAssetAvailability(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	report, old, latest := uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`insert into generated_reports(id,project_id,team_id,source_type,source_id,title) values($1,$2,$3,'task_run','1','Report fixture')`, report, pid, team)
	exec(`insert into generated_report_versions(id,project_id,team_id,generated_report_id,version,status,completeness,content_json) values($1,$3,$4,$5,1,'retired','complete','{}'),($2,$3,$4,$5,2,'draft','incomplete','{"sections":{"issues":[]}}')`, old, latest, pid, team, report)
	var asset int
	if err := f.db.QueryRow(`insert into assets(project_id,team_id,kind,storage_key,logical_key,status,checksum_sha256) values($1,$2,'image','report.jpg','report.jpg','available','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa') returning id`, pid, team).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	for n, v := range []string{"1", "version:1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		exec(`insert into generated_report_evidence(project_id,report_version_id,evidence_type,evidence_id,evidence_version,asset_id,checksum_sha256,href) values($1,$2,'asset',$3,$4,$5,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','/fixture')`, pid, latest, fmt.Sprint(n), v, asset)
	}
	path := fmt.Sprintf("/api/projects/%d/reports/%s", pid, report)
	read := func(path string, want int) map[string]any {
		t.Helper()
		res := f.request(t, "GET", path, "")
		data := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("read %d %+v", res.StatusCode, data)
		}
		return data
	}
	check := func(want string) {
		t.Helper()
		data := read(path, 200)
		if data["versionId"] != latest {
			t.Fatal("did not read latest", data)
		}
		refs := data["evidence"].([]any)
		if len(refs) != 3 {
			t.Fatal("missing evidence", data)
		}
		for _, ref := range refs {
			if ref.(map[string]any)["availability"] != want {
				t.Fatalf("availability %+v", ref)
			}
		}
	}
	check("not-revalidated")
	exec(`update assets set version=2,checksum_sha256='bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' where id=$1`, asset)
	check("unavailable-or-version-changed")
	exec(`update assets set version=1,checksum_sha256='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',deleted_at=now() where id=$1`, asset)
	check("unavailable-or-version-changed")
	_, other := f.project(t)
	read(fmt.Sprintf("/api/projects/%d/reports/%s", other, report), 404)
	read(fmt.Sprintf("/api/projects/%d/reports/%s", pid, uuid.NewString()), 404)
	read(fmt.Sprintf("/api/projects/%d/reports/invalid", pid), 404)
	read(fmt.Sprintf("/api/projects/2147483647/reports/%s", report), 403)
}

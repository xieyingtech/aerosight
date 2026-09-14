package httpapi

import (
	"aerosight/server/internal/algorithm"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestInspectionSnapshotsAreProjectScopedAndSealed(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	_, other := f.project(t)
	id := func(q string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := f.db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	task := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'snapshot','manual','typed-task-v2') returning id", pid, team)
	version := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", pid, team, task)
	step := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses) values($1,$2,$3,1,'observe','observe','inspection.observe','inspection.observe') returning id", pid, team, version)
	run := id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status) values($1,$2,$3,$4,'manual','running') returning id", pid, team, task, version)
	runStep := id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,1,'succeeded') returning id", pid, team, run, step)
	observation := uuid.NewString()
	evidence := uuid.NewString()
	exec(`insert into inspection_observations(id,project_id,team_id,task_run_id,task_run_step_id,source_mode,scope_description,observed_from,observed_to,manifest_json) values($1,$2,$3,$4,$5,'assets','fixture',now(),now(),'{"scopeDescription":"fixture"}')`, observation, pid, team, run, runStep)
	call := func(path string, want int) {
		t.Helper()
		res := f.request(t, "GET", path, "")
		defer res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("%s got %d want %d", path, res.StatusCode, want)
		}
	}
	path := fmt.Sprintf("/api/projects/%d/inspection/observations/%s", pid, observation)
	call(path, 404)
	root := t.TempDir()
	f.server.AttachMediaStorage(root)
	imageBody, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aL1sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "projects", fmt.Sprint(pid))
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(dir, "original.png")
	if err = os.WriteFile(imagePath, imageBody, 0600); err != nil {
		t.Fatal(err)
	}
	asset := id(`insert into assets(project_id,team_id,kind,storage_key,logical_key,status) values($1,$2,'image',$3,'original.png','available') returning id`, pid, team, fmt.Sprintf("projects/%d/original.png", pid))
	digest := sha256.Sum256(imageBody)
	manifest, _ := json.Marshal(map[string]any{"scopeDescription": "fixture", "assets": []any{map[string]any{"projectId": pid, "teamId": team, "assetId": asset, "version": 1, "checksumSha256": hex.EncodeToString(digest[:])}}})
	exec(`update inspection_observations set manifest_json=$2 where id=$1`, observation, string(manifest))
	exec("update inspection_observations set sealed_at=now() where id=$1", observation)
	call(path, 200)
	exec(`insert into inspection_evidence_sets(id,project_id,team_id,task_run_id,task_run_step_id,observation_id,source,model_version) values($1,$2,$3,$4,$5,$6,'external','fixture')`, evidence, pid, team, run, runStep, observation)
	evidencePath := fmt.Sprintf("/api/projects/%d/inspection/evidence-sets/%s", pid, evidence)
	call(evidencePath, 200)
	for kind, snapshot := range map[string]string{"observations": observation, "evidence-sets": evidence} {
		call(fmt.Sprintf("/api/projects/%d/inspection/%s/%s", other, kind, snapshot), 404)
		call(fmt.Sprintf("/api/projects/%d/inspection/%s/bad-id", pid, kind), 404)
	}
	imageURL := fmt.Sprintf("%s/assets/%d/content", path, asset)
	res := f.request(t, "GET", imageURL, "")
	got, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 || !bytes.Equal(got, imageBody) || res.Header.Get("Content-Type") != "image/png" || res.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("image preview %d %s %v", res.StatusCode, got, err)
	}
	call(fmt.Sprintf("%s/assets/%d/content", path, asset+1), 403)
	call(fmt.Sprintf("/api/projects/%d/inspection/observations/%s/assets/%d/content", other, observation, asset), 403)
	if err = os.WriteFile(imagePath, []byte("changed bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	call(imageURL, 409)
	if err = os.WriteFile(imagePath, imageBody, 0600); err != nil {
		t.Fatal(err)
	}
	exec(`update assets set version=2 where id=$1`, asset)
	call(imageURL, 409)
	exec(`update assets set version=1 where id=$1`, asset)
	// A remote error must not fall back to the valid local file.
	f.server.AttachInspectionMedia(func(context.Context, int, int, int) (algorithm.AlgorithmAsset, bool, error) {
		return algorithm.AlgorithmAsset{}, true, errors.New("fixture remote unavailable")
	})
	call(imageURL, 404)
	f.server.AttachInspectionMedia(func(context.Context, int, int, int) (algorithm.AlgorithmAsset, bool, error) {
		return algorithm.AlgorithmAsset{Body: imageBody}, true, nil
	})
	call(imageURL, 200)
	f.server.AttachInspectionMedia(func(context.Context, int, int, int) (algorithm.AlgorithmAsset, bool, error) {
		return algorithm.AlgorithmAsset{Body: []byte("remote changed")}, true, nil
	})
	call(imageURL, 409)
	exec("delete from team_members where team_id=$1", team)
	call(imageURL, 403)
	call(path, 403)
	call(evidencePath, 403)
}

package httpapi

import (
	"aerosight/server/internal/taskdefinition"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const reportTaskYAML = "apiVersion: aerosight/v2\nname: 图片分析报告\ntrigger: {type: manual}\nsteps:\n  - key: report\n    uses: report.generate\n    with: {scope: current-run}\n"

func TestTaskV2AuthorNormalizationAndReferences(t *testing.T) {
	definition, _, _, err := parseTaskAuthorInput(map[string]any{"sourceFormat": "yaml", "source": reportTaskYAML})
	if err != nil {
		t.Fatal(err)
	}
	if definition["concurrencyLimit"] != float64(1) {
		t.Fatal("default concurrency missing")
	}
	for name, source := range map[string]string{
		"nested forward reference": "apiVersion: aerosight/v2\nname: bad\ntrigger: {type: manual}\nsteps: [{key: first, uses: report.generate, with: {nested: [steps.second.outputs.reportId]}}, {key: second, uses: report.generate}]",
		"unknown version":          "apiVersion: other\nname: bad\nsteps: []",
		"unknown capability":       "apiVersion: aerosight/v2\nname: bad\ntrigger: {type: manual}\nsteps: [{key: bad, uses: shell.run}]",
		"forward reference":        "apiVersion: aerosight/v2\nname: bad\ntrigger: {type: manual}\nsteps: [{key: first, uses: inspection.detect, with: {observationId: steps.second.outputs.observationId}}, {key: second, uses: inspection.observe}]",
		"forward dependency":       "apiVersion: aerosight/v2\nname: bad\ntrigger: {type: manual}\nsteps: [{key: first, uses: report.generate, dependsOn: [second]}, {key: second, uses: report.generate}]",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := parseTaskAuthorInput(map[string]any{"sourceFormat": "yaml", "source": source}); err == nil {
				t.Fatal("invalid definition accepted")
			}
		})
	}
}

func TestTaskAuthoringCreateRevisionPublishPostgres(t *testing.T) {
	f := newAPIFixture(t)
	_, pid := f.project(t)
	base := fmt.Sprintf("/api/projects/%d/tasks", pid)
	call := func(path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, "POST", path, string(raw))
		out := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s status %d want %d: %+v", path, res.StatusCode, want, out)
		}
		return out
	}
	source := gin.H{"sourceFormat": "yaml", "source": reportTaskYAML}
	validated := call(base+"/validate", source, 200)
	if validated["canPublish"] != true {
		t.Fatalf("report unavailable: %+v", validated)
	}
	var count int
	if err := f.db.QueryRow("select count(*) from tasks where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
		t.Fatal("validation mutated tasks")
	}
	source["idempotencyKey"] = "create-report"
	created := call(base, source, 201)
	replay := call(base, source, 200)
	if replay["taskId"] != created["taskId"] || replay["replayed"] != true {
		t.Fatal("create replay did not reuse task")
	}
	tid := int(created["taskId"].(float64))
	vid := created["versionId"]
	path := fmt.Sprintf("%s/%d/versions", base, tid)
	call(path, gin.H{"action": "save", "versionId": vid, "sourceFormat": "yaml", "source": reportTaskYAML, "expectedRevision": 0}, 409)
	saved := call(path, gin.H{"action": "save", "versionId": vid, "sourceFormat": "yaml", "source": reportTaskYAML, "expectedRevision": 1}, 200)
	if saved["revision"] != float64(2) {
		t.Fatal("revision not incremented")
	}
	call(path, gin.H{"action": "publish", "versionId": vid, "expectedRevision": 1}, 409)
	call(path, gin.H{"action": "publish", "versionId": vid, "expectedRevision": 2}, 200)
	call(path, gin.H{"action": "save", "versionId": vid, "sourceFormat": "yaml", "source": reportTaskYAML, "expectedRevision": 2}, 400)
	var text, status, hash string
	if err := f.db.QueryRow("select v.author_source,t.status,v.definition_hash from task_versions v join tasks t on t.id=v.task_id where v.id=$1", vid).Scan(&text, &status, &hash); err != nil {
		t.Fatal(err)
	}
	if text != reportTaskYAML || status != "disabled" || hash == "" {
		t.Fatal("source/hash/status not preserved")
	}
	next := call(path, gin.H{"action": "create"}, 200)
	nextID := next["draft"].(map[string]any)["id"]
	if err := f.db.QueryRow("select author_source from task_versions where id=$1", nextID).Scan(&text); err != nil || text != reportTaskYAML {
		t.Fatal("clone lost YAML source")
	}
	source["source"] = reportTaskYAML + "description: changed\n"
	call(base, source, 409)
}

func TestTaskV2PublicationBlocksUndeployedAndCrossProjectResources(t *testing.T) {
	f := newAPIFixture(t)
	_, pid := f.project(t)
	otherTeam, otherPID := f.project(t)
	var asset int
	if err := f.db.QueryRow("insert into assets(project_id,team_id,kind,storage_key,logical_key) values($1,$2,'image','private','private') returning id", otherPID, otherTeam).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ with, code string }{
		{"", "TASK_CAPABILITY_NOT_DEPLOYED:inspection.observe"},
		{fmt.Sprintf("\n    with: {assetIds: [%d]}", asset), "TASK_RESOURCE_SCOPE_INVALID"},
	} {
		source := "apiVersion: aerosight/v2\nname: inspection\ntrigger: {type: manual}\nsteps:\n  - key: observe\n    uses: inspection.observe" + tc.with + "\n"
		raw, _ := json.Marshal(gin.H{"sourceFormat": "yaml", "source": source})
		res := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/tasks/validate", pid), string(raw))
		out := decodedResponse(t, res)
		if res.StatusCode != 200 || out["canPublish"] != false {
			t.Fatalf("invalid publication accepted: %+v", out)
		}
		issues := out["issues"].([]any)
		if issues[0].(map[string]any)["code"] != tc.code {
			t.Fatalf("wrong publication failure: %+v", out)
		}
	}
}

func TestTaskAuthoringEnableDisableAuthorization(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	raw, _ := json.Marshal(gin.H{"sourceFormat": "yaml", "source": reportTaskYAML, "idempotencyKey": "state-test"})
	response := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/tasks", pid), string(raw))
	created := decodedResponse(t, response)
	if response.StatusCode != 201 {
		t.Fatal(created)
	}
	tid := int(created["taskId"].(float64))
	vid := created["versionId"]
	path := fmt.Sprintf("/api/projects/%d/tasks/%d", pid, tid)
	patch := func(state string, want int) {
		t.Helper()
		res := f.request(t, "PATCH", path, fmt.Sprintf(`{"status":%q}`, state))
		data := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("state status %d want %d %+v", res.StatusCode, want, data)
		}
	}
	patch("active", 400)
	raw, _ = json.Marshal(gin.H{"action": "publish", "versionId": vid, "expectedRevision": 1})
	response = f.request(t, "POST", path+"/versions", string(raw))
	published := decodedResponse(t, response)
	if response.StatusCode != 200 {
		t.Fatal(published)
	}
	patch("active", 200)
	var authorized int
	if err := f.db.QueryRow("select authorized_by_user_id from tasks where id=$1", tid).Scan(&authorized); err != nil || authorized <= 0 {
		t.Fatal("missing delegate")
	}
	var run int
	if err := f.db.QueryRow("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status) values($1,$2,$3,$4,'manual','running') returning id", pid, team, tid, vid).Scan(&run); err != nil {
		t.Fatal(err)
	}
	patch("disabled", 200)
	var status string
	if err := f.db.QueryRow("select status from task_runs where id=$1", run).Scan(&status); err != nil || status != "running" {
		t.Fatal("disable changed existing run")
	}
	if _, err := f.db.Exec("delete from team_members where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	patch("active", 403)
}

func TestTaskAuthoringInspectionDraftCanPersist(t *testing.T) {
	f := newAPIFixture(t)
	_, pid := f.project(t)
	source := "apiVersion: aerosight/v2\nname: 巡检模板\ntrigger: {type: manual}\nsteps:\n  - key: observe\n    uses: inspection.observe\n    with: {mode: assets, assetIds: []}\n  - key: detect\n    uses: inspection.detect\n    with: {observationId: steps.observe.outputs.observationId, source: external}\n"
	raw, _ := json.Marshal(gin.H{"sourceFormat": "yaml", "source": source, "idempotencyKey": "inspection-draft"})
	res := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/tasks", pid), string(raw))
	out := decodedResponse(t, res)
	if res.StatusCode != 201 {
		t.Fatalf("inspection draft: %d %+v", res.StatusCode, out)
	}
	var count int
	if err := f.db.QueryRow("select count(*) from task_steps where task_version_id=$1 and uses in ('inspection.observe','inspection.detect')", out["versionId"]).Scan(&count); err != nil || count != 2 {
		t.Fatalf("steps: %d %v", count, err)
	}
}

func TestTaskAuthoringScheduleManualTrialHTTP(t *testing.T) {
	f := newAPIFixture(t)
	_, pid := f.project(t)
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, method, path, string(raw))
		out := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s: %d %+v", path, res.StatusCode, out)
		}
		return out
	}
	source := "apiVersion: aerosight/v2\nname: schedule trial\ntrigger: {type: schedule, cron: '0 8 * * *', timezone: Asia/Shanghai, inputs: {count: 2}}\ninputSchema:\n  type: object\n  properties: {count: {type: integer, default: 1}}\nsteps: [{key: report, uses: report.generate}]\n"
	created := call("POST", fmt.Sprintf("/api/projects/%d/tasks", pid), gin.H{"sourceFormat": "yaml", "source": source, "idempotencyKey": "scheduled-trial"}, 201)
	tid := int(created["taskId"].(float64))
	path := fmt.Sprintf("/api/projects/%d/tasks/%d", pid, tid)
	call("POST", path+"/versions", gin.H{"action": "publish", "versionId": created["versionId"], "expectedRevision": 1}, 200)
	call("PATCH", path, gin.H{"status": "active"}, 200)
	invocation := gin.H{"type": "manual", "idempotencyKey": "trial", "occurredAt": "2026-09-11T08:00:00Z", "inputs": gin.H{"count": "bad"}}
	call("POST", path+"/runs", invocation, 400)
	invocation["inputs"] = gin.H{"count": 3}
	run := call("POST", path+"/runs", invocation, 201)
	replay := call("POST", path+"/runs", invocation, 200)
	if replay["taskRunId"] != run["taskRunId"] {
		t.Fatal("manual retry duplicated run")
	}
	var inputs []byte
	var unchanged bool
	if err := f.db.QueryRow("select run.input_snapshot_json,task.schedule_evaluated_at is null from task_runs run join tasks task on task.id=run.task_id where run.id=$1", run["taskRunId"]).Scan(&inputs, &unchanged); err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(inputs, &snapshot); err != nil {
		t.Fatal(err)
	}
	if !unchanged || snapshot["inputs"].(map[string]any)["count"] != float64(3) {
		t.Fatalf("trial changed schedule or input: %s", inputs)
	}
}

func TestTaskV2ExistingFlightInputContract(t *testing.T) {
	definition, _, _, err := parseTaskAuthorInput(map[string]any{"sourceFormat": "yaml", "source": "apiVersion: aerosight/v2\nname: 已有飞行\ntrigger: {type: manual}\nsteps: [{key: observe, uses: inspection.observe, with: {mode: existing-flight, connectorId: 1, flightUuid: flight}}]"})
	if err != nil {
		t.Fatal(err)
	}
	step := fhObject(definition["steps"].([]any)[0])
	for _, tc := range []struct {
		name      string
		extra     map[string]any
		wantError bool
	}{
		{name: "full flight"},
		{name: "finite", extra: map[string]any{"confirmLimitedScope": true, "assetIds": []any{float64(1)}, "scopeDescription": "选定照片"}},
		{name: "missing scope", extra: map[string]any{"confirmLimitedScope": true, "assetIds": []any{float64(1)}}, wantError: true},
		{name: "missing selection", extra: map[string]any{"confirmLimitedScope": true, "scopeDescription": "选定照片"}, wantError: true},
		{name: "empty scope", extra: map[string]any{"confirmLimitedScope": true, "scopeDescription": "   ", "assetIds": []any{float64(1)}}, wantError: true},
		{name: "duplicate", extra: map[string]any{"assetIds": []any{float64(1), float64(1)}}, wantError: true},
		{name: "foreign type", extra: map[string]any{"connectorId": "1"}, wantError: true},
		{name: "limit", extra: map[string]any{"maxImages": float64(1001)}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"mode": "existing-flight", "connectorId": float64(1), "flightUuid": "flight"}
			for k, v := range tc.extra {
				input[k] = v
			}
			_, err := taskdefinition.MergeInputs(taskJSON(step["inputSchema"]), nil, input)
			if (err != nil) != tc.wantError {
				t.Fatalf("validation got %v wantError %v", err, tc.wantError)
			}
		})
	}
}

func TestTaskAuthoringCanonicalObservationContractWithDeferredInputs(t *testing.T) {
	f := newAPIFixture(t)
	_, pid := f.project(t)
	for _, tc := range []struct {
		name, with string
		valid      bool
	}{
		{"assets valid", `{mode: assets, assetIds: inputs.assets, maxImages: 5}`, true},
		{"assets invalid limit", `{mode: assets, assetIds: inputs.assets, maxImages: 0}`, false},
		{"assets missing required", `{mode: assets, maxImages: 5}`, false},
		{"assets unknown field", `{mode: assets, assetIds: inputs.assets, silentlyIgnore: true}`, false},
		{"flight valid", `{mode: existing-flight, connectorId: inputs.connector, flightUuid: inputs.flight}`, true},
		{"flight missing scope", `{mode: existing-flight, connectorId: inputs.connector, flightUuid: inputs.flight, confirmLimitedScope: true}`, false},
		{"flight blank scope", `{mode: existing-flight, connectorId: inputs.connector, flightUuid: inputs.flight, confirmLimitedScope: true, assetIds: inputs.assets, scopeDescription: ' '}`, false},
		{"flight invalid limit", `{mode: existing-flight, connectorId: inputs.connector, flightUuid: inputs.flight, maxImages: 1001}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := `apiVersion: aerosight/v2
name: contract
trigger: {type: manual}
inputSchema:
  type: object
  properties:
    assets: {type: array, items: {type: integer}}
    connector: {type: integer}
    flight: {type: string}
steps:
  - key: observe
    uses: inspection.observe
    inputSchema: {type: object, additionalProperties: true}
    with: ` + tc.with + "\n"
			raw, _ := json.Marshal(gin.H{"sourceFormat": "yaml", "source": source})
			res := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/tasks/validate", pid), string(raw))
			out := decodedResponse(t, res)
			if res.StatusCode != 200 || out["canPublish"] != tc.valid {
				t.Fatalf("validation %d %+v", res.StatusCode, out)
			}
			if !tc.valid && out["issues"].([]any)[0].(map[string]any)["code"] != "TASK_STEP_INPUT_INVALID:observe" {
				t.Fatal(out)
			}
		})
	}
}

func TestTaskAuthoringPipelineContracts(t *testing.T) {
	f := newAPIFixture(t)
	_, pid := f.project(t)
	for _, tc := range []struct {
		name, uses, with string
		valid            bool
	}{
		{"invalid assessment temperature", "copilot.run", `{mode: assessment, evidenceSetId: inputs.reference, temperature: -1}`, false},
		{"invalid assessment temperature type", "copilot.run", `{mode: assessment, evidenceSetId: inputs.reference, temperature: hot}`, false},
		{"native valid", "inspection.detect", `{source: flighthub-ai, observationId: inputs.reference}`, true},
		{"native unknown field", "inspection.detect", `{source: flighthub-ai, observationId: inputs.reference, threshold: 2}`, false},
		{"native missing observation", "inspection.detect", `{source: flighthub-ai}`, false},
		{"issue valid", "issue.create-or-update", `{assessmentId: inputs.reference, priority: high}`, true},
		{"issue invalid priority", "issue.create-or-update", `{assessmentId: inputs.reference, priority: typo}`, false},
		{"issue invalid title", "issue.create-or-update", `{assessmentId: inputs.reference, title: ''}`, false},
		{"report template scope", "report.generate", `{scope: current-run}`, true},
		{"report legacy empty", "report.generate", `{}`, true},
		{"report other scope", "report.generate", `{scope: all-projects}`, false},
		{"report ignored parameters", "report.generate", `{scope: inputs.reference, title: ignored}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := `apiVersion: aerosight/v2
name: pipeline contract
trigger: {type: manual}
inputSchema: {type: object, properties: {reference: {type: string}}}
steps:
  - key: step
    uses: ` + tc.uses + `
    with: ` + tc.with + "\n"
			// Exercise shipped report defaults as well as a deliberately weak author schema.
			if tc.uses != "report.generate" {
				source += "    inputSchema: {type: object, additionalProperties: true}\n"
			}
			raw, _ := json.Marshal(gin.H{"sourceFormat": "yaml", "source": source})
			res := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/tasks/validate", pid), string(raw))
			out := decodedResponse(t, res)
			if res.StatusCode != 200 || out["canPublish"] != tc.valid {
				t.Fatalf("%d %+v", res.StatusCode, out)
			}
			if !tc.valid && out["issues"].([]any)[0].(map[string]any)["code"] != "TASK_STEP_INPUT_INVALID:step" {
				t.Fatal(out)
			}
			if !tc.valid {
				issue := out["issues"].([]any)[0].(map[string]any)
				if issue["stepKey"] != "step" || len(issue["fields"].([]any)) == 0 {
					t.Fatal("missing field diagnostics", out)
				}
				if tc.name == "issue invalid priority" {
					field := issue["fields"].([]any)[0].(map[string]any)
					if field["path"] != "/with/priority" || field["constraint"] != "enum" {
						t.Fatal("wrong field location", field)
					}
				}
			}

		})
	}
}

func TestTaskV2ReferencesCannotInventCapabilityOutputs(t *testing.T) {
	for _, condition := range []bool{false, true} {
		source := `apiVersion: aerosight/v2
name: forged output
trigger: {type: manual}
steps:
  - key: detect
    uses: inspection.detect
    with: {source: external}
    outputSchema: {type: object, properties: {invented: {type: string}}}
  - key: report
    uses: report.generate
`
		if condition {
			source += "    condition: {op: eq, left: {ref: steps.detect.outputs.invented}, right: {value: anything}}\n"
		} else {
			source += "    with: {scope: steps.detect.outputs.invented}\n"
		}
		_, _, _, err := parseTaskAuthorInput(map[string]any{"sourceFormat": "yaml", "source": source})
		if err == nil || !strings.Contains(err.Error(), "TASK_REFERENCE_CAPABILITY_FIELD_MISSING") {
			t.Fatal("wrong invented-output validation", err)
		}
	}
}
func TestTaskAuthorNormalizedFormatsAndV1Compatibility(t *testing.T) {
	yaml := reportTaskYAML
	parsed, _, _, err := parseTaskAuthorInput(map[string]any{"sourceFormat": "yaml", "source": yaml})
	if err != nil {
		t.Fatal(err)
	}
	// Explicit canonical defaults and source key order do not change execution identity.
	raw, _ := json.Marshal(parsed)
	other, _, _, err := parseTaskAuthorInput(map[string]any{"sourceFormat": "json", "source": string(raw)})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := taskdefinition.Hash(parsed)
	b, _ := taskdefinition.Hash(other)
	if a != b {
		t.Fatal("normalized JSON/YAML diverged")
	}

	var legacy map[string]any
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatal(err)
	}
	delete(legacy, "apiVersion")
	delete(legacy["trigger"].(map[string]any), "inputs")
	for _, rawStep := range legacy["steps"].([]any) {
		delete(rawStep.(map[string]any), "capabilityVersion")
	}
	encodedLegacy, _ := json.Marshal(legacy)
	if _, _, _, err := parseTaskAuthorInput(map[string]any{"sourceFormat": "json", "source": string(encodedLegacy)}); err != nil {
		t.Fatal("legacy typed Task rejected", err)
	}

}

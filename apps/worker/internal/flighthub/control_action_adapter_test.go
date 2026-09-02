package flighthub

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aerosight/worker/internal/driver"
)

type controlManifestRow struct {
	ID     string
	Method string
	Path   string
	Status string
	Domain string
	Risk   string
}

func readControlManifest(t *testing.T) map[string]controlManifestRow {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve endpoint manifest")
	}
	file, err := os.Open(filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/endpoints.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = 11
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]controlManifestRow{}
	for _, row := range rows[1:] {
		if row[5] != "control" {
			continue
		}
		result[row[0]] = controlManifestRow{ID: row[0], Method: row[1], Path: row[2], Status: row[3], Domain: row[5], Risk: row[7]}
	}
	return result
}

func TestControlActionAdaptersCoverReleasedManifest(t *testing.T) {
	t.Parallel()
	manifest := readControlManifest(t)
	if len(manifest) != 14 {
		t.Fatalf("control manifest endpoints = %d, want 14", len(manifest))
	}
	covered := map[string]bool{}
	codes := map[string]bool{}
	deviceCommands := map[string]bool{}
	for _, definition := range ControlActionDefinitions() {
		if codes[definition.Code] {
			t.Fatalf("duplicate control action code %s", definition.Code)
		}
		codes[definition.Code] = true
		row, exists := manifest[definition.EndpointID]
		if !exists {
			t.Fatalf("action %s references endpoint absent from control manifest: %s", definition.Code, definition.EndpointID)
		}
		if row.Status != "released" || row.Domain != "control" || row.Method != definition.Method || row.Path != definition.PathTemplate {
			t.Fatalf("action %s drifted from manifest: definition=%#v manifest=%#v", definition.Code, definition, row)
		}
		if string(definition.Risk) != row.Risk {
			t.Fatalf("action %s risk = %s, manifest risk = %s", definition.Code, definition.Risk, row.Risk)
		}
		covered[definition.EndpointID] = true
		if !json.Valid(definition.InputSchema) || !json.Valid(definition.OutputSchema) {
			t.Fatalf("action %s has invalid JSON schema", definition.Code)
		}
		var inputSchema map[string]json.RawMessage
		if json.Unmarshal(definition.InputSchema, &inputSchema) != nil || string(inputSchema["type"]) != `"object"` || string(inputSchema["additionalProperties"]) != "false" {
			t.Fatalf("action %s input schema is not a closed object", definition.Code)
		}
		if definition.ResultSemantics == "" {
			t.Fatalf("action %s lacks result semantics", definition.Code)
		}
		if definition.Method != http.MethodGet {
			if definition.DefaultEnabled || (definition.Risk != driver.RiskHigh && definition.Risk != driver.RiskCritical) {
				t.Fatalf("mutation %s does not fail closed: %#v", definition.Code, definition)
			}
			if definition.FinalOnHTTPSuccess {
				t.Fatalf("mutation %s treats HTTP success as a final physical result", definition.Code)
			}
		}
		if definition.EndpointID == "454273417e0" {
			deviceCommands[definition.Code] = true
			if definition.ResultSemantics != ControlResultAcceptanceOnly || definition.FinalOnHTTPSuccess {
				t.Fatalf("device command %s must remain acceptance-only", definition.Code)
			}
		}
	}
	for endpointID := range manifest {
		if !covered[endpointID] {
			t.Fatalf("released control endpoint %s has no adapter", endpointID)
		}
	}
	wantCommands := []string{"return_home", "return_home_cancel", "flighttask_pause", "flighttask_recovery"}
	if len(deviceCommands) != len(wantCommands) {
		t.Fatalf("device command adapters = %#v", deviceCommands)
	}
	for _, code := range wantCommands {
		if !deviceCommands[code] {
			t.Fatalf("missing device command adapter %s", code)
		}
	}
}

func TestBuildControlActionRequests(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code         string
		input        string
		method       string
		path         string
		query        string
		body         string
		disableRetry bool
	}{
		{"return_home", `{"deviceSn":"AIRCRAFT_REDACTED"}`, http.MethodPost, "/openapi/v2.0/device/AIRCRAFT_REDACTED/command", "", `{"device_command":"return_home"}`, true},
		{"return_home_cancel", `{"deviceSn":"AIRCRAFT_REDACTED"}`, http.MethodPost, "/openapi/v2.0/device/AIRCRAFT_REDACTED/command", "", `{"device_command":"return_home_cancel"}`, true},
		{"flighttask_pause", `{"deviceSn":"AIRCRAFT_REDACTED"}`, http.MethodPost, "/openapi/v2.0/device/AIRCRAFT_REDACTED/command", "", `{"device_command":"flighttask_pause"}`, true},
		{"flighttask_recovery", `{"deviceSn":"AIRCRAFT_REDACTED"}`, http.MethodPost, "/openapi/v2.0/device/AIRCRAFT_REDACTED/command", "", `{"device_command":"flighttask_recovery"}`, true},
		{"control.status.organization", `{"organizationUuid":"ORG_REDACTED","deviceControlMethod":"flight_control","deviceSn":"DOCK_REDACTED"}`, http.MethodGet, "/openapi/v2.0/organizations/ORG_REDACTED/manage-devices/cmds/control/status", "device_control_method=flight_control&device_sn=DOCK_REDACTED", "", false},
		{"command.status.organization", `{"organizationUuid":"ORG_REDACTED","deviceSns":["DOCK_REDACTED"],"identifiers":["COMMAND_REDACTED"]}`, http.MethodGet, "/openapi/v2.0/organizations/ORG_REDACTED/manage-devices/cmds", "device_sn=DOCK_REDACTED&identifiers=COMMAND_REDACTED", "", false},
		{"tca.status", `{"workspaceId":"WORKSPACE_REDACTED"}`, http.MethodGet, "/openapi/v2.0/workspaces/WORKSPACE_REDACTED/groups/tcas", "", "", false},
		{"cloud_control.status", `{"droneSns":["AIRCRAFT_REDACTED"]}`, http.MethodGet, "/openapi/v2.0/cloud-controls", "drone_sn_list=AIRCRAFT_REDACTED", "", false},
		{"camera.change_lens", `{"sn":"AIRCRAFT_REDACTED","cameraIndex":"CAMERA_REDACTED","lensType":"wide"}`, http.MethodPost, "/openapi/v2.0/device/change-lens", "", `{"sn":"AIRCRAFT_REDACTED","camera_index":"CAMERA_REDACTED","lens_type":"wide"}`, true},
		{"camera.change", `{"sn":"DOCK_REDACTED","cameraIndex":"CAMERA_REDACTED","cameraPosition":"outdoor"}`, http.MethodPost, "/openapi/v2.0/device/change-camera", "", `{"sn":"DOCK_REDACTED","camera_index":"CAMERA_REDACTED","camera_position":"outdoor"}`, true},
		{"rtk.calibrate", `{"deviceSn":"DOCK_REDACTED","host":"ntrip.invalid","port":8002,"account":"ACCOUNT_REDACTED","password":"PASSWORD_REDACTED","mountPoint":"MOUNT_REDACTED"}`, http.MethodPost, "/openapi/v2.0/device/DOCK_REDACTED/rtk", "", `{"host":"ntrip.invalid","port":8002,"account":"ACCOUNT_REDACTED","password":"PASSWORD_REDACTED","mount_point":"MOUNT_REDACTED"}`, true},
		{"relay.pair", `{"deviceSn":"RELAY_REDACTED","pairEnable":true,"pairType":"drone"}`, http.MethodPost, "/openapi/v2.0/device/relay_model", "", `{"device_sn":"RELAY_REDACTED","pair_enable":true,"pair_type":"drone"}`, true},
		{"relay.status", `{"deviceSn":"RELAY_REDACTED"}`, http.MethodGet, "/openapi/v2.0/device/RELAY_REDACTED/relay_model", "", "", false},
		{"active_project.update", `{"activeProjectUuid":"PROJECT_REDACTED","deviceSn":"DOCK_REDACTED"}`, http.MethodPut, "/openapi/v2.0/device/active-project", "", `{"active_project_uuid":"PROJECT_REDACTED","device_sn":"DOCK_REDACTED"}`, true},
		{"control.status.project", `{"deviceControlMethod":"flight_control","deviceSn":"DOCK_REDACTED"}`, http.MethodGet, "/openapi/v2.0/topologies/cmds/control/status", "device_control_method=flight_control&device_sn=DOCK_REDACTED", "", false},
		{"control.acquire", `{"droneSn":"AIRCRAFT_REDACTED","flight":true,"payloadIndex":["CAMERA_REDACTED"]}`, http.MethodPost, "/openapi/v2.0/device/control", "", `{"drone_sn":"AIRCRAFT_REDACTED","flight":true,"payload_index":["CAMERA_REDACTED"]}`, true},
		{"control.release", `{"droneSn":"AIRCRAFT_REDACTED","payloadIndex":["CAMERA_REDACTED"]}`, http.MethodDelete, "/openapi/v2.0/device/control", "", `{"drone_sn":"AIRCRAFT_REDACTED","payload_index":["CAMERA_REDACTED"]}`, true},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.code, func(t *testing.T) {
			t.Parallel()
			request, err := BuildControlActionRequest(testCase.code, json.RawMessage(testCase.input))
			if err != nil {
				t.Fatal(err)
			}
			if request.Spec.Method != testCase.method || request.Spec.Path != testCase.path || request.Spec.Query.Encode() != testCase.query || request.Spec.DisableRetry != testCase.disableRetry {
				t.Fatalf("unexpected request spec: %#v", request.Spec)
			}
			if testCase.body == "" {
				if request.Spec.Body != nil {
					t.Fatalf("unexpected body: %#v", request.Spec.Body)
				}
				return
			}
			body, err := json.Marshal(request.Spec.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != testCase.body {
				t.Fatalf("body = %s, want %s", body, testCase.body)
			}
		})
	}
}

func TestControlActionInputsFailClosed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		code  string
		input string
	}{
		{"unknown action", "future.control", `{}`},
		{"unknown field", "return_home", `{"deviceSn":"AIRCRAFT_REDACTED","device_command":"future"}`},
		{"path escape", "return_home", `{"deviceSn":"../escape"}`},
		{"duplicate serial", "cloud_control.status", `{"droneSns":["AIRCRAFT_REDACTED","AIRCRAFT_REDACTED"]}`},
		{"missing relay enable", "relay.pair", `{"deviceSn":"RELAY_REDACTED","pairType":"drone"}`},
		{"unknown relay side", "relay.pair", `{"deviceSn":"RELAY_REDACTED","pairEnable":true,"pairType":"future"}`},
		{"control no-op", "control.acquire", `{"droneSn":"AIRCRAFT_REDACTED"}`},
		{"invalid rtk port", "rtk.calibrate", `{"deviceSn":"DOCK_REDACTED","host":"ntrip.invalid","port":0,"account":"a","password":"p","mountPoint":"m"}`},
		{"rtk host credentials", "rtk.calibrate", `{"deviceSn":"DOCK_REDACTED","host":"user@ntrip.invalid","port":8002,"account":"a","password":"p","mountPoint":"m"}`},
		{"trailing object", "return_home", `{"deviceSn":"AIRCRAFT_REDACTED"}{}`},
	}
	for _, testCase := range cases {
		if _, err := BuildControlActionRequest(testCase.code, json.RawMessage(testCase.input)); !IsSafeCode(err, "request_invalid") {
			t.Errorf("%s error = %v, want request_invalid", testCase.name, err)
		}
	}
}

func TestControlActionOutputDecodersPreserveFinality(t *testing.T) {
	t.Parallel()
	if _, err := DecodeControlActionOutput("return_home", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeControlActionOutput("return_home", json.RawMessage(`{"unexpected":true}`)); !IsSafeCode(err, "schema_incompatible") {
		t.Fatalf("unexpected device command output error: %v", err)
	}
	rtk, err := DecodeControlActionOutput("rtk.calibrate", json.RawMessage(`{"bid":"COMMAND_REDACTED","status":"ok"}`))
	if err != nil || rtk.(RTKCalibrationOutput).BusinessID != "COMMAND_REDACTED" {
		t.Fatalf("unexpected RTK output: %#v err=%v", rtk, err)
	}
	if _, err := DecodeControlActionOutput("rtk.calibrate", json.RawMessage(`{"bid":"COMMAND_REDACTED","status":"completed"}`)); !IsSafeCode(err, "schema_incompatible") {
		t.Fatalf("RTK final-state drift was accepted: %v", err)
	}
	relay, err := DecodeControlActionOutput("relay.status", json.RawMessage(`{"status":"ok","output":{"status":2},"bid":"COMMAND_REDACTED"}`))
	if err != nil || relay.(RelayPairingOutput).Output.Status != 2 {
		t.Fatalf("unexpected relay output: %#v err=%v", relay, err)
	}
	cloud, err := DecodeControlActionOutput("cloud_control.status", json.RawMessage(`{"roles":[]}`))
	if err != nil || cloud.(OpenControlOutputSummary).FieldCount != 1 {
		t.Fatalf("unexpected cloud-control summary: %#v err=%v", cloud, err)
	}
	tca, err := DecodeControlActionOutput("tca.status", json.RawMessage(`[{"vendor_field":"ignored"}]`))
	if err != nil || tca.(OpenControlOutputSummary).ItemCount != 1 {
		t.Fatalf("unexpected TCA summary: %#v err=%v", tca, err)
	}
	ownership, err := DecodeControlActionOutput("control.acquire", json.RawMessage(`{"drone_sn":"AIRCRAFT_REDACTED","controls":[{"type":"flight","gateway":{"sn":"DOCK_REDACTED"},"user":{"call_sign":"OPERATOR_REDACTED","user_id":"USER_REDACTED","type":"cloud"}}]}`))
	if err != nil || len(ownership.(ControlOwnershipOutput).Controls) != 1 {
		t.Fatalf("unexpected ownership output: %#v err=%v", ownership, err)
	}
}

func TestControlActionSchemasMarkOnlyDocumentedOpenOutputs(t *testing.T) {
	t.Parallel()
	open := map[string]bool{}
	for _, definition := range ControlActionDefinitions() {
		if strings.Contains(string(definition.OutputSchema), `"x-vendor-schema-open":true`) {
			open[definition.Code] = true
		}
		if strings.Contains(strings.ToLower(string(definition.OutputSchema)), "password") {
			t.Fatalf("action %s output schema exposes a credential field", definition.Code)
		}
	}
	if len(open) != 2 || !open["tca.status"] || !open["cloud_control.status"] {
		t.Fatalf("unexpected vendor-open outputs: %#v", open)
	}
	for _, definition := range ControlActionDefinitions() {
		if definition.Code == "rtk.calibrate" {
			if len(definition.SensitiveInputFields) != 1 || definition.SensitiveInputFields[0] != "password" || !strings.Contains(string(definition.InputSchema), `"writeOnly":true`) {
				t.Fatalf("RTK password classification missing: %#v", definition)
			}
			return
		}
	}
	t.Fatal("missing RTK adapter")
}

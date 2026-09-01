package observability

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperationalMetricCatalogCoversRequiredSignals(t *testing.T) {
	registry := MustDefaultMetricRegistry()
	traffic := []struct {
		name   string
		value  float64
		labels map[string]string
	}{
		{"aerosight_device_connection_transitions_total", 1, map[string]string{"state": "degraded", "reason": "adapter"}},
		{"aerosight_ingest_latency_seconds", 0.2, map[string]string{"event_type": "pose", "outcome": "accepted"}},
		{"aerosight_outbox_deliveries_total", 1, map[string]string{"event_family": "mission", "outcome": "consumed"}},
		{"aerosight_command_ack_latency_seconds", 1.4, map[string]string{"adapter_type": "dji", "outcome": "nack"}},
		{"aerosight_live_stream_transitions_total", 1, map[string]string{"adapter_type": "dji", "state": "live"}},
		{"aerosight_algorithm_latency_seconds", 3.1, map[string]string{"adapter_type": "http-json", "outcome": "succeeded"}},
		{"aerosight_task_step_transitions_total", 1, map[string]string{"uses": "algorithm.run", "outcome": "succeeded"}},
		{"aerosight_issue_creation_latency_seconds", 0.4, map[string]string{"source": "task", "outcome": "created"}},
		{"aerosight_sse_connections_total", 1, map[string]string{"outcome": "resumed"}},
		{"aerosight_copilot_tool_rejections_total", 1, map[string]string{"tool": "request_mission_start", "reason": "confirmation"}},
		{"aerosight_report_failures_total", 1, map[string]string{"operation": "export", "reason": "storage"}},
		{"aerosight_connector_sync_total", 1, map[string]string{"connector": "dji_flighthub2", "outcome": "schema_incompatible"}},
		{"aerosight_connector_sync_duration_seconds", 2.4, map[string]string{"connector": "dji_flighthub2", "outcome": "failed"}},
		{"aerosight_connector_sync_backlog", 3, map[string]string{"connector": "dji_flighthub2"}},
	}
	for _, sample := range traffic {
		if err := registry.Record(sample.name, sample.value, sample.labels); err != nil {
			t.Fatalf("record %s: %v", sample.name, err)
		}
	}
	recorder := httptest.NewRecorder()
	registry.ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	for _, sample := range traffic {
		if !strings.Contains(body, sample.name) {
			t.Fatalf("metrics output lacks %s", sample.name)
		}
	}
}

func TestGaugeRecordsCurrentValueInsteadOfAccumulating(t *testing.T) {
	registry := MustDefaultMetricRegistry()
	labels := map[string]string{"connector": "dji_flighthub2"}
	if err := registry.Record("aerosight_connector_sync_backlog", 9, labels); err != nil {
		t.Fatal(err)
	}
	if err := registry.Record("aerosight_connector_sync_backlog", 2, labels); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	registry.ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(recorder.Body.String(), `aerosight_connector_sync_backlog{connector="dji_flighthub2"} 2`) {
		t.Fatalf("gauge did not retain current value: %s", recorder.Body.String())
	}
}

func TestMetricLabelsRejectSecretsAndHighCardinalityTraffic(t *testing.T) {
	registry := MustDefaultMetricRegistry()
	secrets := []string{"Bearer sandbox-secret-token", "1581FABCDEFGHIJKL", "temporary-security-token", "https://objects.example/item?signature=signed-value"}
	identifier := "86c6a355-f1d8-4893-9147-cb93082ef7b0"
	unsafeLabels := []map[string]string{
		{"outcome": identifier},
		{"outcome": "opened", "project_id": "17"},
	}
	for _, secret := range secrets {
		unsafeLabels = append(unsafeLabels, map[string]string{"outcome": secret})
	}
	for _, labels := range unsafeLabels {
		if err := registry.Record("aerosight_sse_connections_total", 1, labels); err == nil {
			t.Fatalf("unsafe metric labels accepted: %#v", labels)
		}
	}
	recorder := httptest.NewRecorder()
	registry.ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	for _, secret := range append(secrets, identifier, "project_id") {
		if strings.Contains(body, secret) {
			t.Fatalf("rejected high-cardinality or sensitive data reached metrics output: %s", body)
		}
	}
}

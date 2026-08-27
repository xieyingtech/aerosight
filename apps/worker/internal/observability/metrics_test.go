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
		{"aerosight_alert_automation_total", 1, map[string]string{"mode": "draft_only", "outcome": "drafted"}},
		{"aerosight_sse_connections_total", 1, map[string]string{"outcome": "resumed"}},
		{"aerosight_ai_tool_rejections_total", 1, map[string]string{"tool": "request_mission_start", "reason": "confirmation"}},
		{"aerosight_report_failures_total", 1, map[string]string{"operation": "export", "reason": "storage"}},
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

func TestMetricLabelsRejectSecretsAndHighCardinalityTraffic(t *testing.T) {
	registry := MustDefaultMetricRegistry()
	secret := "Bearer sandbox-secret-token"
	identifier := "86c6a355-f1d8-4893-9147-cb93082ef7b0"
	for _, labels := range []map[string]string{
		{"outcome": secret},
		{"outcome": identifier},
		{"outcome": "opened", "project_id": "17"},
	} {
		if err := registry.Record("aerosight_sse_connections_total", 1, labels); err == nil {
			t.Fatalf("unsafe metric labels accepted: %#v", labels)
		}
	}
	recorder := httptest.NewRecorder()
	registry.ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	if strings.Contains(body, secret) || strings.Contains(body, identifier) || strings.Contains(body, "project_id") {
		t.Fatalf("rejected high-cardinality data reached metrics output: %s", body)
	}
}

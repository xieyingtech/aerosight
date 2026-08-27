package observability

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type MetricKind string

const (
	Counter   MetricKind = "counter"
	Histogram MetricKind = "histogram"
)

type MetricDefinition struct {
	Name          string
	Help          string
	Kind          MetricKind
	AllowedLabels map[string][]string
}

func DefaultMetricDefinitions() []MetricDefinition {
	return []MetricDefinition{
		{Name: "aerosight_device_connection_transitions_total", Help: "Canonical device connection transitions.", Kind: Counter, AllowedLabels: labels("state", "online,degraded,offline,unknown", "reason", "heartbeat,adapter,error,manual,unknown")},
		{Name: "aerosight_ingest_latency_seconds", Help: "Adapter telemetry ingest latency.", Kind: Histogram, AllowedLabels: labels("event_type", "pose,battery,heartbeat,connection", "outcome", "accepted,duplicate,rejected,scope_mismatch")},
		{Name: "aerosight_outbox_deliveries_total", Help: "Outbox delivery outcomes.", Kind: Counter, AllowedLabels: labels("event_family", "mission,media,algorithm,alert,other", "outcome", "consumed,retried,dead_letter")},
		{Name: "aerosight_command_ack_latency_seconds", Help: "Device command acknowledgement latency.", Kind: Histogram, AllowedLabels: labels("adapter_type", "simulator,dji", "outcome", "ack,nack,timeout,unknown")},
		{Name: "aerosight_live_stream_transitions_total", Help: "Live stream state transitions.", Kind: Counter, AllowedLabels: labels("adapter_type", "simulator,dji", "state", "starting,live,degraded,stopping,stopped,failed")},
		{Name: "aerosight_algorithm_latency_seconds", Help: "External algorithm execution latency.", Kind: Histogram, AllowedLabels: labels("adapter_type", "http-json", "outcome", "succeeded,failed,timed_out,rate_limited")},
		{Name: "aerosight_alert_automation_total", Help: "Alert automation outcomes.", Kind: Counter, AllowedLabels: labels("mode", "manual,draft_only,auto", "outcome", "drafted,skipped,failed")},
		{Name: "aerosight_sse_connections_total", Help: "Project SSE connection outcomes.", Kind: Counter, AllowedLabels: labels("outcome", "opened,resumed,closed,rejected")},
		{Name: "aerosight_ai_tool_rejections_total", Help: "AI tool requests rejected before effects.", Kind: Counter, AllowedLabels: labels("tool", "query_devices,query_missions,query_alerts,query_media,query_tracks,query_map_context,create_report_draft,create_issue_draft,request_mission_start", "reason", "permission,scope,schema,confirmation,kill_switch,prompt_injection")},
		{Name: "aerosight_report_failures_total", Help: "Generated report failures.", Kind: Counter, AllowedLabels: labels("operation", "aggregate,publish,export", "reason", "incomplete,data_query,storage,authorization,unknown")},
	}
}

func labels(values ...string) map[string][]string {
	result := make(map[string][]string, len(values)/2)
	for index := 0; index < len(values); index += 2 {
		result[values[index]] = strings.Split(values[index+1], ",")
	}
	return result
}

type metricSample struct {
	labels  map[string]string
	count   float64
	value   float64
	buckets map[float64]float64
}

type Registry struct {
	mu          sync.RWMutex
	definitions map[string]MetricDefinition
	samples     map[string]map[string]*metricSample
}

func NewMetricRegistry(definitions []MetricDefinition) (*Registry, error) {
	registry := &Registry{definitions: map[string]MetricDefinition{}, samples: map[string]map[string]*metricSample{}}
	for _, definition := range definitions {
		if definition.Name == "" || definition.Help == "" || (definition.Kind != Counter && definition.Kind != Histogram) {
			return nil, errors.New("invalid metric definition")
		}
		if _, duplicate := registry.definitions[definition.Name]; duplicate {
			return nil, fmt.Errorf("duplicate metric definition %q", definition.Name)
		}
		registry.definitions[definition.Name] = definition
		registry.samples[definition.Name] = map[string]*metricSample{}
	}
	return registry, nil
}

func MustDefaultMetricRegistry() *Registry {
	registry, err := NewMetricRegistry(DefaultMetricDefinitions())
	if err != nil {
		panic(err)
	}
	return registry
}

var DefaultMetrics = MustDefaultMetricRegistry()

var defaultHistogramBuckets = []float64{0.1, 0.5, 1, 2, 5, 10, 30}

func (registry *Registry) Record(name string, value float64, provided map[string]string) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	definition, ok := registry.definitions[name]
	if !ok {
		return errors.New("METRIC_NOT_DECLARED")
	}
	if value < 0 {
		return errors.New("METRIC_VALUE_INVALID")
	}
	if len(provided) != len(definition.AllowedLabels) {
		return errors.New("METRIC_LABEL_SET_INVALID")
	}
	keys := make([]string, 0, len(provided))
	for key, value := range provided {
		allowed, declared := definition.AllowedLabels[key]
		if !declared || !contains(allowed, value) {
			return errors.New("METRIC_LABEL_VALUE_REJECTED")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+provided[key])
	}
	key := strings.Join(parts, ",")
	sample := registry.samples[name][key]
	if sample == nil {
		sample = &metricSample{labels: cloneLabels(provided), buckets: map[float64]float64{}}
		registry.samples[name][key] = sample
	}
	if definition.Kind == Counter {
		sample.value += value
	} else {
		sample.count++
		sample.value += value
		for _, boundary := range defaultHistogramBuckets {
			if value <= boundary {
				sample.buckets[boundary]++
			}
		}
	}
	return nil
}

func (registry *Registry) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	names := make([]string, 0, len(registry.definitions))
	for name := range registry.definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		definition := registry.definitions[name]
		_, _ = fmt.Fprintf(writer, "# HELP %s %s\n# TYPE %s %s\n", name, definition.Help, name, definition.Kind)
		keys := make([]string, 0, len(registry.samples[name]))
		for key := range registry.samples[name] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			sample := registry.samples[name][key]
			labelText := renderLabels(sample.labels)
			if definition.Kind == Counter {
				_, _ = fmt.Fprintf(writer, "%s%s %s\n", name, labelText, strconv.FormatFloat(sample.value, 'f', -1, 64))
			} else {
				for _, boundary := range defaultHistogramBuckets {
					_, _ = fmt.Fprintf(writer, "%s_bucket%s %s\n", name, renderLabelsWith(sample.labels, "le", strconv.FormatFloat(boundary, 'f', -1, 64)), strconv.FormatFloat(sample.buckets[boundary], 'f', -1, 64))
				}
				_, _ = fmt.Fprintf(writer, "%s_bucket%s %s\n", name, renderLabelsWith(sample.labels, "le", "+Inf"), strconv.FormatFloat(sample.count, 'f', -1, 64))
				_, _ = fmt.Fprintf(writer, "%s_count%s %s\n%s_sum%s %s\n", name, labelText, strconv.FormatFloat(sample.count, 'f', -1, 64), name, labelText, strconv.FormatFloat(sample.value, 'f', -1, 64))
			}
		}
	}
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func cloneLabels(labels map[string]string) map[string]string {
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}

func renderLabels(labels map[string]string) string {
	return renderLabelsWith(labels, "", "")
}

func renderLabelsWith(labels map[string]string, extraKey, extraValue string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(`%s=%q`, key, labels[key]))
	}
	if extraKey != "" {
		parts = append(parts, fmt.Sprintf(`%s=%q`, extraKey, extraValue))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

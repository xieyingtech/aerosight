package observability

import (
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"math"
	"net/http"
	"sort"
	"strings"
)

type MetricKind string

const (
	Counter   MetricKind = "counter"
	Histogram MetricKind = "histogram"
	Gauge     MetricKind = "gauge"
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
		{Name: "aerosight_task_step_transitions_total", Help: "Typed task step execution outcomes.", Kind: Counter, AllowedLabels: labels("uses", "device.command,device.collect,algorithm.run,issue.create-or-update,copilot.run,report.generate", "outcome", "started,succeeded,failed,skipped,paused,retried")},
		{Name: "aerosight_issue_creation_latency_seconds", Help: "Task issue create or update latency.", Kind: Histogram, AllowedLabels: labels("source", "task,manual", "outcome", "created,updated,failed")},
		{Name: "aerosight_sse_connections_total", Help: "Project SSE connection outcomes.", Kind: Counter, AllowedLabels: labels("outcome", "opened,resumed,closed,rejected")},
		{Name: "aerosight_copilot_tool_rejections_total", Help: "Copilot tool requests rejected before effects.", Kind: Counter, AllowedLabels: labels("tool", "query_devices,query_missions,query_issues,query_media,query_tracks,query_map_context,create_report_draft,create_issue_draft,request_mission_start", "reason", "permission,scope,schema,confirmation,kill_switch,prompt_injection")},
		{Name: "aerosight_report_failures_total", Help: "Generated report failures.", Kind: Counter, AllowedLabels: labels("operation", "aggregate,publish,export", "reason", "incomplete,data_query,storage,authorization,unknown")},
		{Name: "aerosight_connector_sync_total", Help: "Connector synchronization outcomes.", Kind: Counter, AllowedLabels: labels("connector", "dji_flighthub2", "outcome", "succeeded,failed,rate_limited,credential_invalid,schema_incompatible,directory_incomplete,lease_lost")},
		{Name: "aerosight_connector_sync_duration_seconds", Help: "Connector synchronization duration.", Kind: Histogram, AllowedLabels: labels("connector", "dji_flighthub2", "outcome", "succeeded,failed")},
		{Name: "aerosight_connector_sync_backlog", Help: "Pending connector synchronization requests.", Kind: Gauge, AllowedLabels: labels("connector", "dji_flighthub2")},
	}
}

func labels(values ...string) map[string][]string {
	result := make(map[string][]string, len(values)/2)
	for index := 0; index < len(values); index += 2 {
		result[values[index]] = strings.Split(values[index+1], ",")
	}
	return result
}

type Registry struct {
	definitions map[string]MetricDefinition
	counters    map[string]*prometheus.CounterVec
	gauges      map[string]*prometheus.GaugeVec
	histograms  map[string]*prometheus.HistogramVec
	registry    *prometheus.Registry
	handler     http.Handler
}

var defaultHistogramBuckets = []float64{0.1, 0.5, 1, 2, 5, 10, 30}

func NewMetricRegistry(definitions []MetricDefinition) (*Registry, error) {
	r := &Registry{definitions: map[string]MetricDefinition{}, counters: map[string]*prometheus.CounterVec{}, gauges: map[string]*prometheus.GaugeVec{}, histograms: map[string]*prometheus.HistogramVec{}, registry: prometheus.NewRegistry()}
	for _, d := range definitions {
		if d.Name == "" || d.Help == "" || (d.Kind != Counter && d.Kind != Gauge && d.Kind != Histogram) {
			return nil, errors.New("invalid metric definition")
		}
		if _, ok := r.definitions[d.Name]; ok {
			return nil, fmt.Errorf("duplicate metric definition %q", d.Name)
		}
		r.definitions[d.Name] = d
		keys := make([]string, 0, len(d.AllowedLabels))
		for k := range d.AllowedLabels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var collector prometheus.Collector
		switch d.Kind {
		case Counter:
			v := prometheus.NewCounterVec(prometheus.CounterOpts{Name: d.Name, Help: d.Help}, keys)
			r.counters[d.Name] = v
			collector = v
		case Gauge:
			v := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: d.Name, Help: d.Help}, keys)
			r.gauges[d.Name] = v
			collector = v
		case Histogram:
			v := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: d.Name, Help: d.Help, Buckets: defaultHistogramBuckets}, keys)
			r.histograms[d.Name] = v
			collector = v
		}
		if err := r.registry.Register(collector); err != nil {
			return nil, err
		}
	}
	r.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	r.handler = promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
	return r, nil
}
func MustDefaultMetricRegistry() *Registry {
	r, err := NewMetricRegistry(DefaultMetricDefinitions())
	if err != nil {
		panic(err)
	}
	return r
}

var DefaultMetrics = MustDefaultMetricRegistry()

func (r *Registry) Gatherer() prometheus.Gatherer { return r.registry }

func (r *Registry) Record(name string, value float64, labels map[string]string) error {
	d, ok := r.definitions[name]
	if !ok {
		return errors.New("METRIC_NOT_DECLARED")
	}
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return errors.New("METRIC_VALUE_INVALID")
	}
	if len(labels) != len(d.AllowedLabels) {
		return errors.New("METRIC_LABEL_SET_INVALID")
	}
	for key, v := range labels {
		allowed, ok := d.AllowedLabels[key]
		if !ok || !contains(allowed, v) {
			return errors.New("METRIC_LABEL_VALUE_REJECTED")
		}
	}
	switch d.Kind {
	case Counter:
		r.counters[name].With(labels).Add(value)
	case Gauge:
		r.gauges[name].With(labels).Set(value)
	case Histogram:
		r.histograms[name].With(labels).Observe(value)
	}
	return nil
}
func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) { r.handler.ServeHTTP(w, req) }
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

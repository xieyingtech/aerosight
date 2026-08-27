package algorithm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryRecorder struct {
	mu       sync.Mutex
	attempts []Attempt
}

func (recorder *memoryRecorder) RecordAttempt(_ context.Context, attempt Attempt) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.attempts = append(recorder.attempts, attempt)
	return nil
}

type recordingSleeper struct{ calls []time.Duration }

func (sleeper *recordingSleeper) Sleep(context.Context, time.Duration) error {
	sleeper.calls = append(sleeper.calls, 1)
	return nil
}

func validRequest(endpoint string) Request {
	return Request{
		Endpoint: endpoint,
		Input: Input{
			SchemaVersion: InputSchemaVersionV1,
			RunID:         "0f98d89b-d901-44f7-a1d4-4f28fd365d63",
			ProjectID:     3,
			Definition:    DefinitionReference{DefinitionVersionID: 9, ProviderType: "http-json", ModelOrProcess: "detector", ExecutionMode: "synchronous", MappingVersion: "v1"},
			InputAsset:    AssetReference{AssetID: 7, Version: 2, ChecksumSHA256: strings.Repeat("a", 64), MIMEType: "image/jpeg", AccessURL: "https://assets.example.test/signed/image.jpg?signature=secret", AccessExpiresAt: time.Now().Add(time.Hour)},
			Context:       map[string]any{"capturedAt": time.Now().UTC().Format(time.RFC3339)}, Parameters: map[string]any{"threshold": 0.7},
		},
		Mapping: Mapping{DetectionsPath: "results", KeyPath: "id", LabelPath: "class", ConfidencePath: "score", GeometryPath: "bbox"},
		Timeout: time.Second,
	}
}

func TestHTTPJSONAdapterSendsPresignedURLAndMapsSynchronousResponse(t *testing.T) {
	var received map[string]any
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":[{"id":"d-1","class":"suspected-construction","score":0.91,"bbox":{"type":"bbox","x":1,"y":2,"width":3,"height":4}}]}`))
	}))
	defer server.Close()
	recorder := &memoryRecorder{}
	adapter := NewHTTPJSONAdapter(server.Client(), recorder, nil)
	outcome, err := adapter.Execute(context.Background(), validRequest(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Kind != "completed" || len(outcome.Detections) != 1 || outcome.Detections[0].Label != "suspected-construction" {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	inputAsset := received["inputAsset"].(map[string]any)
	if !strings.Contains(inputAsset["accessUrl"].(string), "signature=secret") {
		t.Fatal("provider did not receive presigned asset URL")
	}
	encoded, _ := json.Marshal(received)
	if strings.Contains(string(encoded), "base64") {
		t.Fatal("asset bytes must not be embedded in provider request")
	}
	if len(recorder.attempts) != 1 || recorder.attempts[0].Status != "succeeded" || recorder.attempts[0].RequestHash == "" {
		t.Fatalf("attempt was not audited: %+v", recorder.attempts)
	}
}

func TestHTTPJSONAdapterAcceptsAsynchronousJob(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Retry-After", "4")
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte(`{"externalJobId":"job-42"}`))
	}))
	defer server.Close()
	request := validRequest(server.URL)
	request.Input.Definition.ExecutionMode = "asynchronous"
	outcome, err := NewHTTPJSONAdapter(server.Client(), nil, nil).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Kind != "accepted" || outcome.ExternalJobID != "job-42" || outcome.NextPollAt.IsZero() {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
}

func TestHTTPJSONAdapterWaitsForSignedCallback(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte(`{"externalJobId":"callback-job-7"}`))
	}))
	defer server.Close()
	request := validRequest(server.URL)
	request.Input.Definition.ExecutionMode = "callback"
	request.Input.Callback = map[string]string{"url": "https://aerosight.example.test/callbacks/algorithms/run", "token": strings.Repeat("t", 32)}
	outcome, err := NewHTTPJSONAdapter(server.Client(), nil, nil).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Kind != "waiting_callback" || outcome.ExternalJobID != "callback-job-7" {
		t.Fatalf("unexpected callback outcome: %+v", outcome)
	}
}

func TestHTTPJSONAdapterAuditsTimeout(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(80 * time.Millisecond)
		_, _ = writer.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()
	recorder := &memoryRecorder{}
	request := validRequest(server.URL)
	request.Timeout = 10 * time.Millisecond
	_, err := NewHTTPJSONAdapter(server.Client(), recorder, nil).Execute(context.Background(), request)
	if err == nil {
		t.Fatal("expected timeout")
	}
	if len(recorder.attempts) != 1 || recorder.attempts[0].Status != "timed_out" || recorder.attempts[0].ErrorCategory != "timeout" {
		t.Fatalf("timeout was not audited: %+v", recorder.attempts)
	}
}

func TestHTTPJSONAdapterBacksOffAfterRateLimit(t *testing.T) {
	var calls int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = writer.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()
	recorder := &memoryRecorder{}
	sleeper := &recordingSleeper{}
	adapter := NewHTTPJSONAdapter(server.Client(), recorder, nil)
	adapter.sleeper = sleeper
	outcome, err := adapter.Execute(context.Background(), validRequest(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Kind != "completed" || calls != 2 || len(sleeper.calls) != 1 {
		t.Fatalf("rate limit retry failed: calls=%d sleeps=%d", calls, len(sleeper.calls))
	}
	if recorder.attempts[0].Status != "rate_limited" || recorder.attempts[1].Status != "succeeded" {
		t.Fatalf("attempt audit mismatch: %+v", recorder.attempts)
	}
}

func TestHTTPJSONAdapterReportsFormatDriftWithRawResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"predictions":[{"category":"changed"}]}`))
	}))
	defer server.Close()
	recorder := &memoryRecorder{}
	outcome, err := NewHTTPJSONAdapter(server.Client(), recorder, nil).Execute(context.Background(), validRequest(server.URL))
	if !errors.Is(err, ErrFormatDrift) || len(outcome.Raw) == 0 || len(outcome.MappingDiagnostics) == 0 {
		t.Fatalf("expected diagnosable format drift, got outcome=%+v err=%v", outcome, err)
	}
	if recorder.attempts[0].ErrorCategory != "format_drift" {
		t.Fatalf("format drift was not audited: %+v", recorder.attempts)
	}
}

func TestHTTPJSONAdapterOpensCircuitAfterRepeatedProviderFailures(t *testing.T) {
	var calls int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	breaker := NewCircuitBreaker(2, time.Hour)
	adapter := NewHTTPJSONAdapter(server.Client(), nil, breaker)
	adapter.maxAttempts = 1
	_, _ = adapter.Execute(context.Background(), validRequest(server.URL))
	_, _ = adapter.Execute(context.Background(), validRequest(server.URL))
	_, err := adapter.Execute(context.Background(), validRequest(server.URL))
	if !errors.Is(err, ErrCircuitOpen) || calls != 2 {
		t.Fatalf("expected open circuit after two failures, calls=%d err=%v", calls, err)
	}
}

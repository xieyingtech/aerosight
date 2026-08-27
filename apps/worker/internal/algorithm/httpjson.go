package algorithm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const InputSchemaVersionV1 = "aerosight.algorithm.input/v1"

var (
	ErrCircuitOpen = errors.New("algorithm provider circuit is open")
	ErrFormatDrift = errors.New("algorithm provider response no longer matches its mapping")
)

type AssetReference struct {
	AssetID         int       `json:"assetId"`
	Version         int       `json:"version"`
	ChecksumSHA256  string    `json:"checksumSha256"`
	MIMEType        string    `json:"mimeType"`
	AccessURL       string    `json:"accessUrl"`
	AccessExpiresAt time.Time `json:"accessExpiresAt"`
}

type DefinitionReference struct {
	DefinitionVersionID int64  `json:"definitionVersionId"`
	ProviderType        string `json:"providerType"`
	ModelOrProcess      string `json:"modelOrProcess"`
	ExecutionMode       string `json:"executionMode"`
	MappingVersion      string `json:"mappingVersion"`
}

type Input struct {
	SchemaVersion string              `json:"schemaVersion"`
	RunID         string              `json:"runId"`
	ProjectID     int                 `json:"projectId"`
	Definition    DefinitionReference `json:"definition"`
	InputAsset    AssetReference      `json:"inputAsset"`
	Context       map[string]any      `json:"context"`
	Parameters    map[string]any      `json:"parameters"`
	Callback      map[string]string   `json:"callback,omitempty"`
}

type Mapping struct {
	DetectionsPath string `json:"detectionsPath"`
	KeyPath        string `json:"keyPath"`
	LabelPath      string `json:"labelPath"`
	ConfidencePath string `json:"confidencePath"`
	GeometryPath   string `json:"geometryPath"`
}

type Request struct {
	Endpoint string
	Headers  map[string]string
	Input    Input
	Mapping  Mapping
	Timeout  time.Duration
}

type Detection struct {
	DetectionKey  string         `json:"detectionKey"`
	Label         string         `json:"label"`
	Confidence    float64        `json:"confidence"`
	PixelGeometry any            `json:"pixelGeometry"`
	Attributes    map[string]any `json:"attributes"`
}

type Outcome struct {
	Kind               string      `json:"kind"`
	ExternalJobID      string      `json:"externalJobId,omitempty"`
	NextPollAt         time.Time   `json:"nextPollAt,omitempty"`
	Detections         []Detection `json:"detections,omitempty"`
	Raw                []byte      `json:"-"`
	MappingDiagnostics []string    `json:"mappingDiagnostics,omitempty"`
}

type Attempt struct {
	RunID          string
	Number         int
	Status         string
	RequestHash    string
	ResponseStatus *int
	ExternalJobID  string
	Duration       time.Duration
	ErrorCategory  string
	StartedAt      time.Time
	FinishedAt     time.Time
}

type AttemptRecorder interface {
	RecordAttempt(context.Context, Attempt) error
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Sleeper interface {
	Sleep(context.Context, time.Duration) error
}

type realSleeper struct{}

func (realSleeper) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type circuitState struct {
	failures int
	openedAt time.Time
}

type CircuitBreaker struct {
	mu        sync.Mutex
	states    map[string]circuitState
	threshold int
	cooldown  time.Duration
	now       func() time.Time
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	if threshold < 1 {
		threshold = 3
	}
	return &CircuitBreaker{states: map[string]circuitState{}, threshold: threshold, cooldown: cooldown, now: time.Now}
}

func (breaker *CircuitBreaker) allow(key string) bool {
	breaker.mu.Lock()
	defer breaker.mu.Unlock()
	state := breaker.states[key]
	if state.openedAt.IsZero() {
		return true
	}
	if breaker.now().Sub(state.openedAt) >= breaker.cooldown {
		delete(breaker.states, key)
		return true
	}
	return false
}

func (breaker *CircuitBreaker) success(key string) {
	breaker.mu.Lock()
	defer breaker.mu.Unlock()
	delete(breaker.states, key)
}

func (breaker *CircuitBreaker) failure(key string) {
	breaker.mu.Lock()
	defer breaker.mu.Unlock()
	state := breaker.states[key]
	state.failures++
	if state.failures >= breaker.threshold && state.openedAt.IsZero() {
		state.openedAt = breaker.now()
	}
	breaker.states[key] = state
}

type HTTPJSONAdapter struct {
	client      HTTPDoer
	recorder    AttemptRecorder
	sleeper     Sleeper
	breaker     *CircuitBreaker
	maxAttempts int
	baseBackoff time.Duration
	now         func() time.Time
}

func NewHTTPJSONAdapter(client HTTPDoer, recorder AttemptRecorder, breaker *CircuitBreaker) *HTTPJSONAdapter {
	if client == nil {
		client = http.DefaultClient
	}
	if breaker == nil {
		breaker = NewCircuitBreaker(3, 30*time.Second)
	}
	return &HTTPJSONAdapter{
		client: client, recorder: recorder, sleeper: realSleeper{}, breaker: breaker,
		maxAttempts: 3, baseBackoff: 250 * time.Millisecond, now: time.Now,
	}
}

func (adapter *HTTPJSONAdapter) Execute(ctx context.Context, request Request) (Outcome, error) {
	if err := validateRequest(request); err != nil {
		return Outcome{}, err
	}
	if !adapter.breaker.allow(request.Endpoint) {
		return Outcome{}, ErrCircuitOpen
	}
	body, err := json.Marshal(request.Input)
	if err != nil {
		return Outcome{}, err
	}
	hash := sha256.Sum256(body)
	requestHash := hex.EncodeToString(hash[:])
	var lastErr error
	for number := 1; number <= adapter.maxAttempts; number++ {
		outcome, retry, attempt, executeErr := adapter.executeAttempt(ctx, request, body, requestHash, number)
		if recordErr := adapter.record(ctx, attempt); recordErr != nil {
			return Outcome{}, fmt.Errorf("record algorithm attempt: %w", recordErr)
		}
		if executeErr == nil {
			adapter.breaker.success(request.Endpoint)
			return outcome, nil
		}
		lastErr = executeErr
		if !retry || number == adapter.maxAttempts {
			adapter.breaker.failure(request.Endpoint)
			return outcome, executeErr
		}
		if err := adapter.sleeper.Sleep(ctx, adapter.backoff(number, attempt.ResponseStatus)); err != nil {
			return Outcome{}, err
		}
	}
	return Outcome{}, lastErr
}

func (adapter *HTTPJSONAdapter) executeAttempt(
	ctx context.Context, request Request, body []byte, requestHash string, number int,
) (Outcome, bool, Attempt, error) {
	started := adapter.now()
	attempt := Attempt{RunID: request.Input.RunID, Number: number, Status: "running", RequestHash: requestHash, StartedAt: started}
	requestCtx := ctx
	cancel := func() {}
	if request.Timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, request.Timeout)
	}
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodPost, request.Endpoint, bytes.NewReader(body))
	if err != nil {
		return Outcome{}, false, adapter.finish(attempt, "failed", "request", nil), err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	for name, value := range request.Headers {
		httpRequest.Header.Set(name, value)
	}
	response, err := adapter.client.Do(httpRequest)
	if err != nil {
		category := "transport"
		status := "failed"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			category, status = "timeout", "timed_out"
		}
		return Outcome{}, category == "transport", adapter.finish(attempt, status, category, nil), err
	}
	defer response.Body.Close()
	attempt.ResponseStatus = &response.StatusCode
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if readErr != nil {
		return Outcome{Raw: raw}, true, adapter.finish(attempt, "failed", "response_read", nil), readErr
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return Outcome{Raw: raw}, true, adapter.finish(attempt, "rate_limited", "rate_limit", nil), fmt.Errorf("provider rate limited request")
	}
	if response.StatusCode >= 500 {
		return Outcome{Raw: raw}, true, adapter.finish(attempt, "failed", "provider_5xx", nil), fmt.Errorf("provider returned %s", response.Status)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Outcome{Raw: raw}, false, adapter.finish(attempt, "failed", "provider_4xx", nil), fmt.Errorf("provider returned %s", response.Status)
	}
	if response.StatusCode == http.StatusAccepted {
		outcome, parseErr := acceptedOutcome(raw, response.Header, adapter.now())
		if parseErr != nil {
			return Outcome{Raw: raw}, false, adapter.finish(attempt, "failed", "format_drift", nil), parseErr
		}
		if request.Input.Definition.ExecutionMode == "callback" {
			outcome.Kind = "waiting_callback"
		}
		return outcome, false, adapter.finish(attempt, "succeeded", "", &outcome), nil
	}
	outcome, parseErr := mapCompleted(raw, request.Mapping)
	if parseErr != nil {
		return outcome, false, adapter.finish(attempt, "failed", "format_drift", nil), parseErr
	}
	return outcome, false, adapter.finish(attempt, "succeeded", "", &outcome), nil
}

func (adapter *HTTPJSONAdapter) finish(attempt Attempt, status, category string, outcome *Outcome) Attempt {
	attempt.Status = status
	attempt.ErrorCategory = category
	attempt.FinishedAt = adapter.now()
	attempt.Duration = attempt.FinishedAt.Sub(attempt.StartedAt)
	if outcome != nil {
		attempt.ExternalJobID = outcome.ExternalJobID
	}
	return attempt
}

func (adapter *HTTPJSONAdapter) record(ctx context.Context, attempt Attempt) error {
	if adapter.recorder == nil {
		return nil
	}
	return adapter.recorder.RecordAttempt(ctx, attempt)
}

func (adapter *HTTPJSONAdapter) backoff(number int, status *int) time.Duration {
	if status != nil && *status == http.StatusTooManyRequests {
		return adapter.baseBackoff * time.Duration(1<<(number-1))
	}
	return adapter.baseBackoff * time.Duration(1<<(number-1))
}

func validateRequest(request Request) error {
	if request.Endpoint == "" || request.Input.RunID == "" || request.Input.ProjectID <= 0 {
		return errors.New("algorithm request is missing endpoint, run, or project")
	}
	if request.Input.SchemaVersion != InputSchemaVersionV1 || request.Input.Definition.ProviderType != "http-json" {
		return errors.New("http-json adapter received an unsupported input contract")
	}
	if !strings.HasPrefix(request.Input.InputAsset.AccessURL, "https://") {
		return errors.New("algorithm input requires a presigned HTTPS asset URL")
	}
	if request.Input.InputAsset.AccessExpiresAt.Before(time.Now()) {
		return errors.New("algorithm input asset URL has expired")
	}
	return nil
}

func acceptedOutcome(raw []byte, headers http.Header, now time.Time) (Outcome, error) {
	var payload struct {
		ExternalJobID string `json:"externalJobId"`
		NextPollAt    string `json:"nextPollAt"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.ExternalJobID == "" {
		return Outcome{}, fmt.Errorf("%w: asynchronous response requires externalJobId", ErrFormatDrift)
	}
	nextPollAt, err := time.Parse(time.RFC3339, payload.NextPollAt)
	if err != nil {
		retryAfter, parseErr := strconv.Atoi(headers.Get("Retry-After"))
		if parseErr != nil || retryAfter < 1 {
			retryAfter = 1
		}
		nextPollAt = now.Add(time.Duration(retryAfter) * time.Second)
	}
	return Outcome{Kind: "accepted", ExternalJobID: payload.ExternalJobID, NextPollAt: nextPollAt, Raw: raw}, nil
}

func mapCompleted(raw []byte, mapping Mapping) (Outcome, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Outcome{Raw: raw, MappingDiagnostics: []string{"response is not valid JSON"}}, fmt.Errorf("%w: invalid JSON", ErrFormatDrift)
	}
	itemsValue, ok := pathValue(payload, mapping.DetectionsPath)
	items, okArray := itemsValue.([]any)
	if !ok || !okArray {
		diagnostic := fmt.Sprintf("detections path %q is missing or not an array", mapping.DetectionsPath)
		return Outcome{Raw: raw, MappingDiagnostics: []string{diagnostic}}, fmt.Errorf("%w: %s", ErrFormatDrift, diagnostic)
	}
	detections := make([]Detection, 0, len(items))
	var diagnostics []string
	for index, item := range items {
		key, keyOK := stringAt(item, mapping.KeyPath)
		label, labelOK := stringAt(item, mapping.LabelPath)
		confidence, confidenceOK := numberAt(item, mapping.ConfidencePath)
		geometry, geometryOK := pathValue(item, mapping.GeometryPath)
		if !keyOK {
			key = strconv.Itoa(index)
		}
		if !labelOK || !confidenceOK || !geometryOK || confidence < 0 || confidence > 1 {
			diagnostics = append(diagnostics, fmt.Sprintf("detection[%d] does not match label/confidence/geometry mapping", index))
			continue
		}
		detections = append(detections, Detection{DetectionKey: key, Label: label, Confidence: confidence, PixelGeometry: geometry, Attributes: map[string]any{}})
	}
	if len(diagnostics) > 0 {
		return Outcome{Raw: raw, MappingDiagnostics: diagnostics}, fmt.Errorf("%w: %s", ErrFormatDrift, strings.Join(diagnostics, "; "))
	}
	return Outcome{Kind: "completed", Detections: detections, Raw: raw}, nil
}

func pathValue(value any, path string) (any, bool) {
	if path == "" || path == "$" {
		return value, true
	}
	current := value
	for _, part := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func stringAt(value any, path string) (string, bool) {
	found, ok := pathValue(value, path)
	result, typeOK := found.(string)
	return result, ok && typeOK && result != ""
}

func numberAt(value any, path string) (float64, bool) {
	found, ok := pathValue(value, path)
	result, typeOK := found.(float64)
	return result, ok && typeOK
}

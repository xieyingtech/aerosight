package flighthub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ChinaAPIOrigin       = "https://es-flight-api-cn.djigate.com"
	ProjectPageSize      = 20
	DeviceDirectoryLimit = 1000
)

type Config struct {
	Timeout           time.Duration
	MaxRetries        int
	MaxProjectPages   int
	MaxResponseBytes  int64
	HTTPClient        *http.Client
	RequestID         func() string
	Sleep             func(context.Context, time.Duration) error
	Now               func() time.Time
	Jitter            func(time.Duration) time.Duration
	MaxConcurrent     int
	RequestsPerSecond float64
	RequestBurst      int
	AllowedLinkHosts  []string
}

type Client struct {
	baseURL          *url.URL
	httpClient       *http.Client
	timeout          time.Duration
	maxRetries       int
	maxProjectPages  int
	maxResponseBytes int64
	requestID        func() string
	sleep            func(context.Context, time.Duration) error
	now              func() time.Time
	jitter           func(time.Duration) time.Duration
	gate             *requestGate
	allowedLinkHosts map[string]struct{}
}

type LinkPurpose string

const (
	LinkUpload   LinkPurpose = "upload"
	LinkDownload LinkPurpose = "download"
	LinkLive     LinkPurpose = "live"
	LinkModel    LinkPurpose = "model"
)

type requestGate struct {
	semaphore chan struct{}
	mu        sync.Mutex
	rate      float64
	capacity  float64
	tokens    float64
	last      time.Time
	now       func() time.Time
	sleep     func(context.Context, time.Duration) error
}

func newRequestGate(maxConcurrent int, rate float64, burst int, now func() time.Time, sleep func(context.Context, time.Duration) error) *requestGate {
	return &requestGate{
		semaphore: make(chan struct{}, maxConcurrent), rate: rate, capacity: float64(burst), tokens: float64(burst),
		last: now(), now: now, sleep: sleep,
	}
}

func (gate *requestGate) enter(ctx context.Context) (func(), error) {
	select {
	case gate.semaphore <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	gate.mu.Lock()
	now := gate.now()
	var wait time.Duration
	if now.Before(gate.last) {
		wait = gate.last.Sub(now) + time.Duration(float64(time.Second)/gate.rate)
		gate.last = now.Add(wait)
		gate.tokens = 0
	} else {
		elapsed := now.Sub(gate.last).Seconds()
		gate.tokens = min(gate.capacity, gate.tokens+elapsed*gate.rate)
		gate.last = now
		if gate.tokens >= 1 {
			gate.tokens--
		} else {
			wait = time.Duration((1 - gate.tokens) / gate.rate * float64(time.Second))
			gate.tokens = 0
			gate.last = now.Add(wait)
		}
	}
	gate.mu.Unlock()
	if wait > 0 {
		if err := gate.sleep(ctx, wait); err != nil {
			<-gate.semaphore
			return nil, err
		}
	}
	return func() { <-gate.semaphore }, nil
}

type Project struct {
	UUID             string `json:"uuid"`
	Name             string `json:"name"`
	OrganizationUUID string `json:"org_uuid"`
}

type DeviceModel struct {
	Key     string `json:"key"`
	Domain  string `json:"domain"`
	Type    string `json:"type"`
	Subtype string `json:"sub_type"`
	Name    string `json:"name"`
	Class   string `json:"class"`
}

type Device struct {
	SN           string          `json:"sn"`
	Callsign     string          `json:"callsign"`
	Model        DeviceModel     `json:"device_model"`
	Online       bool            `json:"device_online_status"`
	ModeCode     int             `json:"mode_code"`
	CameraList   json.RawMessage `json:"camera_list"`
	onlineStatus *bool
}

type Topology struct {
	Gateway *Device `json:"gateway"`
	Drone   *Device `json:"drone"`
}

type APIError struct {
	SafeCode   string
	Retryable  bool
	HTTPStatus int
	RetryAfter time.Duration
}

func (err *APIError) Error() string { return "DJI_FLIGHTHUB_" + strings.ToUpper(err.SafeCode) }

func (err *APIError) ConnectorSafeCode() string { return err.SafeCode }

type envelope struct {
	Code    int             `json:"code"`
	Message json.RawMessage `json:"message"`
	Data    json.RawMessage `json:"data"`
	Empty   bool            `json:"-"`
}

type requestSpec struct {
	Method       string
	Path         string
	Query        url.Values
	Body         any
	Profile      string
	DataOptional bool
}

var endpointBusinessProfiles = map[string]map[int]struct{}{
	"default":           {},
	"live-share-list":   {231011: {}},
	"live-share-detail": {231011: {}},
}

func defaultSleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func NewChinaClient(config Config) (*Client, error) {
	if config.Timeout == 0 {
		config.Timeout = 8 * time.Second
	}
	if config.MaxProjectPages == 0 {
		config.MaxProjectPages = 50
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = 4 << 20
	}
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 4
	}
	if config.RequestsPerSecond == 0 {
		config.RequestsPerSecond = 4
	}
	if config.RequestBurst == 0 {
		config.RequestBurst = 4
	}
	if len(config.AllowedLinkHosts) == 0 {
		config.AllowedLinkHosts = []string{"es-flight-api-cn.djigate.com"}
	}
	if config.Timeout < 500*time.Millisecond || config.Timeout > 30*time.Second ||
		config.MaxRetries < 0 || config.MaxRetries > 3 ||
		config.MaxProjectPages < 1 || config.MaxProjectPages > 100 ||
		config.MaxResponseBytes < 1024 || config.MaxResponseBytes > 16<<20 ||
		config.MaxConcurrent < 1 || config.MaxConcurrent > 32 ||
		config.RequestsPerSecond <= 0 || config.RequestsPerSecond > 100 ||
		config.RequestBurst < 1 || config.RequestBurst > 100 ||
		config.RequestID == nil {
		return nil, errors.New("DJI_FLIGHTHUB_CONFIGURATION_INVALID")
	}
	baseURL, err := url.Parse(ChinaAPIOrigin)
	if err != nil {
		return nil, errors.New("DJI_FLIGHTHUB_CONFIGURATION_INVALID")
	}
	httpClient := http.Client{}
	if config.HTTPClient != nil {
		httpClient = *config.HTTPClient
	}
	httpClient.Timeout = config.Timeout
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("DJI_FLIGHTHUB_REDIRECT_FORBIDDEN")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	jitter := config.Jitter
	if jitter == nil {
		jitter = func(window time.Duration) time.Duration {
			if window <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(window) + 1))
		}
	}
	sleep := firstNonNilSleep(config.Sleep)
	allowedLinkHosts := make(map[string]struct{}, len(config.AllowedLinkHosts))
	for _, host := range config.AllowedLinkHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" || strings.ContainsAny(host, "/:@?#*") {
			return nil, errors.New("DJI_FLIGHTHUB_CONFIGURATION_INVALID")
		}
		allowedLinkHosts[host] = struct{}{}
	}
	return &Client{
		baseURL:          baseURL,
		httpClient:       &httpClient,
		timeout:          config.Timeout,
		maxRetries:       config.MaxRetries,
		maxProjectPages:  config.MaxProjectPages,
		maxResponseBytes: config.MaxResponseBytes,
		requestID:        config.RequestID,
		sleep:            sleep,
		now:              now,
		jitter:           jitter,
		gate:             newRequestGate(config.MaxConcurrent, config.RequestsPerSecond, config.RequestBurst, now, sleep),
		allowedLinkHosts: allowedLinkHosts,
	}, nil
}

func firstNonNilSleep(sleep func(context.Context, time.Duration) error) func(context.Context, time.Duration) error {
	if sleep != nil {
		return sleep
	}
	return defaultSleep
}

func classifyStatus(status int, retryAfter string, now time.Time) *APIError {
	switch status {
	case http.StatusUnauthorized:
		return &APIError{SafeCode: "credential_invalid", HTTPStatus: status}
	case http.StatusForbidden:
		return &APIError{SafeCode: "scope_forbidden", HTTPStatus: status}
	case http.StatusNotFound:
		return &APIError{SafeCode: "scope_not_found", HTTPStatus: status}
	case http.StatusTooManyRequests:
		return &APIError{SafeCode: "rate_limited", Retryable: true, HTTPStatus: status, RetryAfter: parseRetryAfter(retryAfter, now)}
	default:
		if status >= 500 {
			return &APIError{SafeCode: "upstream_unavailable", Retryable: true, HTTPStatus: status}
		}
		if status >= 400 {
			return &APIError{SafeCode: "upstream_error", HTTPStatus: status}
		}
	}
	return nil
}

func classifyBusinessCode(code, status int, emptyCodes map[int]struct{}) (*APIError, bool) {
	if _, empty := emptyCodes[code]; empty {
		return nil, true
	}
	switch code {
	case 0:
		return nil, false
	case 200401:
		return &APIError{SafeCode: "credential_invalid", HTTPStatus: status}, false
	case 200403:
		return &APIError{SafeCode: "scope_forbidden", HTTPStatus: status}, false
	case 200404:
		return &APIError{SafeCode: "scope_not_found", HTTPStatus: status}, false
	case 200610:
		return &APIError{SafeCode: "configuration_required", HTTPStatus: status}, false
	case 210429:
		return &APIError{SafeCode: "rate_limited", Retryable: true, HTTPStatus: status}, false
	case 200500, 210318, 210500, 210504:
		return &APIError{SafeCode: "upstream_unavailable", Retryable: true, HTTPStatus: status}, false
	default:
		return &APIError{SafeCode: "upstream_error", HTTPStatus: status}, false
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return min(time.Duration(seconds)*time.Second, 10*time.Second)
	}
	if parsed, err := http.ParseTime(value); err == nil {
		return min(max(time.Duration(0), parsed.Sub(now)), 10*time.Second)
	}
	return 0
}

func (client *Client) retryDelay(attempt int) time.Duration {
	base := min(250*time.Millisecond*time.Duration(1<<attempt), 2*time.Second)
	return min(base+client.jitter(base/4), 2*time.Second)
}

func (client *Client) ValidateTemporaryLink(purpose LinkPurpose, raw string, expiresAt time.Time) (*url.URL, error) {
	switch purpose {
	case LinkUpload, LinkDownload, LinkLive, LinkModel:
	default:
		return nil, &APIError{SafeCode: "temporary_link_invalid"}
	}
	if expiresAt.IsZero() || !expiresAt.After(client.now()) || expiresAt.After(client.now().Add(24*time.Hour)) {
		return nil, &APIError{SafeCode: "temporary_link_expired"}
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, &APIError{SafeCode: "temporary_link_invalid"}
	}
	if _, allowed := client.allowedLinkHosts[strings.ToLower(parsed.Hostname())]; !allowed {
		return nil, &APIError{SafeCode: "temporary_link_host_forbidden"}
	}
	return parsed, nil
}

func validateRequestSpec(spec requestSpec) error {
	switch spec.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return &APIError{SafeCode: "request_invalid"}
	}
	if !strings.HasPrefix(spec.Path, "/openapi/v2.0/") || strings.Contains(spec.Path, "://") ||
		strings.ContainsAny(spec.Path, "?#\\") || strings.Contains(spec.Path, "//") || strings.Contains(spec.Path, "/../") ||
		strings.ContainsAny(spec.Path, "{}") {
		return &APIError{SafeCode: "request_invalid"}
	}
	if spec.Method == http.MethodGet && spec.Body != nil {
		return &APIError{SafeCode: "request_invalid"}
	}
	if spec.Profile != "" {
		if _, exists := endpointBusinessProfiles[spec.Profile]; !exists {
			return &APIError{SafeCode: "request_invalid"}
		}
	}
	return nil
}

func resolvePathTemplate(template string, parameters map[string]string) (string, error) {
	path := template
	for name, rawValue := range parameters {
		value := strings.TrimSpace(rawValue)
		if value == "" || value != rawValue || strings.ContainsAny(value, "/\\?#") {
			return "", &APIError{SafeCode: "request_invalid"}
		}
		placeholder := "{" + name + "}"
		if !strings.Contains(path, placeholder) {
			return "", &APIError{SafeCode: "request_invalid"}
		}
		path = strings.ReplaceAll(path, placeholder, url.PathEscape(value))
	}
	if strings.ContainsAny(path, "{}") {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return path, nil
}

func (client *Client) request(ctx context.Context, token, projectUUID string, spec requestSpec) (envelope, error) {
	if strings.TrimSpace(token) == "" {
		return envelope{}, &APIError{SafeCode: "credential_invalid"}
	}
	if err := validateRequestSpec(spec); err != nil {
		return envelope{}, err
	}
	emptyCodes := endpointBusinessProfiles["default"]
	if spec.Profile != "" {
		emptyCodes = endpointBusinessProfiles[spec.Profile]
	}
	var encodedBody []byte
	if spec.Body != nil {
		var err error
		encodedBody, err = json.Marshal(spec.Body)
		if err != nil {
			return envelope{}, &APIError{SafeCode: "request_invalid"}
		}
	}
	target := *client.baseURL
	target.Path = spec.Path
	target.RawQuery = spec.Query.Encode()
	for attempt := 0; attempt <= client.maxRetries; attempt++ {
		release, gateErr := client.gate.enter(ctx)
		if gateErr != nil {
			return envelope{}, gateErr
		}
		attemptContext, cancel := context.WithTimeout(ctx, client.timeout)
		var requestBody io.Reader
		if encodedBody != nil {
			requestBody = bytes.NewReader(encodedBody)
		}
		request, err := http.NewRequestWithContext(attemptContext, spec.Method, target.String(), requestBody)
		if err != nil {
			cancel()
			release()
			return envelope{}, &APIError{SafeCode: "upstream_error"}
		}
		request.Header.Set("Accept", "application/json")
		if encodedBody != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		request.Header.Set("X-User-Token", token)
		request.Header.Set("X-Request-Id", client.requestID())
		request.Header.Set("X-Language", "zh")
		if projectUUID != "" {
			request.Header.Set("X-Project-Uuid", projectUUID)
		}

		response, requestErr := client.httpClient.Do(request)
		if requestErr != nil {
			cancel()
			release()
			safeErr := &APIError{SafeCode: "upstream_unavailable", Retryable: true}
			if errors.Is(attemptContext.Err(), context.DeadlineExceeded) {
				safeErr.SafeCode = "request_timeout"
			}
			if attempt == client.maxRetries {
				return envelope{}, safeErr
			}
			if err := client.sleep(ctx, client.retryDelay(attempt)); err != nil {
				return envelope{}, err
			}
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, client.maxResponseBytes+1))
		_ = response.Body.Close()
		cancel()
		release()
		if readErr != nil {
			if attempt == client.maxRetries {
				return envelope{}, &APIError{SafeCode: "upstream_unavailable", Retryable: true, HTTPStatus: response.StatusCode}
			}
			continue
		}
		if int64(len(body)) > client.maxResponseBytes {
			return envelope{}, &APIError{SafeCode: "response_too_large", HTTPStatus: response.StatusCode}
		}
		var decoded envelope
		decodeErr := json.Unmarshal(body, &decoded)
		if statusErr := classifyStatus(response.StatusCode, response.Header.Get("Retry-After"), client.now()); statusErr != nil {
			if statusErr.SafeCode == "upstream_error" && decodeErr == nil && decoded.Data != nil {
				if businessErr, _ := classifyBusinessCode(decoded.Code, response.StatusCode, emptyCodes); businessErr != nil {
					statusErr = businessErr
				}
			}
			if !statusErr.Retryable || attempt == client.maxRetries {
				return envelope{}, statusErr
			}
			delay := statusErr.RetryAfter
			if delay == 0 {
				delay = client.retryDelay(attempt)
			}
			if err := client.sleep(ctx, delay); err != nil {
				return envelope{}, err
			}
			continue
		}
		if decodeErr != nil || (decoded.Data == nil && !spec.DataOptional) {
			return envelope{}, &APIError{SafeCode: "schema_incompatible", HTTPStatus: response.StatusCode}
		}
		businessErr, empty := classifyBusinessCode(decoded.Code, response.StatusCode, emptyCodes)
		if businessErr != nil {
			if !businessErr.Retryable || attempt == client.maxRetries {
				return envelope{}, businessErr
			}
			if err := client.sleep(ctx, client.retryDelay(attempt)); err != nil {
				return envelope{}, err
			}
			continue
		}
		decoded.Empty = empty
		return decoded, nil
	}
	return envelope{}, &APIError{SafeCode: "upstream_unavailable", Retryable: true}
}

func (client *Client) ListProjects(ctx context.Context, token string) ([]Project, error) {
	projects := make([]Project, 0)
	seen := map[string]struct{}{}
	for page := 1; page <= client.maxProjectPages; page++ {
		payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/project", Query: url.Values{
			"usage":       {"complete"},
			"sort_column": {"create_time"},
			"sort_type":   {"desc"},
			"page":        {strconv.Itoa(page)},
			"page_size":   {strconv.Itoa(ProjectPageSize)},
		}})
		if err != nil {
			return nil, err
		}
		var data struct {
			List []Project `json:"list"`
		}
		if err := json.Unmarshal(payload.Data, &data); err != nil || data.List == nil {
			return nil, &APIError{SafeCode: "schema_incompatible", HTTPStatus: http.StatusOK}
		}
		for _, project := range data.List {
			project.UUID = strings.ToLower(strings.TrimSpace(project.UUID))
			project.OrganizationUUID = strings.ToLower(strings.TrimSpace(project.OrganizationUUID))
			project.Name = strings.TrimSpace(project.Name)
			if project.UUID == "" || project.OrganizationUUID == "" || project.Name == "" {
				return nil, &APIError{SafeCode: "schema_incompatible", HTTPStatus: http.StatusOK}
			}
			if _, duplicate := seen[project.UUID]; duplicate {
				return nil, &APIError{SafeCode: "schema_incompatible", HTTPStatus: http.StatusOK}
			}
			seen[project.UUID] = struct{}{}
			projects = append(projects, project)
		}
		if len(data.List) < ProjectPageSize {
			return projects, nil
		}
	}
	return nil, &APIError{SafeCode: "project_page_limit", HTTPStatus: http.StatusOK}
}

type rawDevice struct {
	SN         string          `json:"sn"`
	Callsign   string          `json:"callsign"`
	Model      DeviceModel     `json:"device_model"`
	Online     *bool           `json:"device_online_status"`
	ModeCode   int             `json:"mode_code"`
	CameraList json.RawMessage `json:"camera_list"`
}

type rawTopology struct {
	Gateway *rawDevice `json:"gateway"`
	Drone   *rawDevice `json:"drone"`
}

func validatedDevice(raw *rawDevice) (*Device, error) {
	if raw == nil {
		return nil, nil
	}
	if strings.TrimSpace(raw.SN) == "" || strings.TrimSpace(raw.Model.Key) == "" || strings.TrimSpace(raw.Model.Class) == "" || raw.Online == nil {
		return nil, &APIError{SafeCode: "schema_incompatible", HTTPStatus: http.StatusOK}
	}
	return &Device{
		SN: strings.TrimSpace(raw.SN), Callsign: strings.TrimSpace(raw.Callsign), Model: raw.Model,
		Online: *raw.Online, ModeCode: raw.ModeCode, CameraList: raw.CameraList, onlineStatus: raw.Online,
	}, nil
}

func (client *Client) ListDevices(ctx context.Context, token, projectUUID string) ([]Topology, error) {
	if strings.TrimSpace(projectUUID) == "" {
		return nil, &APIError{SafeCode: "scope_forbidden"}
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/project/device"})
	if err != nil {
		return nil, err
	}
	var data struct {
		List []rawTopology `json:"list"`
	}
	if err := json.Unmarshal(payload.Data, &data); err != nil || data.List == nil {
		return nil, &APIError{SafeCode: "schema_incompatible", HTTPStatus: http.StatusOK}
	}
	if len(data.List) >= DeviceDirectoryLimit {
		return nil, &APIError{SafeCode: "directory_limit_reached", HTTPStatus: http.StatusOK}
	}
	topologies := make([]Topology, 0, len(data.List))
	for _, item := range data.List {
		gateway, gatewayErr := validatedDevice(item.Gateway)
		if gatewayErr != nil {
			return nil, gatewayErr
		}
		drone, droneErr := validatedDevice(item.Drone)
		if droneErr != nil {
			return nil, droneErr
		}
		if gateway == nil && drone == nil {
			return nil, &APIError{SafeCode: "schema_incompatible", HTTPStatus: http.StatusOK}
		}
		topologies = append(topologies, Topology{Gateway: gateway, Drone: drone})
	}
	return topologies, nil
}

func IsSafeCode(err error, code string) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.SafeCode == code
}

func SafeCode(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.SafeCode
	}
	return fmt.Sprintf("%T", err)
}

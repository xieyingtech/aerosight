package flighthub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ChinaAPIOrigin       = "https://es-flight-api-cn.djigate.com"
	ProjectPageSize      = 20
	DeviceDirectoryLimit = 1000
)

type Config struct {
	Timeout          time.Duration
	MaxRetries       int
	MaxProjectPages  int
	MaxResponseBytes int64
	HTTPClient       *http.Client
	RequestID        func() string
	Sleep            func(context.Context, time.Duration) error
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
	if config.Timeout < 500*time.Millisecond || config.Timeout > 30*time.Second ||
		config.MaxRetries < 0 || config.MaxRetries > 3 ||
		config.MaxProjectPages < 1 || config.MaxProjectPages > 100 ||
		config.MaxResponseBytes < 1024 || config.MaxResponseBytes > 16<<20 ||
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
	return &Client{
		baseURL:          baseURL,
		httpClient:       &httpClient,
		timeout:          config.Timeout,
		maxRetries:       config.MaxRetries,
		maxProjectPages:  config.MaxProjectPages,
		maxResponseBytes: config.MaxResponseBytes,
		requestID:        config.RequestID,
		sleep:            firstNonNilSleep(config.Sleep),
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

func classifyBusinessCode(code, status int) *APIError {
	switch code {
	case 0:
		return nil
	case 200401:
		return &APIError{SafeCode: "credential_invalid", HTTPStatus: status}
	case 200403:
		return &APIError{SafeCode: "scope_forbidden", HTTPStatus: status}
	case 200404:
		return &APIError{SafeCode: "scope_not_found", HTTPStatus: status}
	case 210429:
		return &APIError{SafeCode: "rate_limited", Retryable: true, HTTPStatus: status}
	case 200500, 210318, 210500, 210504:
		return &APIError{SafeCode: "upstream_unavailable", Retryable: true, HTTPStatus: status}
	default:
		return &APIError{SafeCode: "upstream_error", HTTPStatus: status}
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

func (client *Client) request(ctx context.Context, token, projectUUID, path string, query url.Values) (envelope, error) {
	if strings.TrimSpace(token) == "" {
		return envelope{}, &APIError{SafeCode: "credential_invalid"}
	}
	target := *client.baseURL
	target.Path = path
	target.RawQuery = query.Encode()
	for attempt := 0; attempt <= client.maxRetries; attempt++ {
		attemptContext, cancel := context.WithTimeout(ctx, client.timeout)
		request, err := http.NewRequestWithContext(attemptContext, http.MethodGet, target.String(), nil)
		if err != nil {
			cancel()
			return envelope{}, &APIError{SafeCode: "upstream_error"}
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("X-User-Token", token)
		request.Header.Set("X-Request-Id", client.requestID())
		request.Header.Set("X-Language", "zh")
		if projectUUID != "" {
			request.Header.Set("X-Project-Uuid", projectUUID)
		}

		response, requestErr := client.httpClient.Do(request)
		if requestErr != nil {
			cancel()
			safeErr := &APIError{SafeCode: "upstream_unavailable", Retryable: true}
			if errors.Is(attemptContext.Err(), context.DeadlineExceeded) {
				safeErr.SafeCode = "request_timeout"
			}
			if attempt == client.maxRetries {
				return envelope{}, safeErr
			}
			if err := client.sleep(ctx, min(250*time.Millisecond*time.Duration(1<<attempt), 2*time.Second)); err != nil {
				return envelope{}, err
			}
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, client.maxResponseBytes+1))
		_ = response.Body.Close()
		cancel()
		if readErr != nil {
			if attempt == client.maxRetries {
				return envelope{}, &APIError{SafeCode: "upstream_unavailable", Retryable: true, HTTPStatus: response.StatusCode}
			}
			continue
		}
		if int64(len(body)) > client.maxResponseBytes {
			return envelope{}, &APIError{SafeCode: "response_too_large", HTTPStatus: response.StatusCode}
		}
		if statusErr := classifyStatus(response.StatusCode, response.Header.Get("Retry-After"), time.Now()); statusErr != nil {
			if !statusErr.Retryable || attempt == client.maxRetries {
				return envelope{}, statusErr
			}
			delay := statusErr.RetryAfter
			if delay == 0 {
				delay = min(250*time.Millisecond*time.Duration(1<<attempt), 2*time.Second)
			}
			if err := client.sleep(ctx, delay); err != nil {
				return envelope{}, err
			}
			continue
		}
		var decoded envelope
		if err := json.Unmarshal(body, &decoded); err != nil || decoded.Data == nil {
			return envelope{}, &APIError{SafeCode: "schema_incompatible", HTTPStatus: response.StatusCode}
		}
		if businessErr := classifyBusinessCode(decoded.Code, response.StatusCode); businessErr != nil {
			if !businessErr.Retryable || attempt == client.maxRetries {
				return envelope{}, businessErr
			}
			if err := client.sleep(ctx, min(250*time.Millisecond*time.Duration(1<<attempt), 2*time.Second)); err != nil {
				return envelope{}, err
			}
			continue
		}
		return decoded, nil
	}
	return envelope{}, &APIError{SafeCode: "upstream_unavailable", Retryable: true}
}

func (client *Client) ListProjects(ctx context.Context, token string) ([]Project, error) {
	projects := make([]Project, 0)
	seen := map[string]struct{}{}
	for page := 1; page <= client.maxProjectPages; page++ {
		payload, err := client.request(ctx, token, "", "/openapi/v2.0/project", url.Values{
			"usage":       {"complete"},
			"sort_column": {"create_time"},
			"sort_type":   {"desc"},
			"page":        {strconv.Itoa(page)},
			"page_size":   {strconv.Itoa(ProjectPageSize)},
		})
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
	payload, err := client.request(ctx, token, projectUUID, "/openapi/v2.0/project/device", nil)
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

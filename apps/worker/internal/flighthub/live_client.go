package flighthub

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxLiveCredentialBytes = 16 << 10
	maxLiveStringBytes     = 4096
	maxLiveCredentialTTL   = 24 * time.Hour
)

type LiveQuality string

const (
	LiveQualityAdaptive            LiveQuality = "adaptive"
	LiveQualitySmooth              LiveQuality = "smooth"
	LiveQualityUltraHighDefinition LiveQuality = "ultra_high_definition"
)

type LiveStreamStartRequest struct {
	SN          string      `json:"sn"`
	CameraIndex string      `json:"camera_index"`
	VideoExpire int         `json:"video_expire"`
	QualityType LiveQuality `json:"quality_type"`
}

// LiveStreamAuthorization contains an opaque, short-lived supplier credential.
// URL must never be persisted or emitted to ordinary logs, traces, metrics, or audits.
type LiveStreamAuthorization struct {
	ExpireTimestamp int64     `json:"expire_ts"`
	URL             string    `json:"url"`
	URLType         string    `json:"url_type"`
	ExpiresAt       time.Time `json:"-"`
}

type StreamQualityRequest struct {
	SN          string      `json:"sn"`
	CameraIndex string      `json:"camera_index"`
	QualityType LiveQuality `json:"quality_type"`
}

// RecordingTask is a named representation of the vendor-defined recording task.
// The released contract currently specifies the collection envelope but leaves
// task properties open, so callers must not derive capabilities from these fields.
type RecordingTask map[string]json.RawMessage

// LiveShare is a named representation of the vendor-defined live-share object.
// The released contract currently leaves share properties open.
type LiveShare map[string]json.RawMessage

type LiveShareListOptions struct {
	PageOptions
	Status int
}

type StreamConverterListOptions struct {
	PageOptions
	DeviceSN    string
	CameraIndex string
	Schema      string
	Enabled     *bool
}

// StreamConverterBypassOption may contain destination credentials and must be
// treated as sensitive by every projection and observability boundary.
type StreamConverterBypassOption struct {
	URL            string `json:"url"`
	ServerID       string `json:"server_id"`
	ServerIP       string `json:"server_ip"`
	ServerPort     string `json:"server_port"`
	DevicePassword string `json:"device_password"`
	LocalPort      string `json:"local_port"`
	DeviceID       string `json:"device_id"`
	LocalChannel   string `json:"local_channel"`
	RTSPURL        string `json:"rtsp_url"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	EnableTS       *int64 `json:"enable_ts"`
}

type StreamConverter struct {
	Name           string                       `json:"converter_name"`
	UpdatedAt      string                       `json:"update_ts"`
	ID             string                       `json:"converter_id"`
	State          string                       `json:"state"`
	Code           int                          `json:"code"`
	BypassOption   *StreamConverterBypassOption `json:"bypass_option"`
	SN             string                       `json:"sn"`
	CameraIndex    string                       `json:"camera"`
	Video          string                       `json:"video"`
	VideoType      string                       `json:"video_type"`
	Schema         string                       `json:"schema"`
	AutoPushStream bool                         `json:"auto_push_stream"`
	DeviceOnline   bool                         `json:"device_online_status"`
	DeviceCallsign string                       `json:"device_callsign"`
}

// StreamConverterSchemaOption is the union of the released RTMP/GB28181
// contract and the RTSP variant used by the current official success example.
type StreamConverterSchemaOption struct {
	URL            string `json:"url,omitempty"`
	ServerIP       string `json:"server_ip,omitempty"`
	ServerPort     string `json:"server_port,omitempty"`
	DevicePassword string `json:"device_password,omitempty"`
	LocalPort      string `json:"local_port,omitempty"`
	DeviceID       string `json:"device_id,omitempty"`
	LocalChannel   string `json:"local_channel,omitempty"`
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	EnableTS       *bool  `json:"enable_ts,omitempty"`
}

type StreamConverterCreateRequest struct {
	Name         string                      `json:"converter_name"`
	SN           string                      `json:"sn"`
	CameraIndex  string                      `json:"camera_index"`
	Schema       string                      `json:"schema"`
	SchemaOption StreamConverterSchemaOption `json:"schema_option"`
}

type StreamConverterCreateResult struct {
	ID string `json:"converter_id"`
}

type StreamConverterStateRequest struct {
	AutoPushStream bool `json:"auto_push_stream"`
}

func validLiveQuality(quality LiveQuality) bool {
	switch quality {
	case LiveQualityAdaptive, LiveQualitySmooth, LiveQualityUltraHighDefinition:
		return true
	default:
		return false
	}
}

func liveRequestString(value string, maximum int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maximum || strings.ContainsAny(value, "\x00\r\n") {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return value, nil
}

func validLiveOpaque(value string, maximum int, allowEmpty bool) bool {
	if value != strings.TrimSpace(value) || len(value) > maximum || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	return allowEmpty || value != ""
}

func validOpaqueObject[T ~map[string]json.RawMessage](item *T) bool {
	if item == nil || *item == nil || len(*item) > 128 {
		return false
	}
	for key := range *item {
		if !validLiveOpaque(key, 128, false) {
			return false
		}
	}
	return true
}

func validateStreamConverterOption(option *StreamConverterBypassOption) bool {
	if option == nil {
		return false
	}
	values := []string{
		option.URL, option.ServerID, option.ServerIP, option.ServerPort,
		option.DevicePassword, option.LocalPort, option.DeviceID, option.LocalChannel,
		option.RTSPURL, option.Username, option.Password,
	}
	for _, value := range values {
		if !validLiveOpaque(value, maxLiveStringBytes, true) {
			return false
		}
	}
	return option.EnableTS == nil || *option.EnableTS >= 0
}

func validateStreamConverter(item *StreamConverter) bool {
	if item == nil || !validLiveOpaque(item.ID, 256, false) ||
		!validLiveOpaque(item.Name, 256, false) || !validLiveOpaque(item.SN, 256, false) ||
		!validLiveOpaque(item.CameraIndex, 256, false) || !validLiveOpaque(item.Video, 256, false) ||
		!validLiveOpaque(item.VideoType, 256, true) || !validLiveOpaque(item.DeviceCallsign, 256, true) ||
		!validLiveOpaque(item.UpdatedAt, 256, true) ||
		!validEnum(item.Schema, "rtmp", "gb28181", "rtsp") || item.Schema == "" ||
		!validEnum(item.State, "initialized", "running", "stopped", "error", "failed") || item.State == "" {
		return false
	}
	return validateStreamConverterOption(item.BypassOption)
}

func (client *Client) StartLiveStream(ctx context.Context, token, projectUUID string, input LiveStreamStartRequest) (LiveStreamAuthorization, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return LiveStreamAuthorization{}, err
	}
	if input.SN, err = liveRequestString(input.SN, 256); err != nil {
		return LiveStreamAuthorization{}, err
	}
	if input.CameraIndex, err = liveRequestString(input.CameraIndex, 256); err != nil || !validLiveQuality(input.QualityType) || input.VideoExpire < 1 || time.Duration(input.VideoExpire)*time.Second > maxLiveCredentialTTL {
		return LiveStreamAuthorization{}, &APIError{SafeCode: "request_invalid"}
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodPost, Path: "/openapi/v2.0/live-stream/start", Body: input, DisableRetry: true,
	})
	if err != nil {
		return LiveStreamAuthorization{}, err
	}
	var result LiveStreamAuthorization
	if err := json.Unmarshal(payload.Data, &result); err != nil || result.ExpireTimestamp <= 0 ||
		!validLiveOpaque(result.URL, maxLiveCredentialBytes, false) || !validLiveOpaque(result.URLType, 64, false) {
		return LiveStreamAuthorization{}, schemaError()
	}
	result.ExpiresAt = time.Unix(result.ExpireTimestamp, 0).UTC()
	if !result.ExpiresAt.After(client.now()) || result.ExpiresAt.After(client.now().Add(maxLiveCredentialTTL)) {
		return LiveStreamAuthorization{}, &APIError{SafeCode: "temporary_link_expired", HTTPStatus: http.StatusOK}
	}
	return result, nil
}

func (client *Client) SetStreamQuality(ctx context.Context, token, projectUUID string, input StreamQualityRequest) error {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return err
	}
	if input.SN, err = liveRequestString(input.SN, 256); err != nil {
		return err
	}
	if input.CameraIndex, err = liveRequestString(input.CameraIndex, 256); err != nil || !validLiveQuality(input.QualityType) {
		return &APIError{SafeCode: "request_invalid"}
	}
	_, err = client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodPut, Path: "/openapi/v2.0/device/stream/quality", Body: input, DataOptional: true, DisableRetry: true,
	})
	return err
}

func recordingPath(template string, parameters map[string]string) (string, error) {
	for key, value := range parameters {
		var err error
		parameters[key], err = liveRequestString(value, 256)
		if err != nil {
			return "", err
		}
	}
	return resolvePathTemplate(template, parameters)
}

func (client *Client) ListOrganizationRecordingTasks(ctx context.Context, token, organizationUUID, serial string) ([]RecordingTask, error) {
	path, err := recordingPath("/openapi/v2.0/organizations/{uuid}/devices/{sn}/streams", map[string]string{"uuid": organizationUUID, "sn": serial})
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return nil, err
	}
	return decodeList(payload, false, validOpaqueObject[RecordingTask])
}

func (client *Client) ListProjectRecordingTasks(ctx context.Context, token, projectUUID, serial string) ([]RecordingTask, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return nil, err
	}
	path, err := recordingPath("/openapi/v2.0/devices/{sn}/streams", map[string]string{"sn": serial})
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return nil, err
	}
	return decodeList(payload, false, validOpaqueObject[RecordingTask])
}

func (client *Client) ListLiveShares(ctx context.Context, token, projectUUID string, options LiveShareListOptions) ([]LiveShare, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return nil, err
	}
	query, err := pageQuery(options.PageOptions)
	if err != nil || options.Status < 0 || options.Status > 2 {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	query.Set("status", strconv.Itoa(options.Status))
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodGet, Path: "/openapi/v2.0/live-shares", Query: query, Profile: "live-share-list",
	})
	if err != nil {
		return nil, err
	}
	if payload.Empty {
		return []LiveShare{}, nil
	}
	var shares []LiveShare
	if err := json.Unmarshal(payload.Data, &shares); err != nil || shares == nil {
		return nil, schemaError()
	}
	for index := range shares {
		if !validOpaqueObject(&shares[index]) {
			return nil, schemaError()
		}
	}
	return shares, nil
}

func (client *Client) GetLiveShare(ctx context.Context, token, projectUUID, serial string) (*LiveShare, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return nil, err
	}
	path, err := recordingPath("/openapi/v2.0/live-shares/{sn}", map[string]string{"sn": serial})
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodGet, Path: path, Profile: "live-share-detail",
	})
	if err != nil {
		return nil, err
	}
	if payload.Empty || string(payload.Data) == "null" {
		return nil, nil
	}
	var share LiveShare
	if err := json.Unmarshal(payload.Data, &share); err != nil || !validOpaqueObject(&share) {
		return nil, schemaError()
	}
	return &share, nil
}

func (client *Client) ListStreamConverters(ctx context.Context, token, projectUUID string, options StreamConverterListOptions) (PageResult[StreamConverter], error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return PageResult[StreamConverter]{}, err
	}
	query, err := pageQuery(options.PageOptions)
	if err != nil {
		return PageResult[StreamConverter]{}, err
	}
	for name, value := range map[string]string{"device_sn": options.DeviceSN, "camera_index": options.CameraIndex} {
		if err := addOptionalQuery(query, name, value); err != nil {
			return PageResult[StreamConverter]{}, err
		}
	}
	options.Schema = strings.TrimSpace(options.Schema)
	if options.Schema != "" && !validEnum(options.Schema, "rtmp", "gb28181", "rtsp") {
		return PageResult[StreamConverter]{}, &APIError{SafeCode: "request_invalid"}
	}
	if options.Schema != "" {
		query.Set("schema", options.Schema)
	}
	if options.Enabled != nil {
		query.Set("enable", strconv.FormatBool(*options.Enabled))
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/stream-converters", Query: query})
	if err != nil {
		return PageResult[StreamConverter]{}, err
	}
	return decodePage(payload, validateStreamConverter)
}

func validateStreamConverterCreate(input StreamConverterCreateRequest) (StreamConverterCreateRequest, error) {
	var err error
	if input.Name, err = liveRequestString(input.Name, 256); err != nil {
		return input, err
	}
	if input.SN, err = liveRequestString(input.SN, 256); err != nil {
		return input, err
	}
	if input.CameraIndex, err = liveRequestString(input.CameraIndex, 256); err != nil {
		return input, err
	}
	input.Schema = strings.TrimSpace(input.Schema)
	option := &input.SchemaOption
	require := func(values ...string) bool {
		for _, value := range values {
			if !validLiveOpaque(value, maxLiveStringBytes, false) {
				return false
			}
		}
		return true
	}
	valid := false
	switch input.Schema {
	case "rtmp":
		valid = require(option.URL)
	case "gb28181":
		valid = require(option.ServerIP, option.ServerPort, option.DevicePassword, option.LocalPort, option.DeviceID, option.LocalChannel)
	case "rtsp":
		valid = require(option.Username, option.Password) && option.EnableTS != nil
	}
	if !valid {
		return input, &APIError{SafeCode: "request_invalid"}
	}
	return input, nil
}

func (client *Client) CreateStreamConverter(ctx context.Context, token, projectUUID string, input StreamConverterCreateRequest) (StreamConverterCreateResult, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return StreamConverterCreateResult{}, err
	}
	input, err = validateStreamConverterCreate(input)
	if err != nil {
		return StreamConverterCreateResult{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodPost, Path: "/openapi/v2.0/live-stream/converter", Body: input, DisableRetry: true,
	})
	if err != nil {
		return StreamConverterCreateResult{}, err
	}
	var result StreamConverterCreateResult
	if err := json.Unmarshal(payload.Data, &result); err != nil || !validLiveOpaque(result.ID, 256, false) {
		return StreamConverterCreateResult{}, schemaError()
	}
	return result, nil
}

func converterPath(template, converterID string) (string, error) {
	converterID, err := liveRequestString(converterID, 256)
	if err != nil {
		return "", err
	}
	parameter := "converter_id"
	if strings.Contains(template, "{uuid}") {
		parameter = "uuid"
	}
	return resolvePathTemplate(template, map[string]string{parameter: converterID})
}

func (client *Client) SetStreamConverterEnabled(ctx context.Context, token, projectUUID, converterID string, enabled bool) error {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return err
	}
	path, err := converterPath("/openapi/v2.0/live-stream/converter/{uuid}", converterID)
	if err != nil {
		return err
	}
	_, err = client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodPut, Path: path, Body: StreamConverterStateRequest{AutoPushStream: enabled}, DataOptional: true, DisableRetry: true,
	})
	return err
}

func (client *Client) DeleteStreamConverter(ctx context.Context, token, projectUUID, converterID string) error {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return err
	}
	path, err := converterPath("/openapi/v2.0/stream-converters/{converter_id}", converterID)
	if err != nil {
		return err
	}
	_, err = client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodDelete, Path: path, DataOptional: true, DisableRetry: true,
	})
	return err
}

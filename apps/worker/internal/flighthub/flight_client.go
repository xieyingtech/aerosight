package flighthub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxFlightTaskBatch = 100

const maxFlightTaskMedia = 10_000

const maxFlightAlertBatch = 100

type WaylinePayload struct {
	Domain   string `json:"domain"`
	Type     string `json:"type"`
	LensType string `json:"lens_type"`
}

type WaylineSummary struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	PayloadInformation []WaylinePayload `json:"payload_information"`
	DeviceModelKey     string           `json:"device_model_key"`
	TemplateTypes      []string         `json:"template_types"`
	UpdatedAt          int64            `json:"update_time"`
	SizeBytes          int64            `json:"size"`
}

type WaylineDetail struct {
	WaylineSummary
	DownloadURL   string  `json:"download_url"`
	Distance      float64 `json:"distance"`
	WaypointCount int     `json:"wayline_point_nums"`
}

type WaylineUploadCompleteRequest struct {
	Name      string `json:"name"`
	ObjectKey string `json:"object_key"`
}

type WaylineUploadResult struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

type FlightTaskOperation struct {
	OperatorAccount string `json:"operator_account"`
}

type FlightTaskException struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	HappenAt string `json:"happen_at"`
	SN       string `json:"sn"`
}

type FlightTaskSummary struct {
	BelongToSN        string                `json:"belong_to_sn"`
	UUID              string                `json:"uuid"`
	Name              string                `json:"name"`
	TaskType          string                `json:"task_type"`
	Status            string                `json:"status"`
	SN                string                `json:"sn"`
	LandingDockSN     string                `json:"landing_dock_sn"`
	WaylineUUID       string                `json:"wayline_uuid"`
	BeginAt           string                `json:"begin_at"`
	EndAt             string                `json:"end_at"`
	RunAt             string                `json:"run_at"`
	CompletedAt       string                `json:"completed_at"`
	MediaUploadStatus string                `json:"media_upload_status"`
	ResumableStatus   string                `json:"resumable_status"`
	BreakPointResume  bool                  `json:"is_break_point_resume"`
	CurrentWaypoint   int                   `json:"current_waypoint_index"`
	TotalWaypoints    int                   `json:"total_waypoints"`
	FlightType        string                `json:"flight_type"`
	FlightStatus      string                `json:"flight_status"`
	FolderID          int64                 `json:"folder_id"`
	Operations        []FlightTaskOperation `json:"operations"`
	Exceptions        []FlightTaskException `json:"exceptions"`
}

type FlightTaskListOptions struct {
	SNs            []string
	Name           string
	BeginAt        int64
	EndAt          int64
	TaskType       string
	Statuses       []string
	FlightTaskType string
}

type DefaultFlightTaskName struct {
	Name      string `json:"name"`
	IndexName string `json:"index_name"`
}

type FlightTaskFolderInfo struct {
	FolderID          int64 `json:"folder_id"`
	ExpectedFileCount int   `json:"expected_file_count"`
	UploadedFileCount int   `json:"uploaded_file_count"`
}

type FlightTask struct {
	UUID                       string               `json:"uuid"`
	Name                       string               `json:"name"`
	TaskType                   string               `json:"task_type"`
	Status                     string               `json:"status"`
	SN                         string               `json:"sn"`
	WaylineUUID                string               `json:"wayline_uuid"`
	BeginAt                    string               `json:"begin_at"`
	EndAt                      string               `json:"end_at"`
	OutOfControlActionInFlight string               `json:"out_of_control_action_in_flight"`
	RTHAltitude                int                  `json:"rth_altitude"`
	RTHMode                    string               `json:"rth_mode"`
	ResumableStatus            string               `json:"resumable_status"`
	RepeatType                 string               `json:"repeat_type"`
	Interval                   int                  `json:"interval"`
	DaysOfWeek                 []int                `json:"days_of_week"`
	DaysOfMonth                []int                `json:"days_of_month"`
	WeekOfMonth                int                  `json:"week_of_month"`
	RecurringStartTimes        []int64              `json:"recurring_task_start_time_list"`
	ContinuousTaskPeriods      [][]int64            `json:"continuous_task_periods"`
	MinimumBatteryCapacity     int                  `json:"min_battery_capacity"`
	FolderInfo                 FlightTaskFolderInfo `json:"folder_info"`
}

type FlightTaskRelatedUser struct {
	UserName string `json:"user_name"`
	UserID   string `json:"user_id"`
	OperType string `json:"oper_type"`
}

type FlightRecord struct {
	ID               int64  `json:"ID"`
	CreatedAt        string `json:"CreatedAt"`
	FlightPieceUUID  string `json:"flight_piece_id"`
	UploadSN         string `json:"upload_sn"`
	FlightRecordPath string `json:"flight_record_path"`
}

type FlightTaskPiece struct {
	ID                int64                `json:"ID"`
	CreatedAt         string               `json:"created_at"`
	BeginAt           string               `json:"begin_at"`
	EndAt             string               `json:"end_at"`
	RunAt             string               `json:"run_at"`
	CompletedAt       string               `json:"completed_at"`
	FlightTaskUUID    string               `json:"flight_task_id"`
	FlightPieceUUID   string               `json:"flight_piece_id"`
	TakeoffDockSN     string               `json:"take_off_airport_sn"`
	LandingDockSN     string               `json:"land_airport_sn"`
	TaskType          int                  `json:"task_type"`
	Type              string               `json:"type"`
	Status            string               `json:"status"`
	FlightTaskType    int                  `json:"flight_task_type"`
	TaskFlightType    string               `json:"task_flight_type"`
	TaskFlightStatus  string               `json:"task_flight_status"`
	FlightTaskStatus  int                  `json:"flight_task_status"`
	TaskStatus        int                  `json:"task_status"`
	TaskName          string               `json:"task_name"`
	UserID            string               `json:"user_id"`
	ProjectUUID       string               `json:"prj_uuid"`
	WaylineUUID       string               `json:"wayline_uuid"`
	CloudToCloudID    string               `json:"cloud_to_cloud_id"`
	Source            int                  `json:"source"`
	WorkflowUUID      string               `json:"workflow_uuid"`
	Exception         *FlightTaskException `json:"exception"`
	FlightRecords     []FlightRecord       `json:"flight_records"`
	DroneSN           string               `json:"drone_sn"`
	ResumptionUUIDs   []string             `json:"in_flight_uuids"`
	WaylineUUIDs      []string             `json:"wayline_uuids"`
	InFlightInfos     json.RawMessage      `json:"in_flight_infos"`
	TrackInformation  json.RawMessage      `json:"track_information"`
	WaylineExtensions json.RawMessage      `json:"ext"`
	WaylineName       string               `json:"wayline_name"`
	DroneInfo         json.RawMessage      `json:"drone_info"`
}

type FlightTaskDetail struct {
	BeginAt        string                  `json:"begin_at"`
	EndAt          string                  `json:"end_at"`
	CreatedAt      string                  `json:"create_at"`
	RunAt          string                  `json:"run_at"`
	CompletedAt    string                  `json:"completed_at"`
	FlightTaskUUID string                  `json:"flight_task_id"`
	RelatedUsers   []FlightTaskRelatedUser `json:"related_users"`
	TaskType       int                     `json:"task_type"`
	FlightTaskType int                     `json:"flight_task_type"`
	FolderID       int64                   `json:"folder_id"`
	FlightPieces   []FlightTaskPiece       `json:"flight_pieces"`
}

type FlightTaskDispatchWarning struct {
	Code        string `json:"error_code"`
	Type        string `json:"error_type"`
	Description string `json:"description"`
}

type FlightTaskDispatchPosition struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Height    float64 `json:"height"`
}

type FlightTaskDispatchCheck struct {
	Warnings       []FlightTaskDispatchWarning `json:"errors"`
	DevicePosition *FlightTaskDispatchPosition `json:"device_position"`
}

type FlightTaskStatusUpdate struct {
	Status string `json:"status"`
}

type FlightTaskResumptionRequest struct {
	TaskUUID string `json:"task_uuid"`
}

type FlightTaskParent struct {
	UUID string `json:"uuid"`
}

type ResumedFlightTask struct {
	UUID                      string            `json:"uuid"`
	BeginAt                   int64             `json:"begin_at"`
	EndAt                     int64             `json:"end_at"`
	WaylineValidityCheckCodes []int             `json:"wayline_validity_check_codes"`
	ParentTask                *FlightTaskParent `json:"parent_task"`
}

type FlightTaskResumption struct {
	Task ResumedFlightTask `json:"task"`
}

type FlightTrackPoint struct {
	Timestamp int64   `json:"timestamp"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Height    float64 `json:"height"`
}

type FlightTrack struct {
	ID             string             `json:"track_id"`
	DroneSN        string             `json:"drone_sn"`
	FlightDistance int64              `json:"flight_distance"`
	FlightDuration int64              `json:"flight_duration"`
	Points         []FlightTrackPoint `json:"points"`
}

type FlightTaskTrack struct {
	Name        string      `json:"name"`
	WaylineUUID string      `json:"wayline_uuid"`
	Track       FlightTrack `json:"track"`
}

type FlightControlChange struct {
	Time        int64  `json:"control_change_time"`
	UserName    string `json:"user_name"`
	UserID      string `json:"user_id"`
	ControlType string `json:"control_type"`
}

type FlightOperationLog struct {
	Method   string `json:"method"`
	Time     int64  `json:"time"`
	Bid      string `json:"bid"`
	UserName string `json:"user_name"`
	UserID   string `json:"user_id"`
}

type FlightOperationUser struct {
	UserName string `json:"user_name"`
	UserID   string `json:"user_id"`
	OperType string `json:"oper_type"`
}

type FlightTaskOperationTimeline struct {
	ControlChanges []FlightControlChange `json:"control_change"`
	PayloadChanges []FlightControlChange `json:"payload_change"`
	OperationLogs  []FlightOperationLog  `json:"oper_logs"`
	RelatedUsers   []FlightOperationUser `json:"related_users"`
}

type FlightTaskMedia struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	FileType    string `json:"file_type"`
	Suffix      string `json:"suffix"`
	SizeBytes   int64  `json:"size"`
	PreviewURL  string `json:"preview_url"`
	OriginalURL string `json:"original_url"`
	CreatedAt   string `json:"create_at"`
	UpdatedAt   string `json:"update_at"`
}

type FlightExportOptions struct {
	Page        int
	PageSize    int
	ContentType string
	Status      string
	ExportID    string
}

type FlightExportPagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

type FlightExportRecord struct {
	UUID             string   `json:"uuid"`
	CreatedAt        string   `json:"created_at"`
	ExportTime       *string  `json:"export_time"`
	ContentType      string   `json:"content_type"`
	Status           string   `json:"export_status"`
	Progress         int      `json:"progress"`
	FileName         string   `json:"file_name"`
	FileTypes        []string `json:"file_type"`
	ObjectKey        string   `json:"object_key"`
	UserName         string   `json:"user_name"`
	FailedReasonCode int      `json:"failed_reason_code"`
}

type FlightExportPage struct {
	Pagination FlightExportPagination `json:"pagination"`
	List       []FlightExportRecord   `json:"list"`
}

type TemporaryDownload struct {
	URL       string
	ExpiresAt time.Time
}

type FlightAlertOptions struct {
	DroneSN          string
	BeginAt          int64
	EndAt            int64
	AlgorithmSource  *int
	AlgorithmSources []int
	Page             int
	PageSize         int
}

type FlightAlertSummary struct {
	FlightID    string `json:"flight_id"`
	Count       int64  `json:"count"`
	TaskName    string `json:"flight_task_name"`
	TaskType    int    `json:"flight_task_type"`
	StartTime   int64  `json:"start_time"`
	Status      int    `json:"status"`
	IsCommented bool   `json:"is_commented"`
}

type FlightAlertPage struct {
	Data      []FlightAlertSummary `json:"data"`
	Total     int                  `json:"total"`
	Page      int                  `json:"page"`
	PageSize  int                  `json:"page_size"`
	PageCount int                  `json:"page_count"`
}

type AIAlertOptions struct {
	FlightIDs        []string
	DroneSNs         []string
	AlgorithmSources []int
	TargetTypes      []int
	BeginAt          int64
	EndAt            int64
	Page             int
	PageSize         int
}

type AIAlertLocation struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Altitude  *float64 `json:"altitude"`
}

type AIAlertTriggerAction struct {
	Action   int `json:"action"`
	Duration int `json:"duration"`
}

type AIAlertTarget struct {
	TargetType       int     `json:"target_type"`
	Confidence       float64 `json:"target_value"`
	UseMinThreshold  bool    `json:"use_min_threshold"`
	UseMaxThreshold  bool    `json:"use_max_threshold"`
	MaximumThreshold float64 `json:"target_max_threshold"`
	MinimumThreshold float64 `json:"target_min_threshold"`
	Label            string  `json:"label"`
}

type AIAlertRecord struct {
	AlertUUID       string                 `json:"alert_uuid"`
	FlightID        string                 `json:"flight_id"`
	ProjectID       string                 `json:"project_id"`
	DroneSN         string                 `json:"drone_sn"`
	GatewaySN       string                 `json:"gateway_sn"`
	Status          int                    `json:"status"`
	Reason          string                 `json:"reason"`
	AlgorithmSource int                    `json:"algorithm_source"`
	Location        *AIAlertLocation       `json:"location"`
	FileID          int64                  `json:"file_id"`
	MediaIndex      int64                  `json:"media_index"`
	TaskName        string                 `json:"task_name"`
	TriggerActions  []AIAlertTriggerAction `json:"trigger_actions"`
	Targets         []AIAlertTarget        `json:"target_alert_infos"`
	Timestamp       int64                  `json:"timestamp"`
	ThumbnailURL    string                 `json:"thumbnail_url"`
	Labels          []string               `json:"labels"`
	IntervalSeconds int                    `json:"interval_seconds"`
}

type AIAlertPage struct {
	Data      map[string][]AIAlertRecord `json:"data"`
	Total     int                        `json:"total"`
	Page      int                        `json:"page"`
	PageSize  int                        `json:"page_size"`
	PageCount int                        `json:"page_count"`
}

func validateIdentifierList(values []string, minimum, maximum int) ([]string, error) {
	if len(values) < minimum || len(values) > maximum {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	result := make([]string, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		value, err := requireScope(value)
		if err != nil || strings.Contains(value, ",") {
			return nil, &APIError{SafeCode: "request_invalid"}
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, &APIError{SafeCode: "request_invalid"}
		}
		seen[value] = struct{}{}
		result[index] = value
	}
	return result, nil
}

func validEnum(value string, allowed ...string) bool {
	if value == "" {
		return true
	}
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func validTimestamp(value string) bool {
	if value == "" {
		return true
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func validFlightOperationCode(value string) bool {
	if value != strings.TrimSpace(value) || len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '-' || character == '.' || character == ':' {
			continue
		}
		return false
	}
	return true
}

func requireFlightTaskID(value string) (string, error) {
	value, err := requireScope(value)
	if err != nil || strings.ContainsAny(value, "/\\?#&=") {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return value, nil
}

func validFlightMedia(item *FlightTaskMedia) bool {
	if strings.TrimSpace(item.UUID) == "" || strings.TrimSpace(item.Name) == "" || item.SizeBytes < 0 ||
		!validEnum(item.FileType, "image", "video", "ppk", "model_2d", "model_3d", "unsupported") ||
		!validTimestamp(item.CreatedAt) || item.CreatedAt == "" || !validTimestamp(item.UpdatedAt) || item.UpdatedAt == "" {
		return false
	}
	if len(item.Suffix) > 32 || strings.ContainsAny(item.Suffix, "/\\\x00\r\n") {
		return false
	}
	return strings.TrimSpace(item.OriginalURL) != ""
}

func validObjectKey(value string) bool {
	trimmed := strings.TrimSpace(value)
	return value == trimmed && trimmed != "" && len(trimmed) <= 1024 && !strings.HasPrefix(trimmed, "/") &&
		!strings.Contains(trimmed, "..") && !strings.ContainsAny(trimmed, "\\\x00\r\n")
}

func validFlightExport(item *FlightExportRecord) bool {
	if strings.TrimSpace(item.UUID) == "" || strings.TrimSpace(item.FileName) == "" ||
		!validTimestamp(item.CreatedAt) || item.CreatedAt == "" ||
		!validEnum(item.ContentType, "summary", "details") ||
		!validEnum(item.Status, "export_in_progress", "export_complete", "export_failed") ||
		item.Progress < 0 || item.Progress > 100 || item.FileTypes == nil {
		return false
	}
	if item.ExportTime != nil && (*item.ExportTime == "" || !validTimestamp(*item.ExportTime)) {
		return false
	}
	for _, fileType := range item.FileTypes {
		if _, err := optionalQuery(fileType); err != nil || strings.TrimSpace(fileType) == "" {
			return false
		}
	}
	if item.Status == "export_complete" {
		return item.Progress == 100 && validObjectKey(item.ObjectKey)
	}
	return strings.TrimSpace(item.ObjectKey) == "" || validObjectKey(item.ObjectKey)
}

func validateWayline(item *WaylineSummary) bool {
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.DeviceModelKey) == "" || item.UpdatedAt < 0 || item.SizeBytes < 0 || item.TemplateTypes == nil || item.PayloadInformation == nil {
		return false
	}
	for _, payload := range item.PayloadInformation {
		if strings.TrimSpace(payload.Domain) == "" || strings.TrimSpace(payload.Type) == "" || strings.TrimSpace(payload.LensType) == "" {
			return false
		}
	}
	return true
}

func validateFlightTaskSummary(item *FlightTaskSummary) bool {
	if strings.TrimSpace(item.UUID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.TaskType) == "" || strings.TrimSpace(item.Status) == "" || strings.TrimSpace(item.SN) == "" || strings.TrimSpace(item.WaylineUUID) == "" {
		return false
	}
	if !validEnum(item.TaskType, "immediate", "timed", "recurring", "continuous") ||
		!validEnum(item.Status, "waiting", "starting_failure", "executing", "paused", "terminated", "success", "suspended", "timeout", "partially_done", "preparing", "queue_for_takeoff") ||
		!validTimestamp(item.BeginAt) || !validTimestamp(item.EndAt) || !validTimestamp(item.RunAt) || !validTimestamp(item.CompletedAt) {
		return false
	}
	return item.CurrentWaypoint >= 0 && item.TotalWaypoints >= 0
}

func decodeFlightTaskList(payload envelope, allowNull bool) ([]FlightTaskSummary, error) {
	return decodeList(payload, allowNull, validateFlightTaskSummary)
}

func (client *Client) ListWaylines(ctx context.Context, token, projectUUID string) ([]WaylineSummary, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/wayline"})
	if err != nil {
		return nil, err
	}
	return decodeList(payload, false, validateWayline)
}

func (client *Client) GetWayline(ctx context.Context, token, projectUUID, waylineID string) (WaylineDetail, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return WaylineDetail{}, err
	}
	waylineID, err = requireScope(waylineID)
	if err != nil {
		return WaylineDetail{}, &APIError{SafeCode: "request_invalid"}
	}
	path, err := resolvePathTemplate("/openapi/v2.0/wayline/{wayline_id}", map[string]string{"wayline_id": waylineID})
	if err != nil {
		return WaylineDetail{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return WaylineDetail{}, err
	}
	var item WaylineDetail
	if err := json.Unmarshal(payload.Data, &item); err != nil || !validateWayline(&item.WaylineSummary) || item.ID != waylineID || strings.TrimSpace(item.DownloadURL) == "" || item.Distance < 0 || item.WaypointCount < 0 {
		return WaylineDetail{}, schemaError()
	}
	if _, err := client.validateResponseLink(LinkDownload, item.DownloadURL); err != nil {
		return WaylineDetail{}, err
	}
	return item, nil
}

func (client *Client) NotifyWaylineUploadComplete(ctx context.Context, token, projectUUID string, input WaylineUploadCompleteRequest) (WaylineUploadResult, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return WaylineUploadResult{}, err
	}
	input.Name, err = optionalQuery(input.Name)
	if err != nil || input.Name == "" {
		return WaylineUploadResult{}, &APIError{SafeCode: "request_invalid"}
	}
	input.ObjectKey = strings.TrimSpace(input.ObjectKey)
	if input.ObjectKey == "" || len(input.ObjectKey) > 1024 || strings.HasPrefix(input.ObjectKey, "/") || strings.Contains(input.ObjectKey, "..") || strings.ContainsAny(input.ObjectKey, "\x00\r\n\\") {
		return WaylineUploadResult{}, &APIError{SafeCode: "request_invalid"}
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodPost, Path: "/openapi/v2.0/wayline/finish-upload", Body: input, DisableRetry: true,
	})
	if err != nil {
		return WaylineUploadResult{}, err
	}
	var result WaylineUploadResult
	if err := json.Unmarshal(payload.Data, &result); err != nil || strings.TrimSpace(result.Name) == "" || strings.TrimSpace(result.UUID) == "" {
		return WaylineUploadResult{}, schemaError()
	}
	return result, nil
}

func (client *Client) ListFlightTasks(ctx context.Context, token, projectUUID string, options FlightTaskListOptions) ([]FlightTaskSummary, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return nil, err
	}
	serials, err := validateIdentifierList(options.SNs, 1, maxFlightTaskBatch)
	if err != nil {
		return nil, err
	}
	if (options.BeginAt == 0) != (options.EndAt == 0) || options.BeginAt < 0 || options.EndAt < options.BeginAt {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	options.TaskType, err = optionalQuery(options.TaskType)
	if err != nil {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	options.FlightTaskType, err = optionalQuery(options.FlightTaskType)
	if err != nil {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	statuses, err := validateIdentifierList(options.Statuses, 0, 16)
	if err != nil {
		return nil, err
	}
	for _, status := range statuses {
		if !validEnum(status, "waiting", "starting_failure", "executing", "paused", "terminated", "success", "suspended", "timeout", "partially_done", "preparing", "queue_for_takeoff") {
			return nil, &APIError{SafeCode: "request_invalid"}
		}
	}
	query := url.Values{"sn": {strings.Join(serials, ",")}}
	if err := addOptionalQuery(query, "name", options.Name); err != nil {
		return nil, err
	}
	if options.BeginAt > 0 {
		query.Set("begin_at", strconv.FormatInt(options.BeginAt, 10))
		query.Set("end_at", strconv.FormatInt(options.EndAt, 10))
	}
	if options.TaskType != "" {
		query.Set("task_type", options.TaskType)
	}
	if len(statuses) > 0 {
		query.Set("status", strings.Join(statuses, ","))
	}
	if options.FlightTaskType != "" {
		query.Set("flight_task_type", options.FlightTaskType)
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/flight-task/list", Query: query})
	if err != nil {
		return nil, err
	}
	return decodeFlightTaskList(payload, true)
}

func workspacePath(template, workspaceID string) (string, error) {
	workspaceID, err := requireScope(workspaceID)
	if err != nil {
		return "", err
	}
	return resolvePathTemplate(template, map[string]string{"workspace_id": workspaceID})
}

func (client *Client) ListRecentFlightTasks(ctx context.Context, token, workspaceID string, serials []string) ([]FlightTaskSummary, error) {
	path, err := workspacePath("/openapi/v2.0/workspaces/{workspace_id}/flight-tasks/recent", workspaceID)
	if err != nil {
		return nil, err
	}
	serials, err = validateIdentifierList(serials, 1, maxFlightTaskBatch)
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path, Query: url.Values{"sn[]": serials}})
	if err != nil {
		return nil, err
	}
	return decodeFlightTaskList(payload, false)
}

func (client *Client) BatchGetFlightTasks(ctx context.Context, token, workspaceID string, taskUUIDs []string) ([]FlightTaskSummary, error) {
	path, err := workspacePath("/openapi/v2.0/workspaces/{workspace_id}/flight-tasks/batch", workspaceID)
	if err != nil {
		return nil, err
	}
	taskUUIDs, err = validateIdentifierList(taskUUIDs, 1, maxFlightTaskBatch)
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path, Query: url.Values{"task_uuids": taskUUIDs}})
	if err != nil {
		return nil, err
	}
	return decodeFlightTaskList(payload, false)
}

func (client *Client) GetDefaultFlightTaskName(ctx context.Context, token, workspaceID, name string) (DefaultFlightTaskName, error) {
	path, err := workspacePath("/openapi/v2.0/workspaces/{workspace_id}/flight-tasks/default-name", workspaceID)
	if err != nil {
		return DefaultFlightTaskName{}, err
	}
	name, err = optionalQuery(name)
	if err != nil || name == "" {
		return DefaultFlightTaskName{}, &APIError{SafeCode: "request_invalid"}
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path, Query: url.Values{"name": {name}}})
	if err != nil {
		return DefaultFlightTaskName{}, err
	}
	var result DefaultFlightTaskName
	if err := json.Unmarshal(payload.Data, &result); err != nil || (strings.TrimSpace(result.Name) == "" && strings.TrimSpace(result.IndexName) == "") {
		return DefaultFlightTaskName{}, schemaError()
	}
	return result, nil
}

func (client *Client) GetFlightTaskDetail(ctx context.Context, token, projectUUID, taskUUID string) (FlightTaskDetail, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return FlightTaskDetail{}, err
	}
	taskUUID, err = requireFlightTaskID(taskUUID)
	if err != nil {
		return FlightTaskDetail{}, &APIError{SafeCode: "request_invalid"}
	}
	query := url.Values{"workspace_id": {projectUUID}, "task_uuid": {taskUUID}}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/flight-task/detail", Query: query})
	if err != nil {
		return FlightTaskDetail{}, err
	}
	var detail FlightTaskDetail
	if err := json.Unmarshal(payload.Data, &detail); err != nil || strings.TrimSpace(detail.FlightTaskUUID) == "" || detail.FlightTaskUUID != taskUUID || detail.FlightPieces == nil || !validTimestamp(detail.BeginAt) || !validTimestamp(detail.EndAt) || !validTimestamp(detail.CreatedAt) || !validTimestamp(detail.RunAt) || !validTimestamp(detail.CompletedAt) {
		return FlightTaskDetail{}, schemaError()
	}
	for _, piece := range detail.FlightPieces {
		if strings.TrimSpace(piece.FlightTaskUUID) == "" || strings.TrimSpace(piece.FlightPieceUUID) == "" || strings.TrimSpace(piece.Status) == "" || !validTimestamp(piece.CreatedAt) || !validTimestamp(piece.BeginAt) || !validTimestamp(piece.EndAt) || !validTimestamp(piece.RunAt) || !validTimestamp(piece.CompletedAt) {
			return FlightTaskDetail{}, schemaError()
		}
	}
	return detail, nil
}

func (client *Client) GetFlightTask(ctx context.Context, token, projectUUID, taskUUID string) (FlightTask, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return FlightTask{}, err
	}
	taskUUID, err = requireFlightTaskID(taskUUID)
	if err != nil {
		return FlightTask{}, &APIError{SafeCode: "request_invalid"}
	}
	path, err := resolvePathTemplate("/openapi/v2.0/flight-task/{task_uuid}", map[string]string{"task_uuid": taskUUID})
	if err != nil {
		return FlightTask{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path, Query: url.Values{"workspace_id": {projectUUID}}})
	if err != nil {
		return FlightTask{}, err
	}
	var task FlightTask
	if err := json.Unmarshal(payload.Data, &task); err != nil || task.UUID != taskUUID || strings.TrimSpace(task.Name) == "" || strings.TrimSpace(task.TaskType) == "" || strings.TrimSpace(task.Status) == "" || strings.TrimSpace(task.SN) == "" || strings.TrimSpace(task.WaylineUUID) == "" || !validTimestamp(task.BeginAt) || !validTimestamp(task.EndAt) {
		return FlightTask{}, schemaError()
	}
	return task, nil
}

func (client *Client) CheckFlightTaskDispatch(ctx context.Context, token, workspaceID, serial, waylineUUID string) (FlightTaskDispatchCheck, error) {
	path, err := workspacePath("/openapi/v2.0/workspaces/{workspace_id}/flight-tasks/dispatch-checks", workspaceID)
	if err != nil {
		return FlightTaskDispatchCheck{}, err
	}
	serial, err = requireScope(serial)
	if err != nil {
		return FlightTaskDispatchCheck{}, &APIError{SafeCode: "request_invalid"}
	}
	waylineUUID, err = requireScope(waylineUUID)
	if err != nil {
		return FlightTaskDispatchCheck{}, &APIError{SafeCode: "request_invalid"}
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path, Query: url.Values{"sn": {serial}, "wayline_uuid": {waylineUUID}}})
	if err != nil {
		return FlightTaskDispatchCheck{}, err
	}
	var result FlightTaskDispatchCheck
	if err := json.Unmarshal(payload.Data, &result); err != nil {
		return FlightTaskDispatchCheck{}, schemaError()
	}
	for _, warning := range result.Warnings {
		if strings.TrimSpace(warning.Code) == "" || strings.TrimSpace(warning.Type) == "" || !validEnum(warning.Type, "warning", "info") {
			return FlightTaskDispatchCheck{}, schemaError()
		}
	}
	if result.DevicePosition == nil || result.DevicePosition.Latitude < -90 || result.DevicePosition.Latitude > 90 || result.DevicePosition.Longitude < -180 || result.DevicePosition.Longitude > 180 {
		return FlightTaskDispatchCheck{}, schemaError()
	}
	return result, nil
}

func (client *Client) UpdateFlightTaskStatus(ctx context.Context, token, projectUUID, taskUUID, status string) error {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return err
	}
	taskUUID, err = requireScope(taskUUID)
	if err != nil {
		return &APIError{SafeCode: "request_invalid"}
	}
	status = strings.TrimSpace(status)
	if !validEnum(status, "suspended", "restored") || status == "" {
		return &APIError{SafeCode: "request_invalid"}
	}
	path, err := resolvePathTemplate("/openapi/v2.0/flight-task/{task_uuid}/status", map[string]string{"task_uuid": taskUUID})
	if err != nil {
		return err
	}
	_, err = client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodPut, Path: path, Body: FlightTaskStatusUpdate{Status: status}, DataOptional: true})
	return err
}

func (client *Client) CreateFlightTaskResumption(ctx context.Context, token, projectUUID, workspaceID, taskUUID string) (FlightTaskResumption, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return FlightTaskResumption{}, err
	}
	path, err := workspacePath("/openapi/v2.0/workspaces/{workspace_id}/flight-tasks/resumptions", workspaceID)
	if err != nil {
		return FlightTaskResumption{}, err
	}
	taskUUID, err = requireScope(taskUUID)
	if err != nil {
		return FlightTaskResumption{}, &APIError{SafeCode: "request_invalid"}
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodPost, Path: path, Body: FlightTaskResumptionRequest{TaskUUID: taskUUID}})
	if err != nil {
		return FlightTaskResumption{}, err
	}
	var result FlightTaskResumption
	if err := json.Unmarshal(payload.Data, &result); err != nil || strings.TrimSpace(result.Task.UUID) == "" || result.Task.BeginAt <= 0 || result.Task.EndAt < result.Task.BeginAt || (result.Task.ParentTask != nil && strings.TrimSpace(result.Task.ParentTask.UUID) == "") {
		return FlightTaskResumption{}, schemaError()
	}
	return result, nil
}

func (client *Client) GetFlightTaskTrack(ctx context.Context, token, projectUUID, taskUUID string) (FlightTaskTrack, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return FlightTaskTrack{}, err
	}
	taskUUID, err = requireFlightTaskID(taskUUID)
	if err != nil {
		return FlightTaskTrack{}, &APIError{SafeCode: "request_invalid"}
	}
	path, err := resolvePathTemplate("/openapi/v2.0/flight-task/{task_uuid}/track", map[string]string{"task_uuid": taskUUID})
	if err != nil {
		return FlightTaskTrack{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return FlightTaskTrack{}, err
	}
	var result FlightTaskTrack
	if err := json.Unmarshal(payload.Data, &result); err != nil || strings.TrimSpace(result.Track.ID) == "" || strings.TrimSpace(result.Track.DroneSN) == "" || result.Track.FlightDistance < 0 || result.Track.FlightDuration < 0 || result.Track.Points == nil {
		return FlightTaskTrack{}, schemaError()
	}
	for _, point := range result.Track.Points {
		if point.Timestamp <= 0 {
			return FlightTaskTrack{}, schemaError()
		}
	}
	return result, nil
}

func (client *Client) GetFlightTaskOperationTimeline(ctx context.Context, token, projectUUID, taskUUID string) (FlightTaskOperationTimeline, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return FlightTaskOperationTimeline{}, err
	}
	taskUUID, err = requireFlightTaskID(taskUUID)
	if err != nil {
		return FlightTaskOperationTimeline{}, &APIError{SafeCode: "request_invalid"}
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodGet, Path: "/openapi/v2.0/flight-task/oper", Query: url.Values{"task_uuid": {taskUUID}},
	})
	if err != nil {
		return FlightTaskOperationTimeline{}, err
	}
	var result FlightTaskOperationTimeline
	if err := json.Unmarshal(payload.Data, &result); err != nil || result.ControlChanges == nil || result.PayloadChanges == nil || result.OperationLogs == nil || result.RelatedUsers == nil {
		return FlightTaskOperationTimeline{}, schemaError()
	}
	for _, change := range append(append([]FlightControlChange(nil), result.ControlChanges...), result.PayloadChanges...) {
		if change.Time <= 0 || !validFlightOperationCode(change.ControlType) {
			return FlightTaskOperationTimeline{}, schemaError()
		}
	}
	for _, item := range result.OperationLogs {
		if item.Time <= 0 || !validFlightOperationCode(item.Method) {
			return FlightTaskOperationTimeline{}, schemaError()
		}
	}
	return result, nil
}

func (client *Client) ListFlightTaskMedia(ctx context.Context, token, projectUUID, taskUUID string) ([]FlightTaskMedia, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return nil, err
	}
	taskUUID, err = requireFlightTaskID(taskUUID)
	if err != nil {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	path, err := resolvePathTemplate("/openapi/v2.0/flight-task/{task_uuid}/media", map[string]string{"task_uuid": taskUUID})
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return nil, err
	}
	items, err := decodeList(payload, false, validFlightMedia)
	if err != nil {
		return nil, err
	}
	if len(items) >= maxFlightTaskMedia {
		return nil, &APIError{SafeCode: "directory_limit_reached", Retryable: true}
	}
	for _, item := range items {
		if item.PreviewURL != "" {
			if _, err := client.validateResponseLink(LinkDownload, item.PreviewURL); err != nil {
				return nil, err
			}
		}
		if _, err := client.validateResponseLink(LinkDownload, item.OriginalURL); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (client *Client) ListFlightTaskExports(ctx context.Context, token, projectUUID string, options FlightExportOptions) (FlightExportPage, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return FlightExportPage{}, err
	}
	if options.Page == 0 {
		options.Page = 1
	}
	if options.PageSize == 0 {
		options.PageSize = 100
	}
	if options.Page < 1 || options.PageSize < 1 || options.PageSize > 100 ||
		!validEnum(options.ContentType, "summary", "details") ||
		!validEnum(options.Status, "export_in_progress", "export_complete", "export_failed") {
		return FlightExportPage{}, &APIError{SafeCode: "request_invalid"}
	}
	query := url.Values{
		"page":      {strconv.Itoa(options.Page)},
		"page_size": {strconv.Itoa(options.PageSize)},
	}
	if options.ContentType != "" {
		query.Set("content_type", options.ContentType)
	}
	if options.Status != "" {
		query.Set("status", options.Status)
	}
	if options.ExportID != "" {
		exportID, exportErr := requireFlightTaskID(options.ExportID)
		if exportErr != nil {
			return FlightExportPage{}, &APIError{SafeCode: "request_invalid"}
		}
		query.Set("export_id", exportID)
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodGet, Path: "/openapi/v2.0/flight-task/export", Query: query,
	})
	if err != nil {
		return FlightExportPage{}, err
	}
	var result FlightExportPage
	if err := json.Unmarshal(payload.Data, &result); err != nil || result.List == nil ||
		result.Pagination.Page < 0 || result.Pagination.PageSize < 0 || result.Pagination.Total < 0 {
		return FlightExportPage{}, schemaError()
	}
	if options.ExportID == "" && (result.Pagination.Page != options.Page || result.Pagination.PageSize != options.PageSize || len(result.List) > options.PageSize) {
		return FlightExportPage{}, schemaError()
	}
	if options.ExportID != "" && len(result.List) > 1 {
		return FlightExportPage{}, schemaError()
	}
	for index := range result.List {
		if !validFlightExport(&result.List[index]) {
			return FlightExportPage{}, schemaError()
		}
	}
	return result, nil
}

func normalizedAlertPage(page, pageSize int) (int, int, error) {
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 50
	}
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return 0, 0, &APIError{SafeCode: "request_invalid"}
	}
	return page, pageSize, nil
}

func addIntegerList(query url.Values, key string, values []int, repeated bool) error {
	if len(values) > 32 {
		return &APIError{SafeCode: "request_invalid"}
	}
	seen := make(map[int]struct{}, len(values))
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		if value < 0 {
			return &APIError{SafeCode: "request_invalid"}
		}
		if _, duplicate := seen[value]; duplicate {
			return &APIError{SafeCode: "request_invalid"}
		}
		seen[value] = struct{}{}
		encoded = append(encoded, strconv.Itoa(value))
	}
	if repeated {
		for _, value := range encoded {
			query.Add(key, value)
		}
	} else if len(encoded) > 0 {
		query.Set(key, strings.Join(encoded, ","))
	}
	return nil
}

func validAlertPagination(page, pageSize, total, pageCount, itemCount int) bool {
	if page < 1 || pageSize < 1 || total < 0 || pageCount < 0 || itemCount < 0 || itemCount > pageSize || itemCount > total {
		return false
	}
	wantPages := 0
	if total > 0 {
		wantPages = (total + pageSize - 1) / pageSize
	}
	return pageCount == wantPages && (pageCount == 0 || page <= pageCount)
}

func (client *Client) ListFlightAlerts(ctx context.Context, token, projectUUID string, options FlightAlertOptions) (FlightAlertPage, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return FlightAlertPage{}, err
	}
	droneSN, err := requireScope(options.DroneSN)
	if err != nil {
		return FlightAlertPage{}, &APIError{SafeCode: "request_invalid"}
	}
	if (options.BeginAt == 0) != (options.EndAt == 0) || options.BeginAt < 0 || options.EndAt < options.BeginAt {
		return FlightAlertPage{}, &APIError{SafeCode: "request_invalid"}
	}
	page, pageSize, err := normalizedAlertPage(options.Page, options.PageSize)
	if err != nil {
		return FlightAlertPage{}, err
	}
	query := url.Values{
		"drone_sn": {droneSN}, "page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(pageSize)},
	}
	if options.BeginAt > 0 {
		query.Set("begin_at", strconv.FormatInt(options.BeginAt, 10))
		query.Set("end_at", strconv.FormatInt(options.EndAt, 10))
	}
	if options.AlgorithmSource != nil {
		if *options.AlgorithmSource < 0 {
			return FlightAlertPage{}, &APIError{SafeCode: "request_invalid"}
		}
		query.Set("algorithm_source", strconv.Itoa(*options.AlgorithmSource))
	}
	if err := addIntegerList(query, "algorithm_sources[]", options.AlgorithmSources, true); err != nil {
		return FlightAlertPage{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/flight-alerts", Query: query})
	if err != nil {
		return FlightAlertPage{}, err
	}
	var result FlightAlertPage
	if err := json.Unmarshal(payload.Data, &result); err != nil || result.Data == nil || result.Page != page || result.PageSize != pageSize ||
		!validAlertPagination(result.Page, result.PageSize, result.Total, result.PageCount, len(result.Data)) {
		return FlightAlertPage{}, schemaError()
	}
	seen := make(map[string]struct{}, len(result.Data))
	for _, item := range result.Data {
		if strings.TrimSpace(item.FlightID) == "" || item.Count < 0 || item.TaskType < 0 || item.TaskType > 2 || item.StartTime <= 0 || item.Status < 0 || item.Status > 1 {
			return FlightAlertPage{}, schemaError()
		}
		if _, duplicate := seen[item.FlightID]; duplicate {
			return FlightAlertPage{}, schemaError()
		}
		seen[item.FlightID] = struct{}{}
	}
	return result, nil
}

func (client *Client) ListAIAlertRecords(ctx context.Context, token, projectUUID string, options AIAlertOptions) (AIAlertPage, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return AIAlertPage{}, err
	}
	flightIDs, err := validateIdentifierList(options.FlightIDs, 1, maxFlightAlertBatch)
	if err != nil {
		return AIAlertPage{}, err
	}
	droneSNs, err := validateIdentifierList(options.DroneSNs, 0, maxFlightAlertBatch)
	if err != nil {
		return AIAlertPage{}, err
	}
	if (options.BeginAt == 0) != (options.EndAt == 0) || options.BeginAt < 0 || options.EndAt < options.BeginAt {
		return AIAlertPage{}, &APIError{SafeCode: "request_invalid"}
	}
	page, pageSize, err := normalizedAlertPage(options.Page, options.PageSize)
	if err != nil {
		return AIAlertPage{}, err
	}
	query := url.Values{
		"flight_id": {strings.Join(flightIDs, ",")}, "page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(pageSize)},
	}
	if len(droneSNs) > 0 {
		query.Set("drone_sn", strings.Join(droneSNs, ","))
	}
	if options.BeginAt > 0 {
		query.Set("begin_at", strconv.FormatInt(options.BeginAt, 10))
		query.Set("end_at", strconv.FormatInt(options.EndAt, 10))
	}
	if err := addIntegerList(query, "algorithm_sources", options.AlgorithmSources, false); err != nil {
		return AIAlertPage{}, err
	}
	if err := addIntegerList(query, "target_type", options.TargetTypes, false); err != nil {
		return AIAlertPage{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/ai-alert-record", Query: query})
	if err != nil {
		return AIAlertPage{}, err
	}
	var result AIAlertPage
	if err := json.Unmarshal(payload.Data, &result); err != nil || result.Data == nil || result.Page != page || result.PageSize != pageSize {
		return AIAlertPage{}, schemaError()
	}
	requested := make(map[string]struct{}, len(flightIDs))
	for _, flightID := range flightIDs {
		requested[flightID] = struct{}{}
	}
	seen := make(map[string]struct{})
	itemCount := 0
	for flightID, items := range result.Data {
		if _, ok := requested[flightID]; !ok || items == nil {
			return AIAlertPage{}, schemaError()
		}
		for index := range items {
			item := &items[index]
			if item.FlightID != flightID || strings.TrimSpace(item.AlertUUID) == "" || strings.TrimSpace(item.ProjectID) == "" || strings.TrimSpace(item.DroneSN) == "" ||
				item.Status < 0 || item.Status > 5 || item.AlgorithmSource < 0 || item.Timestamp <= 0 || item.FileID < 0 || item.MediaIndex < 0 || item.IntervalSeconds < 0 {
				return AIAlertPage{}, schemaError()
			}
			if _, duplicate := seen[item.AlertUUID]; duplicate {
				return AIAlertPage{}, schemaError()
			}
			seen[item.AlertUUID] = struct{}{}
			for _, action := range item.TriggerActions {
				if action.Action < 0 || action.Action > 2 || action.Duration < 0 {
					return AIAlertPage{}, schemaError()
				}
			}
			for _, target := range item.Targets {
				if target.TargetType < 0 || target.TargetType > 5 || target.Confidence < 0 || target.Confidence > 1 ||
					target.MinimumThreshold < 0 || target.MaximumThreshold < 0 {
					return AIAlertPage{}, schemaError()
				}
			}
			if item.ThumbnailURL != "" {
				if _, err := client.validateDownload(item.ThumbnailURL, 12*time.Hour); err != nil {
					return AIAlertPage{}, err
				}
			}
			itemCount++
		}
		result.Data[flightID] = items
	}
	if !validAlertPagination(result.Page, result.PageSize, result.Total, result.PageCount, itemCount) {
		return AIAlertPage{}, schemaError()
	}
	return result, nil
}

func (client *Client) GetFlightRecordDownloadURL(ctx context.Context, token, projectUUID, objectKey string) (TemporaryDownload, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return TemporaryDownload{}, err
	}
	if !validObjectKey(objectKey) {
		return TemporaryDownload{}, &APIError{SafeCode: "request_invalid"}
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodGet, Path: "/openapi/v2.0/flight-task/oss-url-info/get", Query: url.Values{"object_key": {objectKey}},
	})
	if err != nil {
		return TemporaryDownload{}, err
	}
	var raw string
	if err := json.Unmarshal(payload.Data, &raw); err != nil || strings.TrimSpace(raw) == "" {
		return TemporaryDownload{}, schemaError()
	}
	return client.validateDownload(raw, time.Hour)
}

func (client *Client) validateDownload(raw string, documentedTTL time.Duration) (TemporaryDownload, error) {
	parsed, err := client.validateResponseLink(LinkDownload, raw)
	if err != nil {
		return TemporaryDownload{}, err
	}
	expiresAt := temporaryLinkExpiry(parsed)
	if expiresAt.IsZero() && documentedTTL > 0 {
		expiresAt = client.now().UTC().Add(documentedTTL)
	}
	if _, err := client.ValidateTemporaryLink(LinkDownload, raw, expiresAt); err != nil {
		return TemporaryDownload{}, err
	}
	return TemporaryDownload{URL: raw, ExpiresAt: expiresAt.UTC()}, nil
}

func temporaryLinkExpiry(parsed *url.URL) time.Time {
	query := parsed.Query()
	for _, key := range []string{"auth_key", "Expires", "expires"} {
		value := strings.TrimSpace(query.Get(key))
		if value == "" {
			continue
		}
		if key == "auth_key" {
			value, _, _ = strings.Cut(value, "-")
		}
		if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 {
			return time.Unix(seconds, 0).UTC()
		}
	}
	for _, prefix := range []string{"X-Amz-", "X-Goog-"} {
		dateValue := query.Get(prefix + "Date")
		ttlValue := query.Get(prefix + "Expires")
		issuedAt, dateErr := time.Parse("20060102T150405Z", dateValue)
		ttlSeconds, ttlErr := strconv.ParseInt(ttlValue, 10, 64)
		if dateErr == nil && ttlErr == nil && ttlSeconds > 0 {
			return issuedAt.UTC().Add(time.Duration(ttlSeconds) * time.Second)
		}
	}
	return time.Time{}
}

func (client *Client) RefreshFlightTaskMediaURL(ctx context.Context, token, projectUUID, taskUUID, mediaUUID string) (TemporaryDownload, error) {
	mediaUUID, err := requireFlightTaskID(mediaUUID)
	if err != nil {
		return TemporaryDownload{}, &APIError{SafeCode: "request_invalid"}
	}
	items, err := client.ListFlightTaskMedia(ctx, token, projectUUID, taskUUID)
	if err != nil {
		return TemporaryDownload{}, err
	}
	for _, item := range items {
		if item.UUID == mediaUUID {
			return client.validateDownload(item.OriginalURL, 0)
		}
	}
	return TemporaryDownload{}, &APIError{SafeCode: "scope_not_found"}
}

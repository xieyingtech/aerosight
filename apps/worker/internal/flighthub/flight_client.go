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
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodPost, Path: "/openapi/v2.0/wayline/finish-upload", Body: input})
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

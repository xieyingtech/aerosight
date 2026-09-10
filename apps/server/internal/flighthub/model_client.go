package flighthub

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	maxModelStringBytes     = 4096
	maxModelParameterBytes  = 16 << 10
	maxModelCredentialBytes = 16 << 10
	maxOpenModelFiles       = 100
)

type ModelFileType string

const (
	ModelFile2D ModelFileType = "model_2d"
	ModelFile3D ModelFileType = "model_3d"
)

type ModelSummary struct {
	ID        int64         `json:"id"`
	Name      string        `json:"name"`
	FileType  ModelFileType `json:"file_type"`
	ShowOnMap bool          `json:"show_on_map"`
	Size      int64         `json:"size"`
	UpdatedAt int64         `json:"update_at"`
	CreatedAt int64         `json:"create_at"`
}

// ModelDetail URLs are vendor-provided protected references. Callers must not
// persist or expose them outside a project-authorized short-lived workflow.
type ModelDetail struct {
	ID         int64         `json:"id"`
	Name       string        `json:"name"`
	UserName   string        `json:"user_name"`
	URL        string        `json:"url"`
	PreviewURL string        `json:"preview_url"`
	ShowOnMap  bool          `json:"show_on_map"`
	Size       int64         `json:"size"`
	UpdatedAt  int64         `json:"update_at"`
	CreatedAt  int64         `json:"create_at"`
	FileType   ModelFileType `json:"file_type"`
}

type ModelDownload struct {
	ID        int64
	URL       string
	ExpiresAt time.Time
	Ready     bool
}

type ModelReconstructionPoint struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

type ModelReconstructionArea struct {
	PolygonPoints []ModelReconstructionPoint `json:"polygon_points"`
}

type ModelReconstructionRequest struct {
	Name                 string                   `json:"name"`
	ReconstructionTypes  []ModelFileType          `json:"reconstruction_type"`
	SimplifiedFactor     float64                  `json:"simplified_factor"`
	TaskFolderID         int64                    `json:"task_folder_id"`
	WKT                  string                   `json:"wkt"`
	QualityLevel         string                   `json:"quality_level"`
	ReconstructionMode   string                   `json:"reconstruction_mode"`
	GenerateModelFormats []string                 `json:"generate_model_formats"`
	PredefineArea        *ModelReconstructionArea `json:"predefine_area,omitempty"`
}

type ModelReconstructionResult struct {
	ID int64 `json:"id"`
}

type OpenModelType int

const (
	OpenModel2D    OpenModelType = 1
	OpenModel3D    OpenModelType = 2
	OpenModel3DGS  OpenModelType = 3
	OpenModelLidar OpenModelType = 4
)

type OpenModelStatus int

const (
	OpenModelReconstructionFailed       OpenModelStatus = 8
	OpenModelMapReconstructionSucceeded OpenModelStatus = 9
	OpenModelRequestingResource         OpenModelStatus = 12
	OpenModelRequestingResourceFailed   OpenModelStatus = 13
	OpenModelReconstructionExecuting    OpenModelStatus = 14
	OpenModelReconstructionSucceeded    OpenModelStatus = 15
	OpenModelReconstructionCanceled     OpenModelStatus = 16
)

type OpenModelZipStatus int

const (
	OpenModelZipInitial OpenModelZipStatus = iota
	OpenModelZipRunning
	OpenModelZipFinished
	OpenModelZipFailed
)

type OpenModel struct {
	ResourceUUID           string             `json:"resource_uuid"`
	ModelUUID              string             `json:"model_uuid"`
	ModelType              OpenModelType      `json:"model_type"`
	ModelStatus            OpenModelStatus    `json:"model_status"`
	ModelSize              int64              `json:"model_size"`
	ReconstructionProgress int                `json:"reconstruction_progress"`
	ErrorCode              int                `json:"error_code"`
	ZipStatus              OpenModelZipStatus `json:"zip_status"`
	ZipProgress            int                `json:"zip_progress"`
	ZipFileKey             string             `json:"zip_file_key"`
}

type OpenModelResource struct {
	ResourceUUID string   `json:"resource_uuid"`
	Status       int      `json:"status"`
	Size         int64    `json:"resource_size"`
	FileNames    []string `json:"file_names"`
}

type OpenModelStartRequest struct {
	ResourceUUID           string `json:"resource_uuid"`
	Parameter2D            string `json:"parameter_2d,omitempty"`
	Parameter3D            string `json:"parameter_3d,omitempty"`
	Parameter3DGS          string `json:"parameter_3dgs,omitempty"`
	ParameterLidar         string `json:"parameter_lidar,omitempty"`
	DeleteResourceIfFinish bool   `json:"delete_resource_if_finish"`
}

type OpenModelTaskResult struct {
	UUID         string          `json:"uuid"`
	Status       OpenModelStatus `json:"status"`
	SubErrorCode int             `json:"sub_error_code"`
}

type OpenModelStartResult struct {
	Model2D    *OpenModelTaskResult `json:"model_2d,omitempty"`
	Model3D    *OpenModelTaskResult `json:"model_3d,omitempty"`
	Model3DGS  *OpenModelTaskResult `json:"model_3dgs,omitempty"`
	ModelLidar *OpenModelTaskResult `json:"model_lidar,omitempty"`
}

// OpenModelUploadCredential contains short-lived cloud credentials and an
// opaque callback parameter. It must only exist inside the upload workflow.
type OpenModelUploadCredential struct {
	CloudName       string    `json:"cloud_name"`
	AccessKeyID     string    `json:"access_key_id"`
	SecretAccessKey string    `json:"secret_access_key"`
	SessionToken    string    `json:"session_token"`
	Region          string    `json:"region"`
	BucketName      string    `json:"cloud_bucket_name"`
	CallbackParam   string    `json:"callback_param"`
	StorePath       string    `json:"store_path"`
	ExpireTimestamp int64     `json:"expire_time"`
	Endpoint        string    `json:"end_point"`
	ExpiresAt       time.Time `json:"-"`
}

type OpenModelUploadedFile struct {
	Name string `json:"name"`
	ETag string `json:"etag"`
}

type OpenModelUploadCallbackRequest struct {
	ResourceUUID string                  `json:"resource_uuid"`
	ResourceName string                  `json:"resource_name,omitempty"`
	Callback     string                  `json:"callback_param"`
	Files        []OpenModelUploadedFile `json:"files"`
}

type OpenModelUploadCallbackResult struct {
	ResourceUUID string   `json:"resource_uuid"`
	UploadCount  int      `json:"upload_count"`
	FileNames    []string `json:"file_name_list"`
}

func validModelString(value string, maximum int, allowEmpty bool) bool {
	if value != strings.TrimSpace(value) || len(value) > maximum || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	return allowEmpty || value != ""
}

func validModelFileType(value ModelFileType) bool {
	return value == ModelFile2D || value == ModelFile3D
}

func validModelSummary(item *ModelSummary) bool {
	return item != nil && item.ID > 0 && validModelString(item.Name, 1024, false) &&
		validModelFileType(item.FileType) && item.Size >= 0 && item.CreatedAt > 0 && item.UpdatedAt >= item.CreatedAt
}

func validateModelReconstruction(input *ModelReconstructionRequest) error {
	if input == nil || !validModelString(input.Name, 200, false) || len(input.ReconstructionTypes) < 1 || len(input.ReconstructionTypes) > 2 ||
		input.SimplifiedFactor <= 0 || input.SimplifiedFactor > 1 || input.TaskFolderID <= 0 ||
		!validModelString(input.WKT, maxModelParameterBytes, false) || !validEnum(input.QualityLevel, "high", "medium", "low") ||
		!validEnum(input.ReconstructionMode, "normal", "surround") || len(input.GenerateModelFormats) < 1 || len(input.GenerateModelFormats) > 8 {
		return &APIError{SafeCode: "request_invalid"}
	}
	seenTypes := map[ModelFileType]struct{}{}
	for _, value := range input.ReconstructionTypes {
		if !validModelFileType(value) {
			return &APIError{SafeCode: "request_invalid"}
		}
		if _, duplicate := seenTypes[value]; duplicate {
			return &APIError{SafeCode: "request_invalid"}
		}
		seenTypes[value] = struct{}{}
	}
	allowedFormats := map[string]struct{}{"b3dm": {}, "osgb": {}, "ply": {}, "obj": {}, "pnts": {}, "las": {}, "point_ply": {}, "normal_point_ply": {}}
	seenFormats := map[string]struct{}{}
	for _, value := range input.GenerateModelFormats {
		if _, ok := allowedFormats[value]; !ok {
			return &APIError{SafeCode: "request_invalid"}
		}
		if _, duplicate := seenFormats[value]; duplicate {
			return &APIError{SafeCode: "request_invalid"}
		}
		seenFormats[value] = struct{}{}
	}
	if input.PredefineArea != nil {
		points := input.PredefineArea.PolygonPoints
		if len(points) < 3 || len(points) > 1000 {
			return &APIError{SafeCode: "request_invalid"}
		}
		for _, point := range points {
			if point.Longitude < -180 || point.Longitude > 180 || point.Latitude < -90 || point.Latitude > 90 {
				return &APIError{SafeCode: "request_invalid"}
			}
		}
	}
	return nil
}

func (client *Client) CreateModelReconstruction(ctx context.Context, token, projectUUID string, input ModelReconstructionRequest) (ModelReconstructionResult, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return ModelReconstructionResult{}, err
	}
	if err := validateModelReconstruction(&input); err != nil {
		return ModelReconstructionResult{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodPost, Path: "/openapi/v2.0/model/create", Body: input, DisableRetry: true,
	})
	if err != nil {
		return ModelReconstructionResult{}, err
	}
	var result ModelReconstructionResult
	if json.Unmarshal(payload.Data, &result) != nil || result.ID <= 0 {
		return ModelReconstructionResult{}, schemaError()
	}
	return result, nil
}

func validOpenModelType(value OpenModelType) bool {
	return value >= OpenModel2D && value <= OpenModelLidar
}

func validOpenModelStatus(value OpenModelStatus) bool {
	switch value {
	case OpenModelReconstructionFailed, OpenModelMapReconstructionSucceeded, OpenModelRequestingResource,
		OpenModelRequestingResourceFailed, OpenModelReconstructionExecuting, OpenModelReconstructionSucceeded,
		OpenModelReconstructionCanceled:
		return true
	default:
		return false
	}
}

func validOpenModelZipStatus(value OpenModelZipStatus) bool {
	return value >= OpenModelZipInitial && value <= OpenModelZipFailed
}

func validProgress(value int) bool { return value >= 0 && value <= 100 }

func validOpenModel(item *OpenModel, details bool) bool {
	if item == nil || !validModelString(item.ResourceUUID, 256, false) ||
		!validModelString(item.ModelUUID, 256, false) || !validOpenModelType(item.ModelType) ||
		!validOpenModelStatus(item.ModelStatus) || item.ModelSize < 0 || !validProgress(item.ReconstructionProgress) ||
		!validOpenModelZipStatus(item.ZipStatus) || !validProgress(item.ZipProgress) || item.ErrorCode < 0 {
		return false
	}
	return !details || validModelString(item.ZipFileKey, maxModelStringBytes, true)
}

func modelPath(template, name, value string) (string, error) {
	value, err := requireScope(value)
	if err != nil {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return resolvePathTemplate(template, map[string]string{name: value})
}

func (client *Client) ListModels(ctx context.Context, token, projectUUID string) ([]ModelSummary, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/model"})
	if err != nil {
		return nil, err
	}
	return decodeList(payload, false, validModelSummary)
}

func (client *Client) GetModel(ctx context.Context, token, projectUUID, modelID string) (ModelDetail, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return ModelDetail{}, err
	}
	path, err := modelPath("/openapi/v2.0/model/{model_id}", "model_id", modelID)
	if err != nil {
		return ModelDetail{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return ModelDetail{}, err
	}
	var result ModelDetail
	if json.Unmarshal(payload.Data, &result) != nil || !validModelSummary(&ModelSummary{
		ID: result.ID, Name: result.Name, FileType: result.FileType, ShowOnMap: result.ShowOnMap,
		Size: result.Size, UpdatedAt: result.UpdatedAt, CreatedAt: result.CreatedAt,
	}) || !validModelString(result.UserName, 1024, true) {
		return ModelDetail{}, schemaError()
	}
	for _, raw := range []string{result.URL, result.PreviewURL} {
		if raw != "" {
			if _, err := client.validateResponseLink(LinkModel, raw); err != nil {
				return ModelDetail{}, err
			}
		}
	}
	return result, nil
}

func (client *Client) GetModelDownloadURL(ctx context.Context, token, projectUUID, fileID string) (ModelDownload, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return ModelDownload{}, err
	}
	path, err := modelPath("/openapi/v2.0/model/download-url/{file_id}", "file_id", fileID)
	if err != nil {
		return ModelDownload{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return ModelDownload{}, err
	}
	var raw struct {
		URL string `json:"url"`
		ID  int64  `json:"id"`
	}
	if json.Unmarshal(payload.Data, &raw) != nil || raw.ID <= 0 || !validModelString(raw.URL, maxModelCredentialBytes, true) {
		return ModelDownload{}, schemaError()
	}
	if raw.URL == "" {
		return ModelDownload{ID: raw.ID}, nil
	}
	parsed, err := client.validateResponseLink(LinkModel, raw.URL)
	if err != nil {
		return ModelDownload{}, err
	}
	expiresAt := temporaryLinkExpiry(parsed)
	if _, err := client.ValidateTemporaryLink(LinkModel, raw.URL, expiresAt); err != nil {
		return ModelDownload{}, err
	}
	return ModelDownload{ID: raw.ID, URL: raw.URL, ExpiresAt: expiresAt, Ready: true}, nil
}

func (client *Client) ListRunningOpenModels(ctx context.Context, token, projectUUID string) ([]OpenModel, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/open_model/models/running"})
	if err != nil {
		return nil, err
	}
	return decodeList(payload, false, func(item *OpenModel) bool { return validOpenModel(item, false) })
}

func (client *Client) GetOpenModel(ctx context.Context, token, projectUUID, modelUUID string) (OpenModel, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return OpenModel{}, err
	}
	path, err := modelPath("/openapi/v2.0/open_model/models/{model_uuid}", "model_uuid", modelUUID)
	if err != nil {
		return OpenModel{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return OpenModel{}, err
	}
	var result OpenModel
	if json.Unmarshal(payload.Data, &result) != nil || !validOpenModel(&result, true) {
		return OpenModel{}, schemaError()
	}
	return result, nil
}

func (client *Client) GetOpenModelResource(ctx context.Context, token, projectUUID, resourceUUID string) (OpenModelResource, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return OpenModelResource{}, err
	}
	path, err := modelPath("/openapi/v2.0/open_model/resource/{resource_uuid}", "resource_uuid", resourceUUID)
	if err != nil {
		return OpenModelResource{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return OpenModelResource{}, err
	}
	var result OpenModelResource
	if json.Unmarshal(payload.Data, &result) != nil || !validModelString(result.ResourceUUID, 256, false) ||
		result.Status < 1 || result.Status > 3 || result.Size < 0 || len(result.FileNames) > 20000 {
		return OpenModelResource{}, schemaError()
	}
	for _, name := range result.FileNames {
		if !validModelFileName(name) {
			return OpenModelResource{}, schemaError()
		}
	}
	return result, nil
}

func validModelParameter(value string) bool {
	if !validModelString(value, maxModelParameterBytes, false) {
		return false
	}
	var document map[string]any
	return json.Unmarshal([]byte(value), &document) == nil && document != nil
}

func validateOpenModelStart(input *OpenModelStartRequest) error {
	if input == nil || !validModelString(input.ResourceUUID, 256, false) {
		return &APIError{SafeCode: "request_invalid"}
	}
	parameters := []*string{&input.Parameter2D, &input.Parameter3D, &input.Parameter3DGS, &input.ParameterLidar}
	selected := 0
	for _, parameter := range parameters {
		if *parameter == "" {
			continue
		}
		if !validModelParameter(*parameter) {
			return &APIError{SafeCode: "request_invalid"}
		}
		selected++
	}
	if selected == 0 {
		return &APIError{SafeCode: "request_invalid"}
	}
	return nil
}

func validOpenModelTask(item *OpenModelTaskResult) bool {
	return item != nil && validModelString(item.UUID, 256, false) && validOpenModelStatus(item.Status) && item.SubErrorCode >= 0
}

func validateOpenModelStartResult(result *OpenModelStartResult, input OpenModelStartRequest) bool {
	if result == nil {
		return false
	}
	checks := []struct {
		requested bool
		result    *OpenModelTaskResult
	}{
		{input.Parameter2D != "", result.Model2D},
		{input.Parameter3D != "", result.Model3D},
		{input.Parameter3DGS != "", result.Model3DGS},
		{input.ParameterLidar != "", result.ModelLidar},
	}
	for _, check := range checks {
		if check.requested != (check.result != nil) || (check.result != nil && !validOpenModelTask(check.result)) {
			return false
		}
	}
	return true
}

func (client *Client) StartOpenModelReconstruction(ctx context.Context, token, projectUUID string, input OpenModelStartRequest) (OpenModelStartResult, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return OpenModelStartResult{}, err
	}
	if err := validateOpenModelStart(&input); err != nil {
		return OpenModelStartResult{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodPost, Path: "/openapi/v2.0/open_model/models/reconstruction/start",
		Body: input, DisableRetry: true,
	})
	if err != nil {
		return OpenModelStartResult{}, err
	}
	var result OpenModelStartResult
	if json.Unmarshal(payload.Data, &result) != nil || !validateOpenModelStartResult(&result, input) {
		return OpenModelStartResult{}, schemaError()
	}
	return result, nil
}

func (client *Client) StopOpenModelReconstruction(ctx context.Context, token, projectUUID, modelUUID string) error {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return err
	}
	path, err := modelPath("/openapi/v2.0/open_model/models/{model_uuid}/reconstruction/stop", "model_uuid", modelUUID)
	if err != nil {
		return err
	}
	_, err = client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodPost, Path: path, DataOptional: true, DisableRetry: true,
	})
	return err
}

func (client *Client) DeleteOpenModel(ctx context.Context, token, projectUUID, modelUUID string) error {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return err
	}
	path, err := modelPath("/openapi/v2.0/open_model/models/{model_uuid}", "model_uuid", modelUUID)
	if err != nil {
		return err
	}
	_, err = client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodDelete, Path: path, DataOptional: true, DisableRetry: true,
	})
	return err
}

func (client *Client) DeleteOpenModelResource(ctx context.Context, token, projectUUID, resourceUUID string) error {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return err
	}
	path, err := modelPath("/openapi/v2.0/open_model/resource/{resource_uuid}", "resource_uuid", resourceUUID)
	if err != nil {
		return err
	}
	_, err = client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodDelete, Path: path, DataOptional: true, DisableRetry: true,
	})
	return err
}

func (client *Client) ObtainOpenModelUploadCredential(ctx context.Context, token, projectUUID string) (OpenModelUploadCredential, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return OpenModelUploadCredential{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodPost, Path: "/openapi/v2.0/open_model/stores/obtain_token", DisableRetry: true,
	})
	if err != nil {
		return OpenModelUploadCredential{}, err
	}
	var result OpenModelUploadCredential
	if json.Unmarshal(payload.Data, &result) != nil || !validModelString(result.CloudName, 64, false) ||
		!validModelString(result.AccessKeyID, maxModelCredentialBytes, false) ||
		!validModelString(result.SecretAccessKey, maxModelCredentialBytes, false) ||
		!validModelString(result.SessionToken, maxModelCredentialBytes, false) ||
		!validModelString(result.Region, 128, false) || !validModelString(result.BucketName, 256, false) ||
		!validModelString(result.CallbackParam, maxModelCredentialBytes, false) ||
		!validModelString(result.StorePath, maxModelStringBytes, false) || !strings.Contains(result.StorePath, "{fileName}") ||
		!validModelString(result.Endpoint, maxModelStringBytes, false) || result.ExpireTimestamp <= 0 {
		return OpenModelUploadCredential{}, schemaError()
	}
	result.ExpiresAt = time.Unix(result.ExpireTimestamp, 0).UTC()
	if _, err := client.ValidateTemporaryLink(LinkUpload, result.Endpoint, result.ExpiresAt); err != nil {
		return OpenModelUploadCredential{}, err
	}
	return result, nil
}

func validModelFileName(value string) bool {
	return validModelString(value, 1024, false) && !strings.ContainsAny(value, "/\\") && value != "." && value != ".."
}

func validModelETag(value string) bool {
	return validModelString(value, 512, false) && !strings.ContainsAny(value, " \t")
}

func validateOpenModelUploadCallback(input *OpenModelUploadCallbackRequest) error {
	if input == nil || !validModelString(input.ResourceUUID, 256, false) ||
		!validModelString(input.ResourceName, 255, true) || !validModelString(input.Callback, maxModelCredentialBytes, false) ||
		len(input.Files) < 1 || len(input.Files) > maxOpenModelFiles {
		return &APIError{SafeCode: "request_invalid"}
	}
	seen := make(map[string]struct{}, len(input.Files))
	for _, file := range input.Files {
		if !validModelFileName(file.Name) || !validModelETag(file.ETag) {
			return &APIError{SafeCode: "request_invalid"}
		}
		if _, duplicate := seen[file.Name]; duplicate {
			return &APIError{SafeCode: "request_invalid"}
		}
		seen[file.Name] = struct{}{}
	}
	return nil
}

func (client *Client) NotifyOpenModelUploadComplete(ctx context.Context, token, projectUUID string, input OpenModelUploadCallbackRequest) (OpenModelUploadCallbackResult, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return OpenModelUploadCallbackResult{}, err
	}
	if err := validateOpenModelUploadCallback(&input); err != nil {
		return OpenModelUploadCallbackResult{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodPost, Path: "/openapi/v2.0/open_model/stores/upload_callback",
		Body: input, DisableRetry: true,
	})
	if err != nil {
		return OpenModelUploadCallbackResult{}, err
	}
	var result OpenModelUploadCallbackResult
	if json.Unmarshal(payload.Data, &result) != nil || !validModelString(result.ResourceUUID, 256, false) ||
		result.UploadCount < 0 || result.UploadCount != len(result.FileNames) || result.UploadCount > maxOpenModelFiles {
		return OpenModelUploadCallbackResult{}, schemaError()
	}
	seen := make(map[string]struct{}, len(result.FileNames))
	for _, name := range result.FileNames {
		if !validModelFileName(name) {
			return OpenModelUploadCallbackResult{}, schemaError()
		}
		if _, duplicate := seen[name]; duplicate {
			return OpenModelUploadCallbackResult{}, schemaError()
		}
		seen[name] = struct{}{}
	}
	return result, nil
}

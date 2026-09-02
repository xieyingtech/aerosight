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

const adminPageSize = 20

type SystemHealth struct {
	Healthy bool   `json:"healthy"`
	Status  string `json:"status,omitempty"`
}

type StorageSTSRequest struct {
	SpecifyPath string `json:"specify_path"`
	FileUUID    string `json:"file_uuid"`
}

type StorageSTSCredentials struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	ExpireSeconds   int    `json:"expire"`
	SecurityToken   string `json:"security_token"`
	Platform        int    `json:"platform"`
}

type StorageSTS struct {
	Endpoint        string                `json:"endpoint"`
	Provider        string                `json:"provider"`
	Region          string                `json:"region"`
	Bucket          string                `json:"bucket"`
	ObjectKeyPrefix string                `json:"object_key_prefix"`
	Credentials     StorageSTSCredentials `json:"credentials"`
	ExpiresAt       time.Time             `json:"-"`
}

type SNDecryptRequest struct {
	EncryptedSNs []string `json:"encrypted_sns"`
}

type SNDecryptResult struct {
	Mapping map[string]string `json:"mapping"`
}

type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

type PageResult[T any] struct {
	Pagination Pagination `json:"pagination"`
	List       []T        `json:"list"`
}

type PageOptions struct {
	Page     int
	PageSize int
}

type OrganizationListOptions struct {
	PageOptions
	Query      string
	Status     string
	UserRole   string
	SortColumn string
	SortType   string
}

type OrganizationDeviceBindCode struct {
	Code string `json:"code"`
}

type OrganizationIndustryType struct {
	Type    string `json:"type"`
	Subtype string `json:"sub_type"`
	Others  string `json:"others"`
}

type OrganizationUnitsSystem struct {
	Measure     string `json:"measure_units_system"`
	Temperature string `json:"temperature_units_system"`
}

type Organization struct {
	UUID                  string                     `json:"uuid"`
	Name                  string                     `json:"name"`
	OrganizationID        string                     `json:"organization_id"`
	CreatedAt             int64                      `json:"created_at"`
	UpdatedAt             int64                      `json:"updated_at"`
	Status                string                     `json:"status"`
	DeviceBindCode        OrganizationDeviceBindCode `json:"organization_device_bind_code"`
	IndustryType          OrganizationIndustryType   `json:"organization_industry_type"`
	UnitsSystem           OrganizationUnitsSystem    `json:"organization_units_system"`
	CurrentZoneID         string                     `json:"current_zone_id"`
	MFAEnabled            bool                       `json:"mfa_enabled"`
	LogoID                *string                    `json:"logo_id"`
	UserRole              string                     `json:"organization_user_role"`
	UserCallsign          string                     `json:"organization_user_callsign"`
	JoinedAt              int64                      `json:"join_organization_at"`
	ExperienceImprovement json.RawMessage            `json:"user_experience_improvement"`
	GeoCagingSystem       json.RawMessage            `json:"organization_geo_caging_system"`
}

type OrganizationUserProject struct {
	ProjectUUID     string `json:"project_uuid"`
	ProjectCallsign string `json:"project_callsign"`
}

type OrganizationUser struct {
	UserID        string                    `json:"user_id"`
	Account       string                    `json:"account"`
	AccountSecond string                    `json:"account_second"`
	Nickname      string                    `json:"nickname"`
	Role          string                    `json:"role"`
	CreatedAt     int64                     `json:"created_at"`
	SourceType    string                    `json:"source_type"`
	Projects      []OrganizationUserProject `json:"user_projects_infos"`
}

type OrganizationUserListOptions struct {
	PageOptions
	Query        string
	UserRole     string
	SourceType   string
	UserProjects string
	ProjectUUID  string
	SortColumn   string
	SortType     string
}

type CurrentOrganizationRole struct {
	Account          string          `json:"account"`
	UserID           string          `json:"user_id"`
	OrganizationUUID string          `json:"org_uuid"`
	Role             string          `json:"role"`
	Nickname         string          `json:"nickname"`
	OrganizationName string          `json:"organization_name"`
	MFAEnabled       bool            `json:"mfa_enabled"`
	UnitsConfig      json.RawMessage `json:"units_config"`
	GeoCagingConfig  json.RawMessage `json:"geo_caging_config"`
	AuthInfo         json.RawMessage `json:"auth_info"`
}

type OrganizationPermission struct {
	PermissionID          string                   `json:"permission_id"`
	PermissionName        string                   `json:"permission_name"`
	PermissionDescription string                   `json:"permission_description"`
	Level                 int                      `json:"level"`
	Visible               bool                     `json:"is_visible"`
	Basic                 bool                     `json:"is_basic_permission"`
	Path                  string                   `json:"path"`
	PermissionType        string                   `json:"permission_type"`
	Sequence              int                      `json:"sequence"`
	Predecessor           []string                 `json:"predecessor"`
	FrontendResources     []string                 `json:"frontend_resources"`
	Resources             []string                 `json:"resources"`
	ExtendedResources     []string                 `json:"extended_resources"`
	RelatedPermissions    []string                 `json:"related_permissions"`
	CreatedAt             string                   `json:"created_at"`
	UpdatedAt             string                   `json:"updated_at"`
	DeletedAt             *string                  `json:"deleted_at"`
	Children              []OrganizationPermission `json:"children"`
}

type OrganizationRole struct {
	RoleID          string                   `json:"role_id"`
	RoleName        string                   `json:"role_name"`
	RoleDescription string                   `json:"role_description"`
	WorkspaceID     string                   `json:"workspace_id"`
	Preset          bool                     `json:"is_preset"`
	AddToOrg        bool                     `json:"is_add_to_org"`
	RoleType        string                   `json:"role_type"`
	CreatedAt       string                   `json:"created_at"`
	UpdatedAt       string                   `json:"updated_at"`
	DeletedAt       *string                  `json:"deleted_at"`
	Permissions     []OrganizationPermission `json:"permissions"`
}

type ProjectUser struct {
	UserID                   string          `json:"user_id"`
	Account                  string          `json:"account"`
	Nickname                 string          `json:"nickname"`
	OrganizationUserCallsign string          `json:"organization_user_callsign"`
	ProjectCallsignUpdated   bool            `json:"is_user_project_callsign_updated"`
	Role                     string          `json:"role"`
	OrganizationRole         string          `json:"org_role"`
	RolesPermissions         json.RawMessage `json:"roles_permissions"`
	PhoneFilled              bool            `json:"is_phone_filled"`
	EmailFilled              bool            `json:"is_email_filled"`
}

type ProjectUserListOptions struct {
	PageOptions
	Query      string
	Role       string
	SortColumn string
	SortType   string
}

type OfflinePosition struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timestamp int64   `json:"timestamp"`
}

type ProjectMember struct {
	UserID               string           `json:"user_id"`
	Account              string           `json:"user_account"`
	ProjectCallsign      string           `json:"user_project_callsign"`
	ProjectRole          string           `json:"user_project_role"`
	OrganizationCallsign string           `json:"user_organization_callsign"`
	OrganizationRole     string           `json:"user_organization_role"`
	Online               bool             `json:"user_online_status"`
	PendingOffline       bool             `json:"is_user_pending_offline"`
	Platform             string           `json:"user_platform"`
	OfflinePosition      *OfflinePosition `json:"user_offline_position"`
	ControlDeviceSN      *string          `json:"user_control_device_sn"`
}

type JoinCodeQuery struct {
	ProjectID          string
	FastJoinCode       string
	AssociationDroneSN string
}

type JoinCodeInfo struct {
	ProjectUUID                       string  `json:"project_uuid"`
	ProjectID                         string  `json:"project_id"`
	ProjectName                       string  `json:"project_name"`
	OrganizationName                  string  `json:"organization_name"`
	OrganizationUUID                  string  `json:"organization_uuid"`
	OrganizationID                    string  `json:"organization_id"`
	UserInOrganization                bool    `json:"is_user_in_organization"`
	RecommendUserProjectCallsign      string  `json:"recommend_user_project_callsign"`
	RecommendAssociationDroneCallsign *string `json:"recommend_association_drone_project_callsign"`
}

func schemaError() error {
	return &APIError{SafeCode: "schema_incompatible", HTTPStatus: http.StatusOK}
}

func requireScope(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
		return "", &APIError{SafeCode: "scope_forbidden"}
	}
	return value, nil
}

func optionalQuery(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return value, nil
}

func pageQuery(options PageOptions) (url.Values, error) {
	page, size := options.Page, options.PageSize
	if page == 0 {
		page = 1
	}
	if size == 0 {
		size = adminPageSize
	}
	if page < 1 || page > 10000 || size < 1 || size > 100 {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	return url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(size)}}, nil
}

func addOptionalQuery(query url.Values, name, value string) error {
	value, err := optionalQuery(value)
	if err != nil {
		return err
	}
	if value != "" {
		query.Set(name, value)
	}
	return nil
}

func decodePage[T any](payload envelope, validate func(*T) bool) (PageResult[T], error) {
	var raw struct {
		Pagination *Pagination     `json:"pagination"`
		List       json.RawMessage `json:"list"`
	}
	if err := json.Unmarshal(payload.Data, &raw); err != nil || raw.Pagination == nil || raw.List == nil || string(raw.List) == "null" {
		return PageResult[T]{}, schemaError()
	}
	var list []T
	if err := json.Unmarshal(raw.List, &list); err != nil || list == nil || raw.Pagination.Page < 1 || raw.Pagination.PageSize < 1 || raw.Pagination.Total < 0 {
		return PageResult[T]{}, schemaError()
	}
	for index := range list {
		if !validate(&list[index]) {
			return PageResult[T]{}, schemaError()
		}
	}
	return PageResult[T]{Pagination: *raw.Pagination, List: list}, nil
}

func decodeList[T any](payload envelope, allowNull bool, validate func(*T) bool) ([]T, error) {
	var raw struct {
		List json.RawMessage `json:"list"`
	}
	if err := json.Unmarshal(payload.Data, &raw); err != nil || raw.List == nil {
		return nil, schemaError()
	}
	if string(raw.List) == "null" && allowNull {
		return []T{}, nil
	}
	var list []T
	if err := json.Unmarshal(raw.List, &list); err != nil || list == nil {
		return nil, schemaError()
	}
	for index := range list {
		if !validate(&list[index]) {
			return nil, schemaError()
		}
	}
	return list, nil
}

func (client *Client) CheckHealth(ctx context.Context, token string) (SystemHealth, error) {
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/health"})
	if err != nil {
		return SystemHealth{}, err
	}
	health := SystemHealth{Healthy: true}
	if string(payload.Data) == "null" {
		return health, nil
	}
	var data struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(payload.Data, &data); err != nil {
		return SystemHealth{}, schemaError()
	}
	health.Status = strings.TrimSpace(data.Status)
	return health, nil
}

func (client *Client) CheckSystemStatus(ctx context.Context, token, projectUUID string) (SystemHealth, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return SystemHealth{}, err
	}
	_, err = client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/system_status", DataOptional: true})
	if err != nil {
		return SystemHealth{}, err
	}
	return SystemHealth{Healthy: true}, nil
}

func (client *Client) CreateStorageSTS(ctx context.Context, token, projectUUID string, input StorageSTSRequest) (StorageSTS, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return StorageSTS{}, err
	}
	input.SpecifyPath = strings.TrimSpace(input.SpecifyPath)
	input.FileUUID = strings.TrimSpace(input.FileUUID)
	if len(input.SpecifyPath) > 1024 || strings.HasPrefix(input.SpecifyPath, "/") || strings.Contains(input.SpecifyPath, "..") || strings.ContainsAny(input.SpecifyPath, "\x00\r\n\\") || len(input.FileUUID) > 256 || strings.ContainsAny(input.FileUUID, "\x00\r\n") {
		return StorageSTS{}, &APIError{SafeCode: "request_invalid"}
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodPost, Path: "/openapi/v2.0/project/sts-token", Body: input})
	if err != nil {
		return StorageSTS{}, err
	}
	var result StorageSTS
	if err := json.Unmarshal(payload.Data, &result); err != nil {
		return StorageSTS{}, schemaError()
	}
	result.Provider = strings.TrimSpace(result.Provider)
	result.Endpoint = strings.TrimSpace(result.Endpoint)
	if (result.Provider != "ali" && result.Provider != "aws" && result.Provider != "minio") || result.Endpoint == "" || strings.TrimSpace(result.Region) == "" || strings.TrimSpace(result.Bucket) == "" || strings.TrimSpace(result.ObjectKeyPrefix) == "" || strings.TrimSpace(result.Credentials.AccessKeyID) == "" || strings.TrimSpace(result.Credentials.AccessKeySecret) == "" || strings.TrimSpace(result.Credentials.SecurityToken) == "" || result.Credentials.ExpireSeconds <= 0 || result.Credentials.ExpireSeconds > 86400 {
		return StorageSTS{}, schemaError()
	}
	result.ExpiresAt = client.now().Add(time.Duration(result.Credentials.ExpireSeconds) * time.Second)
	if _, err := client.ValidateTemporaryLink(LinkUpload, result.Endpoint, result.ExpiresAt); err != nil {
		return StorageSTS{}, err
	}
	return result, nil
}

func (client *Client) DecryptSNs(ctx context.Context, token, projectUUID string, input SNDecryptRequest) (SNDecryptResult, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return SNDecryptResult{}, err
	}
	if len(input.EncryptedSNs) < 1 || len(input.EncryptedSNs) > 100 {
		return SNDecryptResult{}, &APIError{SafeCode: "request_invalid"}
	}
	for _, value := range input.EncryptedSNs {
		if strings.TrimSpace(value) == "" || len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") {
			return SNDecryptResult{}, &APIError{SafeCode: "request_invalid"}
		}
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodPost, Path: "/openapi/v2.0/flight-task/sn-decrypt", Body: input, DisableRetry: true})
	if err != nil {
		return SNDecryptResult{}, err
	}
	var nested SNDecryptResult
	if err := json.Unmarshal(payload.Data, &nested); err != nil {
		return SNDecryptResult{}, schemaError()
	}
	if nested.Mapping == nil {
		if err := json.Unmarshal(payload.Data, &nested.Mapping); err != nil {
			return SNDecryptResult{}, schemaError()
		}
	}
	if len(nested.Mapping) == 0 || len(nested.Mapping) > len(input.EncryptedSNs) {
		return SNDecryptResult{}, schemaError()
	}
	for encrypted, plain := range nested.Mapping {
		if strings.TrimSpace(encrypted) == "" || strings.TrimSpace(plain) == "" {
			return SNDecryptResult{}, schemaError()
		}
	}
	return nested, nil
}

func (client *Client) ListOrganizations(ctx context.Context, token string, options OrganizationListOptions) (PageResult[Organization], error) {
	query, err := pageQuery(options.PageOptions)
	if err != nil {
		return PageResult[Organization]{}, err
	}
	for name, value := range map[string]string{"q": options.Query, "org_status": options.Status, "org_user_role": options.UserRole, "sort_column": options.SortColumn, "sort_type": options.SortType} {
		if err := addOptionalQuery(query, name, value); err != nil {
			return PageResult[Organization]{}, err
		}
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/organizations", Query: query})
	if err != nil {
		return PageResult[Organization]{}, err
	}
	return decodePage(payload, func(item *Organization) bool {
		item.UUID = strings.TrimSpace(item.UUID)
		item.Name = strings.TrimSpace(item.Name)
		return item.UUID != "" && item.Name != "" && strings.TrimSpace(item.OrganizationID) != ""
	})
}

func (client *Client) GetOrganization(ctx context.Context, token, organizationUUID string) (Organization, error) {
	organizationUUID, err := requireScope(organizationUUID)
	if err != nil {
		return Organization{}, err
	}
	path, err := resolvePathTemplate("/openapi/v2.0/organizations/{uuid}", map[string]string{"uuid": organizationUUID})
	if err != nil {
		return Organization{}, err
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return Organization{}, err
	}
	var item Organization
	if err := json.Unmarshal(payload.Data, &item); err != nil || strings.TrimSpace(item.UUID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.OrganizationID) == "" {
		return Organization{}, schemaError()
	}
	return item, nil
}

func (client *Client) ListOrganizationUsers(ctx context.Context, token, organizationUUID string, options OrganizationUserListOptions) (PageResult[OrganizationUser], error) {
	organizationUUID, err := requireScope(organizationUUID)
	if err != nil {
		return PageResult[OrganizationUser]{}, err
	}
	path, err := resolvePathTemplate("/openapi/v2.0/organizations/{uuid}/users", map[string]string{"uuid": organizationUUID})
	if err != nil {
		return PageResult[OrganizationUser]{}, err
	}
	query, err := pageQuery(options.PageOptions)
	if err != nil {
		return PageResult[OrganizationUser]{}, err
	}
	for name, value := range map[string]string{"q": options.Query, "user_role": options.UserRole, "source_type": options.SourceType, "user_projects": options.UserProjects, "project_uuid": options.ProjectUUID, "sort_column": options.SortColumn, "sort_type": options.SortType} {
		if err := addOptionalQuery(query, name, value); err != nil {
			return PageResult[OrganizationUser]{}, err
		}
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path, Query: query})
	if err != nil {
		return PageResult[OrganizationUser]{}, err
	}
	return decodePage(payload, func(item *OrganizationUser) bool {
		return strings.TrimSpace(item.UserID) != "" && strings.TrimSpace(item.Role) != ""
	})
}

func (client *Client) GetCurrentOrganizationRole(ctx context.Context, token, organizationUUID string) (CurrentOrganizationRole, error) {
	organizationUUID, err := requireScope(organizationUUID)
	if err != nil {
		return CurrentOrganizationRole{}, err
	}
	path, err := resolvePathTemplate("/openapi/v2.0/organizations/{uuid}/users/current", map[string]string{"uuid": organizationUUID})
	if err != nil {
		return CurrentOrganizationRole{}, err
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return CurrentOrganizationRole{}, err
	}
	var item CurrentOrganizationRole
	if err := json.Unmarshal(payload.Data, &item); err != nil || strings.TrimSpace(item.UserID) == "" || strings.TrimSpace(item.OrganizationUUID) == "" || strings.TrimSpace(item.Role) == "" {
		return CurrentOrganizationRole{}, schemaError()
	}
	return item, nil
}

func (client *Client) ListOrganizationRoles(ctx context.Context, token, organizationUUID, roleType string, options PageOptions) (PageResult[OrganizationRole], error) {
	organizationUUID, err := requireScope(organizationUUID)
	if err != nil {
		return PageResult[OrganizationRole]{}, err
	}
	roleType, err = requireScope(roleType)
	if err != nil {
		return PageResult[OrganizationRole]{}, &APIError{SafeCode: "request_invalid"}
	}
	path, err := resolvePathTemplate("/openapi/v2.0/organizations/{uuid}/roles", map[string]string{"uuid": organizationUUID})
	if err != nil {
		return PageResult[OrganizationRole]{}, err
	}
	query, err := pageQuery(options)
	if err != nil {
		return PageResult[OrganizationRole]{}, err
	}
	query.Set("type", roleType)
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path, Query: query})
	if err != nil {
		return PageResult[OrganizationRole]{}, err
	}
	return decodePage(payload, func(item *OrganizationRole) bool {
		return strings.TrimSpace(item.RoleID) != "" && strings.TrimSpace(item.RoleName) != "" && validPermissions(item.Permissions, 0)
	})
}

func validPermissions(items []OrganizationPermission, depth int) bool {
	if depth > 16 || len(items) > 10000 {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item.PermissionID) == "" || strings.TrimSpace(item.PermissionName) == "" || !validPermissions(item.Children, depth+1) {
			return false
		}
	}
	return true
}

func (client *Client) ListOrganizationPermissions(ctx context.Context, token, organizationUUID, permissionType string) ([]OrganizationPermission, error) {
	return client.listPermissions(ctx, token, organizationUUID, permissionType, nil, "/openapi/v2.0/organizations/{uuid}/permissions")
}

func (client *Client) ListRolePermissions(ctx context.Context, token, organizationUUID, permissionType string, roleIDs []string) ([]OrganizationPermission, error) {
	return client.listPermissions(ctx, token, organizationUUID, permissionType, roleIDs, "/openapi/v2.0/organizations/{uuid}/roles/permissions")
}

func (client *Client) listPermissions(ctx context.Context, token, organizationUUID, permissionType string, roleIDs []string, template string) ([]OrganizationPermission, error) {
	organizationUUID, err := requireScope(organizationUUID)
	if err != nil {
		return nil, err
	}
	permissionType, err = requireScope(permissionType)
	if err != nil {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	path, err := resolvePathTemplate(template, map[string]string{"uuid": organizationUUID})
	if err != nil {
		return nil, err
	}
	query := url.Values{"type": {permissionType}}
	for _, roleID := range roleIDs {
		roleID, err = requireScope(roleID)
		if err != nil {
			return nil, &APIError{SafeCode: "request_invalid"}
		}
		query.Add("role_id", roleID)
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path, Query: query})
	if err != nil {
		return nil, err
	}
	var data struct {
		Permissions []OrganizationPermission `json:"init_permissions"`
	}
	if err := json.Unmarshal(payload.Data, &data); err != nil || data.Permissions == nil || !validPermissions(data.Permissions, 0) {
		return nil, schemaError()
	}
	return data.Permissions, nil
}

func (client *Client) ListProjectUsers(ctx context.Context, token, projectUUID string, options ProjectUserListOptions) (PageResult[ProjectUser], error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return PageResult[ProjectUser]{}, err
	}
	query, err := pageQuery(options.PageOptions)
	if err != nil {
		return PageResult[ProjectUser]{}, err
	}
	for name, value := range map[string]string{"q": options.Query, "prj_user_role": options.Role, "sort_column": options.SortColumn, "sort_type": options.SortType} {
		if err := addOptionalQuery(query, name, value); err != nil {
			return PageResult[ProjectUser]{}, err
		}
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/users", Query: query})
	if err != nil {
		return PageResult[ProjectUser]{}, err
	}
	return decodePage(payload, func(item *ProjectUser) bool {
		return strings.TrimSpace(item.UserID) != "" && strings.TrimSpace(item.Role) != ""
	})
}

func (client *Client) ListProjectMembers(ctx context.Context, token, projectUUID string) ([]ProjectMember, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/members"})
	if err != nil {
		return nil, err
	}
	return decodeList(payload, true, func(item *ProjectMember) bool {
		return strings.TrimSpace(item.UserID) != "" && strings.TrimSpace(item.ProjectRole) != ""
	})
}

func (client *Client) GetJoinCodeInfo(ctx context.Context, token string, input JoinCodeQuery) (JoinCodeInfo, error) {
	projectID, err := requireScope(input.ProjectID)
	if err != nil {
		return JoinCodeInfo{}, &APIError{SafeCode: "request_invalid"}
	}
	joinCode, err := requireScope(input.FastJoinCode)
	if err != nil {
		return JoinCodeInfo{}, &APIError{SafeCode: "request_invalid"}
	}
	query := url.Values{"project_id": {projectID}, "project_fast_join_code": {joinCode}}
	if err := addOptionalQuery(query, "association_drone_device_sn", input.AssociationDroneSN); err != nil {
		return JoinCodeInfo{}, err
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/projects/join-codes", Query: query})
	if err != nil {
		return JoinCodeInfo{}, err
	}
	var item JoinCodeInfo
	if err := json.Unmarshal(payload.Data, &item); err != nil || strings.TrimSpace(item.ProjectUUID) == "" || strings.TrimSpace(item.ProjectID) == "" || strings.TrimSpace(item.ProjectName) == "" || strings.TrimSpace(item.OrganizationUUID) == "" || strings.TrimSpace(item.OrganizationID) == "" || strings.TrimSpace(item.OrganizationName) == "" {
		return JoinCodeInfo{}, schemaError()
	}
	return item, nil
}

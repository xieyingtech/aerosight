package flighthub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

const maxHMSDevices = 100

type DeviceDetail struct {
	SN                   string      `json:"device_sn"`
	Model                DeviceModel `json:"device_model"`
	OrganizationDevice   bool        `json:"is_org_device"`
	OrganizationCallsign string      `json:"device_organization_callsign"`
	ProjectCallsign      string      `json:"device_project_callsign"`
}

type DeviceStateSnapshot struct {
	SN    string                     `json:"device_sn"`
	Model DeviceModel                `json:"device_model"`
	State map[string]json.RawMessage `json:"device_state"`
}

type HMSAlert struct {
	Level                string          `json:"level"`
	Module               string          `json:"module"`
	InTheSky             string          `json:"in_the_sky"`
	Imminent             bool            `json:"imminent"`
	Code                 string          `json:"code"`
	Message              string          `json:"message"`
	Args                 json.RawMessage `json:"args"`
	ID                   string          `json:"hms_id"`
	Status               int             `json:"status"`
	StatusKey            string          `json:"status_key"`
	BeginTime            int64           `json:"begin_time"`
	EndTime              int64           `json:"end_time"`
	DeviceDataCreateTime int64           `json:"device_data_create_time"`
	DeviceDataUpdateTime int64           `json:"device_data_update_time"`
	DomainType           string          `json:"domain_type"`
	SubAlerts            []HMSAlert      `json:"sub_hms_list"`
}

type DeviceHMS struct {
	SN     string `json:"device_sn"`
	Alerts struct {
		List []HMSAlert `json:"list"`
	} `json:"device_hms"`
}

type HistoricalDevice struct {
	SN                         string           `json:"device_sn"`
	Model                      DeviceModel      `json:"device_model"`
	Online                     bool             `json:"device_online_status"`
	PendingOffline             bool             `json:"is_device_pending_offline"`
	OfflinePosition            *OfflinePosition `json:"device_offline_position"`
	ProjectCallsign            string           `json:"device_project_callsign"`
	OrganizationDevice         bool             `json:"is_organization_device"`
	OrganizationCallsign       *string          `json:"device_organization_callsign"`
	ControlUserID              string           `json:"device_control_user_id"`
	ControlUserProjectCallsign string           `json:"device_control_user_project_callsign"`
	ControlUserOrgCallsign     string           `json:"device_control_user_organization_callsign"`
	DeviceIndex                string           `json:"device_index"`
	ParentSNs                  []string         `json:"parent_sns"`
}

type HistoricalTopology struct {
	Index   string             `json:"index"`
	Host    *HistoricalDevice  `json:"host"`
	Parents []HistoricalDevice `json:"parents"`
	Nodes   json.RawMessage    `json:"nodes"`
	Project json.RawMessage    `json:"project"`
}

type AutoRecordingItem struct {
	SN                string `json:"sn"`
	CameraIndex       string `json:"camera_index"`
	CameraSN          string `json:"camera_sn"`
	RecordingStrategy int    `json:"recording_strategy"`
}

type AutoRecordingConfig struct {
	ID                           int64               `json:"id"`
	ProjectID                    string              `json:"project_id"`
	CreatedAt                    string              `json:"created_at"`
	UpdatedAt                    string              `json:"updated_at"`
	AutoCleaningDays             int                 `json:"auto_cleaning_time"`
	AutoCleaningDisabled         bool                `json:"disabled_auto_clean"`
	DockRecordingDisabled        bool                `json:"disable_dock_recording"`
	DroneRecordingDisabled       bool                `json:"disable_drone_recording"`
	BaseStationRecordingDisabled bool                `json:"disable_base_station_recording"`
	RTMPRecordingDisabled        bool                `json:"disable_rtmp_recording"`
	Items                        []AutoRecordingItem `json:"recording_items"`
}

func validateDeviceModel(model DeviceModel) bool {
	return strings.TrimSpace(model.Key) != "" && strings.TrimSpace(model.Class) != ""
}

func devicePath(template, parameter, serial string) (string, error) {
	serial, err := requireScope(serial)
	if err != nil {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return resolvePathTemplate(template, map[string]string{parameter: serial})
}

func (client *Client) GetDeviceDetail(ctx context.Context, token, projectUUID, serial string) (DeviceDetail, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return DeviceDetail{}, err
	}
	path, err := devicePath("/openapi/v2.0/project/device/{sn}", "sn", serial)
	if err != nil {
		return DeviceDetail{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return DeviceDetail{}, err
	}
	var detail DeviceDetail
	if err := json.Unmarshal(payload.Data, &detail); err != nil || strings.TrimSpace(detail.SN) == "" || !validateDeviceModel(detail.Model) {
		return DeviceDetail{}, schemaError()
	}
	return detail, nil
}

func (client *Client) GetDeviceState(ctx context.Context, token, projectUUID, serial string) (DeviceStateSnapshot, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return DeviceStateSnapshot{}, err
	}
	serial, err = requireScope(serial)
	if err != nil {
		return DeviceStateSnapshot{}, &APIError{SafeCode: "request_invalid"}
	}
	path, err := resolvePathTemplate("/openapi/v2.0/device/{device_sn}/state", map[string]string{"device_sn": serial})
	if err != nil {
		return DeviceStateSnapshot{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return DeviceStateSnapshot{}, err
	}
	var snapshot DeviceStateSnapshot
	if err := json.Unmarshal(payload.Data, &snapshot); err != nil || strings.TrimSpace(snapshot.SN) == "" || snapshot.SN != serial || !validateDeviceModel(snapshot.Model) || snapshot.State == nil {
		return DeviceStateSnapshot{}, schemaError()
	}
	return snapshot, nil
}

func validateHMSAlerts(alerts []HMSAlert, depth int) bool {
	if alerts == nil || depth > 8 || len(alerts) > 100 {
		return false
	}
	for _, alert := range alerts {
		if strings.TrimSpace(alert.ID) == "" || strings.TrimSpace(alert.Code) == "" || strings.TrimSpace(alert.Level) == "" || strings.TrimSpace(alert.Module) == "" || !validateHMSAlertsAllowEmpty(alert.SubAlerts, depth+1) {
			return false
		}
	}
	return true
}

func validateHMSAlertsAllowEmpty(alerts []HMSAlert, depth int) bool {
	if alerts == nil {
		return true
	}
	return validateHMSAlerts(alerts, depth)
}

func (client *Client) ListDeviceHMS(ctx context.Context, token, projectUUID string, serials []string) ([]DeviceHMS, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return nil, err
	}
	if len(serials) < 1 || len(serials) > maxHMSDevices {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	seen := make(map[string]struct{}, len(serials))
	for index, serial := range serials {
		serial, err = requireScope(serial)
		if err != nil || strings.Contains(serial, ",") {
			return nil, &APIError{SafeCode: "request_invalid"}
		}
		if _, duplicate := seen[serial]; duplicate {
			return nil, &APIError{SafeCode: "request_invalid"}
		}
		seen[serial] = struct{}{}
		serials[index] = serial
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/device/hms", Query: url.Values{"device_sn_list": {strings.Join(serials, ",")}}})
	if err != nil {
		return nil, err
	}
	return decodeList(payload, false, func(item *DeviceHMS) bool {
		return strings.TrimSpace(item.SN) != "" && validateHMSAlerts(item.Alerts.List, 0)
	})
}

func validateHistoricalDevice(device *HistoricalDevice) bool {
	return device != nil && strings.TrimSpace(device.SN) != "" && validateDeviceModel(device.Model)
}

func (client *Client) ListHistoricalTopologies(ctx context.Context, token, projectUUID string) ([]HistoricalTopology, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/topologies/history"})
	if err != nil {
		return nil, err
	}
	return decodeList(payload, false, func(item *HistoricalTopology) bool {
		if strings.TrimSpace(item.Index) == "" || (item.Host == nil && len(item.Parents) == 0) {
			return false
		}
		if item.Host != nil && !validateHistoricalDevice(item.Host) {
			return false
		}
		for index := range item.Parents {
			if !validateHistoricalDevice(&item.Parents[index]) {
				return false
			}
		}
		return true
	})
}

func (client *Client) GetAutoRecordingConfig(ctx context.Context, token, projectUUID string) (AutoRecordingConfig, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return AutoRecordingConfig{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/auto-record-configs"})
	if err != nil {
		return AutoRecordingConfig{}, err
	}
	var config AutoRecordingConfig
	if err := json.Unmarshal(payload.Data, &config); err != nil || config.ID <= 0 || strings.TrimSpace(config.ProjectID) == "" || config.Items == nil {
		return AutoRecordingConfig{}, schemaError()
	}
	for _, item := range config.Items {
		if strings.TrimSpace(item.SN) == "" || strings.TrimSpace(item.CameraIndex) == "" {
			return AutoRecordingConfig{}, schemaError()
		}
	}
	return config, nil
}

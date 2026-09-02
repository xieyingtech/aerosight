package flighthub

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"aerosight/worker/internal/driver"
)

const (
	maxControlActionInputBytes = 32 << 10
	maxControlListItems        = 1000
)

type ControlResultSemantics string

const (
	ControlResultAcceptanceOnly       ControlResultSemantics = "acceptance_only"
	ControlResultAcceptedWithID       ControlResultSemantics = "accepted_with_identifier"
	ControlResultSynchronousSnapshot  ControlResultSemantics = "synchronous_snapshot"
	ControlResultAsynchronousSnapshot ControlResultSemantics = "asynchronous_snapshot"
)

// ControlActionDefinition is the fail-closed contract between a canonical
// AeroSight action and one released FlightHub endpoint. InputSchema describes
// the canonical adapter input; OutputSchema describes the endpoint data field.
// Definitions do not execute requests and all mutations remain disabled until
// the later command-ledger and field-acceptance gates explicitly enable them.
type ControlActionDefinition struct {
	Code                 string
	EndpointID           string
	Method               string
	PathTemplate         string
	Scope                string
	Risk                 driver.RiskLevel
	DefaultEnabled       bool
	ResultSemantics      ControlResultSemantics
	FinalOnHTTPSuccess   bool
	SensitiveInputFields []string
	InputSchema          json.RawMessage
	OutputSchema         json.RawMessage
}

type ControlActionRequest struct {
	Definition ControlActionDefinition
	Spec       requestSpec
}

type controlActionAdapter struct {
	definition ControlActionDefinition
	build      func(json.RawMessage) (requestSpec, error)
	decode     func(json.RawMessage) (any, error)
}

type deviceCommandInput struct {
	DeviceSN string `json:"deviceSn"`
}

type organizationControlStatusInput struct {
	OrganizationUUID    string `json:"organizationUuid"`
	DeviceControlMethod string `json:"deviceControlMethod"`
	DeviceSN            string `json:"deviceSn,omitempty"`
}

type organizationCommandStatusInput struct {
	OrganizationUUID string   `json:"organizationUuid"`
	DeviceSNs        []string `json:"deviceSns"`
	Identifiers      []string `json:"identifiers,omitempty"`
}

type workspaceInput struct {
	WorkspaceID string `json:"workspaceId"`
}

type cloudControlStatusInput struct {
	DroneSNs []string `json:"droneSns"`
}

type lensChangeInput struct {
	SN          string `json:"sn"`
	CameraIndex string `json:"cameraIndex"`
	LensType    string `json:"lensType"`
}

type cameraChangeInput struct {
	SN             string `json:"sn"`
	CameraIndex    string `json:"cameraIndex"`
	CameraPosition string `json:"cameraPosition,omitempty"`
}

type rtkCalibrationInput struct {
	DeviceSN   string `json:"deviceSn"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Account    string `json:"account"`
	Password   string `json:"password"`
	MountPoint string `json:"mountPoint"`
}

type relayPairingInput struct {
	DeviceSN   string `json:"deviceSn"`
	PairEnable *bool  `json:"pairEnable"`
	PairType   string `json:"pairType"`
}

type activeProjectInput struct {
	ActiveProjectUUID string `json:"activeProjectUuid"`
	DeviceSN          string `json:"deviceSn"`
}

type projectControlStatusInput struct {
	DeviceControlMethod string `json:"deviceControlMethod"`
	DeviceSN            string `json:"deviceSn,omitempty"`
}

type controlOwnershipInput struct {
	DroneSN      string   `json:"droneSn"`
	Flight       bool     `json:"flight,omitempty"`
	PayloadIndex []string `json:"payloadIndex,omitempty"`
}

type deviceCommandBody struct {
	DeviceCommand string `json:"device_command"`
}

type lensChangeBody struct {
	SN          string `json:"sn"`
	CameraIndex string `json:"camera_index"`
	LensType    string `json:"lens_type"`
}

type cameraChangeBody struct {
	SN             string `json:"sn"`
	CameraIndex    string `json:"camera_index"`
	CameraPosition string `json:"camera_position,omitempty"`
}

type rtkCalibrationBody struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Account    string `json:"account"`
	Password   string `json:"password"`
	MountPoint string `json:"mount_point"`
}

type relayPairingBody struct {
	DeviceSN   string `json:"device_sn"`
	PairEnable bool   `json:"pair_enable"`
	PairType   string `json:"pair_type"`
}

type activeProjectBody struct {
	ActiveProjectUUID string `json:"active_project_uuid"`
	DeviceSN          string `json:"device_sn"`
}

type controlOwnershipBody struct {
	DroneSN      string   `json:"drone_sn"`
	Flight       bool     `json:"flight,omitempty"`
	PayloadIndex []string `json:"payload_index,omitempty"`
}

type EmptyControlOutput struct{}

type ControlStatusOutput struct {
	Status               int    `json:"device_control_status"`
	UserID               string `json:"device_control_user_id"`
	ProjectCallsign      string `json:"device_control_user_project_callsign"`
	OrganizationCallsign string `json:"device_control_user_organization_callsign"`
}

type ControlServiceProgress struct {
	BusinessID string `json:"bid"`
	CreateTime int64  `json:"create_time"`
	UpdateTime int64  `json:"update_time"`
	Progress   struct {
		Percent     int `json:"percent"`
		CurrentStep int `json:"current_step"`
	} `json:"progress"`
	DeviceCode int             `json:"device_code"`
	Extension  json.RawMessage `json:"ext"`
}

type DeviceCommandStatus struct {
	SN       string                            `json:"sn"`
	Services map[string]ControlServiceProgress `json:"services"`
}

type CommandStatusOutput struct {
	List []DeviceCommandStatus `json:"list"`
}

// OpenControlOutputSummary is used only where the released vendor schema is
// explicitly open. Raw fields are intentionally discarded and cannot expand
// runtime capabilities.
type OpenControlOutputSummary struct {
	Kind       string
	ItemCount  int
	FieldCount int
}

type RTKCalibrationOutput struct {
	BusinessID string `json:"bid"`
	Status     string `json:"status"`
}

type RelayPairingOutput struct {
	Status string `json:"status"`
	Output struct {
		Status int `json:"status"`
	} `json:"output"`
	BusinessID string `json:"bid"`
}

type ControlOwnershipParty struct {
	Callsign string `json:"call_sign"`
	UserID   string `json:"user_id"`
	Type     string `json:"type"`
}

type ControlOwnership struct {
	Type         string `json:"type"`
	PayloadIndex string `json:"payload_index,omitempty"`
	Gateway      struct {
		SN string `json:"sn"`
	} `json:"gateway"`
	User ControlOwnershipParty `json:"user"`
}

type ControlOwnershipOutput struct {
	DroneSN  string             `json:"drone_sn"`
	Controls []ControlOwnership `json:"controls"`
}

var controlActionAdapters = []controlActionAdapter{
	deviceCommandAdapter("return_home", "return_home"),
	deviceCommandAdapter("return_home_cancel", "return_home_cancel"),
	deviceCommandAdapter("flighttask_pause", "flighttask_pause"),
	deviceCommandAdapter("flighttask_recovery", "flighttask_recovery"),
	{
		definition: controlDefinition("control.status.organization", "454273409e0", http.MethodGet, "/openapi/v2.0/organizations/{uuid}/manage-devices/cmds/control/status", "organization", driver.RiskLow, ControlResultSynchronousSnapshot, true, organizationControlStatusInputSchema, controlStatusOutputSchema),
		build:      buildOrganizationControlStatus,
		decode:     decodeControlStatus,
	},
	{
		definition: controlDefinition("command.status.organization", "454273416e0", http.MethodGet, "/openapi/v2.0/organizations/{uuid}/manage-devices/cmds", "organization", driver.RiskLow, ControlResultAsynchronousSnapshot, true, organizationCommandStatusInputSchema, commandStatusOutputSchema),
		build:      buildOrganizationCommandStatus,
		decode:     decodeCommandStatus,
	},
	{
		definition: controlDefinition("tca.status", "454273421e0", http.MethodGet, "/openapi/v2.0/workspaces/{workspace_id}/groups/tcas", "workspace", driver.RiskLow, ControlResultSynchronousSnapshot, true, workspaceInputSchema, tcaOutputSchema),
		build:      buildTCAStatus,
		decode:     decodeOpenCollection,
	},
	{
		definition: controlDefinition("cloud_control.status", "456458604e0", http.MethodGet, "/openapi/v2.0/cloud-controls", "project", driver.RiskLow, ControlResultSynchronousSnapshot, true, cloudControlStatusInputSchema, cloudControlOutputSchema),
		build:      buildCloudControlStatus,
		decode:     decodeOpenObject,
	},
	{
		definition: controlDefinition("camera.change_lens", "456470750e0", http.MethodPost, "/openapi/v2.0/device/change-lens", "device", driver.RiskHigh, ControlResultAcceptanceOnly, false, lensChangeInputSchema, nullableEmptyControlOutputSchema),
		build:      buildLensChange,
		decode:     decodeNullableEmptyControlOutput,
	},
	{
		definition: controlDefinition("camera.change", "456471285e0", http.MethodPost, "/openapi/v2.0/device/change-camera", "device", driver.RiskHigh, ControlResultAcceptanceOnly, false, cameraChangeInputSchema, nullableEmptyControlOutputSchema),
		build:      buildCameraChange,
		decode:     decodeNullableEmptyControlOutput,
	},
	{
		definition: sensitiveControlDefinition("rtk.calibrate", "456680818e0", http.MethodPost, "/openapi/v2.0/device/{device_sn}/rtk", "device", driver.RiskCritical, ControlResultAcceptedWithID, false, []string{"password"}, rtkCalibrationInputSchema, rtkCalibrationOutputSchema),
		build:      buildRTKCalibration,
		decode:     decodeRTKCalibration,
	},
	{
		definition: controlDefinition("relay.pair", "456680819e0", http.MethodPost, "/openapi/v2.0/device/relay_model", "device", driver.RiskCritical, ControlResultAcceptanceOnly, false, relayPairingInputSchema, nullableEmptyControlOutputSchema),
		build:      buildRelayPairing,
		decode:     decodeNullableEmptyControlOutput,
	},
	{
		definition: controlDefinition("relay.status", "456680820e0", http.MethodGet, "/openapi/v2.0/device/{device_sn}/relay_model", "device", driver.RiskLow, ControlResultAsynchronousSnapshot, true, deviceCommandInputSchema, relayPairingOutputSchema),
		build:      buildRelayStatus,
		decode:     decodeRelayPairing,
	},
	{
		definition: controlDefinition("active_project.update", "456681372e0", http.MethodPut, "/openapi/v2.0/device/active-project", "device", driver.RiskCritical, ControlResultAcceptanceOnly, false, activeProjectInputSchema, nullableEmptyControlOutputSchema),
		build:      buildActiveProject,
		decode:     decodeNullableEmptyControlOutput,
	},
	{
		definition: controlDefinition("control.status.project", "457494961e0", http.MethodGet, "/openapi/v2.0/topologies/cmds/control/status", "project", driver.RiskLow, ControlResultSynchronousSnapshot, true, projectControlStatusInputSchema, controlStatusOutputSchema),
		build:      buildProjectControlStatus,
		decode:     decodeControlStatus,
	},
	{
		definition: controlDefinition("control.acquire", "458069497e0", http.MethodPost, "/openapi/v2.0/device/control", "device", driver.RiskCritical, ControlResultSynchronousSnapshot, false, controlOwnershipInputSchema, controlOwnershipOutputSchema),
		build:      func(raw json.RawMessage) (requestSpec, error) { return buildControlOwnership(http.MethodPost, raw) },
		decode:     decodeControlOwnership,
	},
	{
		definition: controlDefinition("control.release", "458069498e0", http.MethodDelete, "/openapi/v2.0/device/control", "device", driver.RiskCritical, ControlResultSynchronousSnapshot, false, controlOwnershipInputSchema, controlOwnershipOutputSchema),
		build:      func(raw json.RawMessage) (requestSpec, error) { return buildControlOwnership(http.MethodDelete, raw) },
		decode:     decodeControlOwnership,
	},
}

func controlDefinition(code, endpointID, method, path, scope string, risk driver.RiskLevel, semantics ControlResultSemantics, final bool, inputSchema, outputSchema string) ControlActionDefinition {
	return sensitiveControlDefinition(code, endpointID, method, path, scope, risk, semantics, final, nil, inputSchema, outputSchema)
}

func sensitiveControlDefinition(code, endpointID, method, path, scope string, risk driver.RiskLevel, semantics ControlResultSemantics, final bool, sensitiveFields []string, inputSchema, outputSchema string) ControlActionDefinition {
	return ControlActionDefinition{
		Code: code, EndpointID: endpointID, Method: method, PathTemplate: path, Scope: scope,
		Risk: risk, DefaultEnabled: false, ResultSemantics: semantics, FinalOnHTTPSuccess: final,
		SensitiveInputFields: append([]string(nil), sensitiveFields...),
		InputSchema:          json.RawMessage(inputSchema), OutputSchema: json.RawMessage(outputSchema),
	}
}

func deviceCommandAdapter(code, command string) controlActionAdapter {
	return controlActionAdapter{
		definition: controlDefinition(code, "454273417e0", http.MethodPost, "/openapi/v2.0/device/{device_sn}/command", "device", driver.RiskCritical, ControlResultAcceptanceOnly, false, deviceCommandInputSchema, emptyControlOutputSchema),
		build:      func(raw json.RawMessage) (requestSpec, error) { return buildDeviceCommand(command, raw) },
		decode:     decodeEmptyControlOutput,
	}
}

func ControlActionDefinitions() []ControlActionDefinition {
	definitions := make([]ControlActionDefinition, len(controlActionAdapters))
	for index, adapter := range controlActionAdapters {
		definition := adapter.definition
		definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
		definition.OutputSchema = append(json.RawMessage(nil), definition.OutputSchema...)
		definition.SensitiveInputFields = append([]string(nil), definition.SensitiveInputFields...)
		definitions[index] = definition
	}
	return definitions
}

func BuildControlActionRequest(code string, input json.RawMessage) (ControlActionRequest, error) {
	adapter, ok := findControlActionAdapter(code)
	if !ok {
		return ControlActionRequest{}, &APIError{SafeCode: "request_invalid"}
	}
	spec, err := adapter.build(input)
	if err != nil {
		return ControlActionRequest{}, err
	}
	if spec.Method != adapter.definition.Method {
		return ControlActionRequest{}, &APIError{SafeCode: "request_invalid"}
	}
	if err := validateRequestSpec(spec); err != nil {
		return ControlActionRequest{}, err
	}
	return ControlActionRequest{Definition: adapter.definition, Spec: spec}, nil
}

func DecodeControlActionOutput(code string, data json.RawMessage) (any, error) {
	adapter, ok := findControlActionAdapter(code)
	if !ok {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	return adapter.decode(data)
}

func findControlActionAdapter(code string) (controlActionAdapter, bool) {
	for _, adapter := range controlActionAdapters {
		if adapter.definition.Code == code {
			return adapter, true
		}
	}
	return controlActionAdapter{}, false
}

func decodeStrictControl(raw json.RawMessage, target any) error {
	if len(raw) == 0 || len(raw) > maxControlActionInputBytes {
		return &APIError{SafeCode: "request_invalid"}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &APIError{SafeCode: "request_invalid"}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return &APIError{SafeCode: "request_invalid"}
	}
	return nil
}

func controlString(value string, maximum int, optional bool) (string, error) {
	if value != strings.TrimSpace(value) || len(value) > maximum || strings.ContainsAny(value, "\x00\r\n") || (!optional && value == "") {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return value, nil
}

func controlIdentifier(value string, optional bool) (string, error) {
	value, err := controlString(value, 256, optional)
	if err != nil || strings.ContainsAny(value, "/\\?#&=,") {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return value, nil
}

func controlIdentifierList(values []string, minimum, maximum int) ([]string, error) {
	if len(values) < minimum || len(values) > maximum {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	result := make([]string, len(values))
	seen := map[string]struct{}{}
	for index, value := range values {
		var err error
		if result[index], err = controlIdentifier(value, false); err != nil {
			return nil, err
		}
		if _, duplicate := seen[result[index]]; duplicate {
			return nil, &APIError{SafeCode: "request_invalid"}
		}
		seen[result[index]] = struct{}{}
	}
	return result, nil
}

func buildDeviceCommand(command string, raw json.RawMessage) (requestSpec, error) {
	var input deviceCommandInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	deviceSN, err := controlIdentifier(input.DeviceSN, false)
	if err != nil {
		return requestSpec{}, err
	}
	path, err := resolvePathTemplate("/openapi/v2.0/device/{device_sn}/command", map[string]string{"device_sn": deviceSN})
	if err != nil {
		return requestSpec{}, err
	}
	return requestSpec{Method: http.MethodPost, Path: path, Body: deviceCommandBody{DeviceCommand: command}, DisableRetry: true, DataOptional: true}, nil
}

func buildOrganizationControlStatus(raw json.RawMessage) (requestSpec, error) {
	var input organizationControlStatusInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	organizationUUID, err := controlIdentifier(input.OrganizationUUID, false)
	if err != nil {
		return requestSpec{}, err
	}
	method, err := controlIdentifier(input.DeviceControlMethod, false)
	if err != nil {
		return requestSpec{}, err
	}
	deviceSN, err := controlIdentifier(input.DeviceSN, true)
	if err != nil {
		return requestSpec{}, err
	}
	path, err := resolvePathTemplate("/openapi/v2.0/organizations/{uuid}/manage-devices/cmds/control/status", map[string]string{"uuid": organizationUUID})
	if err != nil {
		return requestSpec{}, err
	}
	query := url.Values{"device_control_method": {method}}
	if deviceSN != "" {
		query.Set("device_sn", deviceSN)
	}
	return requestSpec{Method: http.MethodGet, Path: path, Query: query}, nil
}

func buildOrganizationCommandStatus(raw json.RawMessage) (requestSpec, error) {
	var input organizationCommandStatusInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	organizationUUID, err := controlIdentifier(input.OrganizationUUID, false)
	if err != nil {
		return requestSpec{}, err
	}
	serials, err := controlIdentifierList(input.DeviceSNs, 1, 50)
	if err != nil {
		return requestSpec{}, err
	}
	identifiers, err := controlIdentifierList(input.Identifiers, 0, 100)
	if err != nil {
		return requestSpec{}, err
	}
	path, err := resolvePathTemplate("/openapi/v2.0/organizations/{uuid}/manage-devices/cmds", map[string]string{"uuid": organizationUUID})
	if err != nil {
		return requestSpec{}, err
	}
	query := url.Values{"device_sn": serials}
	if len(identifiers) > 0 {
		query["identifiers"] = identifiers
	}
	return requestSpec{Method: http.MethodGet, Path: path, Query: query}, nil
}

func buildTCAStatus(raw json.RawMessage) (requestSpec, error) {
	var input workspaceInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	workspaceID, err := controlIdentifier(input.WorkspaceID, false)
	if err != nil {
		return requestSpec{}, err
	}
	path, err := resolvePathTemplate("/openapi/v2.0/workspaces/{workspace_id}/groups/tcas", map[string]string{"workspace_id": workspaceID})
	if err != nil {
		return requestSpec{}, err
	}
	return requestSpec{Method: http.MethodGet, Path: path, DataOptional: true}, nil
}

func buildCloudControlStatus(raw json.RawMessage) (requestSpec, error) {
	var input cloudControlStatusInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	serials, err := controlIdentifierList(input.DroneSNs, 1, 50)
	if err != nil {
		return requestSpec{}, err
	}
	return requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/cloud-controls", Query: url.Values{"drone_sn_list": serials}}, nil
}

func buildLensChange(raw json.RawMessage) (requestSpec, error) {
	var input lensChangeInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	values := []*string{&input.SN, &input.CameraIndex, &input.LensType}
	for _, value := range values {
		validated, err := controlIdentifier(*value, false)
		if err != nil {
			return requestSpec{}, err
		}
		*value = validated
	}
	return requestSpec{Method: http.MethodPost, Path: "/openapi/v2.0/device/change-lens", Body: lensChangeBody(input), DisableRetry: true, DataOptional: true}, nil
}

func buildCameraChange(raw json.RawMessage) (requestSpec, error) {
	var input cameraChangeInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	var err error
	if input.SN, err = controlIdentifier(input.SN, false); err != nil {
		return requestSpec{}, err
	}
	if input.CameraIndex, err = controlIdentifier(input.CameraIndex, false); err != nil {
		return requestSpec{}, err
	}
	if input.CameraPosition, err = controlIdentifier(input.CameraPosition, true); err != nil {
		return requestSpec{}, err
	}
	return requestSpec{Method: http.MethodPost, Path: "/openapi/v2.0/device/change-camera", Body: cameraChangeBody(input), DisableRetry: true, DataOptional: true}, nil
}

func buildRTKCalibration(raw json.RawMessage) (requestSpec, error) {
	var input rtkCalibrationInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	deviceSN, err := controlIdentifier(input.DeviceSN, false)
	if err != nil || input.Port < 1 || input.Port > 65535 {
		return requestSpec{}, &APIError{SafeCode: "request_invalid"}
	}
	if input.Host, err = controlString(input.Host, 253, false); err != nil || strings.ContainsAny(input.Host, "/\\?#@") {
		return requestSpec{}, &APIError{SafeCode: "request_invalid"}
	}
	for value, maximum := range map[*string]int{&input.Account: 256, &input.Password: 4096, &input.MountPoint: 256} {
		validated, validationErr := controlString(*value, maximum, false)
		if validationErr != nil {
			return requestSpec{}, validationErr
		}
		*value = validated
	}
	path, err := resolvePathTemplate("/openapi/v2.0/device/{device_sn}/rtk", map[string]string{"device_sn": deviceSN})
	if err != nil {
		return requestSpec{}, err
	}
	body := rtkCalibrationBody{Host: input.Host, Port: input.Port, Account: input.Account, Password: input.Password, MountPoint: input.MountPoint}
	return requestSpec{Method: http.MethodPost, Path: path, Body: body, DisableRetry: true}, nil
}

func buildRelayPairing(raw json.RawMessage) (requestSpec, error) {
	var input relayPairingInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	deviceSN, err := controlIdentifier(input.DeviceSN, false)
	if err != nil || input.PairEnable == nil || (input.PairType != "drone" && input.PairType != "relay") {
		return requestSpec{}, &APIError{SafeCode: "request_invalid"}
	}
	body := relayPairingBody{DeviceSN: deviceSN, PairEnable: *input.PairEnable, PairType: input.PairType}
	return requestSpec{Method: http.MethodPost, Path: "/openapi/v2.0/device/relay_model", Body: body, DisableRetry: true, DataOptional: true}, nil
}

func buildRelayStatus(raw json.RawMessage) (requestSpec, error) {
	var input deviceCommandInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	deviceSN, err := controlIdentifier(input.DeviceSN, false)
	if err != nil {
		return requestSpec{}, err
	}
	path, err := resolvePathTemplate("/openapi/v2.0/device/{device_sn}/relay_model", map[string]string{"device_sn": deviceSN})
	if err != nil {
		return requestSpec{}, err
	}
	return requestSpec{Method: http.MethodGet, Path: path}, nil
}

func buildActiveProject(raw json.RawMessage) (requestSpec, error) {
	var input activeProjectInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	activeProjectUUID, err := controlIdentifier(input.ActiveProjectUUID, false)
	if err != nil {
		return requestSpec{}, err
	}
	deviceSN, err := controlIdentifier(input.DeviceSN, false)
	if err != nil {
		return requestSpec{}, err
	}
	body := activeProjectBody{ActiveProjectUUID: activeProjectUUID, DeviceSN: deviceSN}
	return requestSpec{Method: http.MethodPut, Path: "/openapi/v2.0/device/active-project", Body: body, DisableRetry: true, DataOptional: true}, nil
}

func buildProjectControlStatus(raw json.RawMessage) (requestSpec, error) {
	var input projectControlStatusInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	method, err := controlIdentifier(input.DeviceControlMethod, false)
	if err != nil {
		return requestSpec{}, err
	}
	deviceSN, err := controlIdentifier(input.DeviceSN, true)
	if err != nil {
		return requestSpec{}, err
	}
	query := url.Values{"device_control_method": {method}}
	if deviceSN != "" {
		query.Set("device_sn", deviceSN)
	}
	return requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/topologies/cmds/control/status", Query: query}, nil
}

func buildControlOwnership(method string, raw json.RawMessage) (requestSpec, error) {
	var input controlOwnershipInput
	if err := decodeStrictControl(raw, &input); err != nil {
		return requestSpec{}, err
	}
	droneSN, err := controlIdentifier(input.DroneSN, false)
	if err != nil {
		return requestSpec{}, err
	}
	payloadIndex, err := controlIdentifierList(input.PayloadIndex, 0, 32)
	if err != nil || (!input.Flight && len(payloadIndex) == 0) {
		return requestSpec{}, &APIError{SafeCode: "request_invalid"}
	}
	body := controlOwnershipBody{DroneSN: droneSN, Flight: input.Flight, PayloadIndex: payloadIndex}
	return requestSpec{Method: method, Path: "/openapi/v2.0/device/control", Body: body, DisableRetry: true}, nil
}

func decodeOutputStrict(raw json.RawMessage, target any) error {
	if len(raw) == 0 || len(raw) > maxControlActionInputBytes {
		return schemaError()
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return schemaError()
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return schemaError()
	}
	return nil
}

func decodeEmptyControlOutput(raw json.RawMessage) (any, error) {
	var output map[string]json.RawMessage
	if err := decodeOutputStrict(raw, &output); err != nil || len(output) != 0 {
		return nil, schemaError()
	}
	return EmptyControlOutput{}, nil
}

func decodeNullableEmptyControlOutput(raw json.RawMessage) (any, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return EmptyControlOutput{}, nil
	}
	return decodeEmptyControlOutput(raw)
}

func decodeControlStatus(raw json.RawMessage) (any, error) {
	var output ControlStatusOutput
	if err := decodeOutputStrict(raw, &output); err != nil {
		return nil, err
	}
	for _, value := range []string{output.UserID, output.ProjectCallsign, output.OrganizationCallsign} {
		if _, err := controlString(value, 256, true); err != nil {
			return nil, schemaError()
		}
	}
	return output, nil
}

func decodeCommandStatus(raw json.RawMessage) (any, error) {
	var output CommandStatusOutput
	if err := decodeOutputStrict(raw, &output); err != nil || output.List == nil || len(output.List) > maxControlListItems {
		return nil, schemaError()
	}
	for _, item := range output.List {
		if _, err := controlIdentifier(item.SN, false); err != nil || item.Services == nil || len(item.Services) > 128 {
			return nil, schemaError()
		}
		for method, progress := range item.Services {
			if _, err := controlIdentifier(method, false); err != nil || progress.Progress.Percent < 0 || progress.Progress.Percent > 100 || len(progress.Extension) > maxControlActionInputBytes {
				return nil, schemaError()
			}
			if _, err := controlIdentifier(progress.BusinessID, false); err != nil || progress.CreateTime <= 0 || progress.UpdateTime < progress.CreateTime {
				return nil, schemaError()
			}
		}
	}
	return output, nil
}

func decodeOpenCollection(raw json.RawMessage) (any, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return OpenControlOutputSummary{Kind: "null"}, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || items == nil || len(items) > maxControlListItems {
		return nil, schemaError()
	}
	for _, item := range items {
		var object map[string]json.RawMessage
		if len(item) > maxControlActionInputBytes || json.Unmarshal(item, &object) != nil || object == nil {
			return nil, schemaError()
		}
	}
	return OpenControlOutputSummary{Kind: "array", ItemCount: len(items)}, nil
}

func decodeOpenObject(raw json.RawMessage) (any, error) {
	var object map[string]json.RawMessage
	if len(raw) == 0 || len(raw) > maxControlActionInputBytes || json.Unmarshal(raw, &object) != nil || object == nil || len(object) > 128 {
		return nil, schemaError()
	}
	return OpenControlOutputSummary{Kind: "object", FieldCount: len(object)}, nil
}

func decodeRTKCalibration(raw json.RawMessage) (any, error) {
	var output RTKCalibrationOutput
	if err := decodeOutputStrict(raw, &output); err != nil || output.BusinessID == "" || (output.Status != "ok" && output.Status != "failure") {
		return nil, schemaError()
	}
	if _, err := controlIdentifier(output.BusinessID, false); err != nil {
		return nil, schemaError()
	}
	return output, nil
}

func decodeRelayPairing(raw json.RawMessage) (any, error) {
	var output RelayPairingOutput
	if err := decodeOutputStrict(raw, &output); err != nil || (output.Status != "ok" && output.Status != "failed") {
		return nil, schemaError()
	}
	switch output.Output.Status {
	case 200, 1, 2, 3, 65535:
	default:
		return nil, schemaError()
	}
	if _, err := controlIdentifier(output.BusinessID, true); err != nil {
		return nil, schemaError()
	}
	return output, nil
}

func decodeControlOwnership(raw json.RawMessage) (any, error) {
	var output ControlOwnershipOutput
	if err := decodeOutputStrict(raw, &output); err != nil || len(output.Controls) > 64 {
		return nil, schemaError()
	}
	if _, err := controlIdentifier(output.DroneSN, false); err != nil {
		return nil, schemaError()
	}
	for _, control := range output.Controls {
		if control.Type != "flight" && control.Type != "payload" {
			return nil, schemaError()
		}
		for _, value := range []string{control.PayloadIndex, control.Gateway.SN, control.User.Callsign, control.User.UserID, control.User.Type} {
			if _, err := controlString(value, 256, true); err != nil {
				return nil, schemaError()
			}
		}
	}
	return output, nil
}

const (
	identifierPattern                    = `^[A-Za-z0-9._:-]{1,256}$`
	deviceCommandInputSchema             = `{"type":"object","additionalProperties":false,"required":["deviceSn"],"properties":{"deviceSn":{"type":"string","pattern":"` + identifierPattern + `"}}}`
	organizationControlStatusInputSchema = `{"type":"object","additionalProperties":false,"required":["organizationUuid","deviceControlMethod"],"properties":{"organizationUuid":{"type":"string","pattern":"` + identifierPattern + `"},"deviceControlMethod":{"type":"string","pattern":"` + identifierPattern + `"},"deviceSn":{"type":"string","pattern":"` + identifierPattern + `"}}}`
	organizationCommandStatusInputSchema = `{"type":"object","additionalProperties":false,"required":["organizationUuid","deviceSns"],"properties":{"organizationUuid":{"type":"string","pattern":"` + identifierPattern + `"},"deviceSns":{"type":"array","minItems":1,"maxItems":50,"uniqueItems":true,"items":{"type":"string","pattern":"` + identifierPattern + `"}},"identifiers":{"type":"array","maxItems":100,"uniqueItems":true,"items":{"type":"string","pattern":"` + identifierPattern + `"}}}}`
	workspaceInputSchema                 = `{"type":"object","additionalProperties":false,"required":["workspaceId"],"properties":{"workspaceId":{"type":"string","pattern":"` + identifierPattern + `"}}}`
	cloudControlStatusInputSchema        = `{"type":"object","additionalProperties":false,"required":["droneSns"],"properties":{"droneSns":{"type":"array","minItems":1,"maxItems":50,"uniqueItems":true,"items":{"type":"string","pattern":"` + identifierPattern + `"}}}}`
	lensChangeInputSchema                = `{"type":"object","additionalProperties":false,"required":["sn","cameraIndex","lensType"],"properties":{"sn":{"type":"string","pattern":"` + identifierPattern + `"},"cameraIndex":{"type":"string","pattern":"` + identifierPattern + `"},"lensType":{"type":"string","pattern":"` + identifierPattern + `"}}}`
	cameraChangeInputSchema              = `{"type":"object","additionalProperties":false,"required":["sn","cameraIndex"],"properties":{"sn":{"type":"string","pattern":"` + identifierPattern + `"},"cameraIndex":{"type":"string","pattern":"` + identifierPattern + `"},"cameraPosition":{"type":"string","pattern":"` + identifierPattern + `"}}}`
	rtkCalibrationInputSchema            = `{"type":"object","additionalProperties":false,"required":["deviceSn","host","port","account","password","mountPoint"],"properties":{"deviceSn":{"type":"string","pattern":"` + identifierPattern + `"},"host":{"type":"string","minLength":1,"maxLength":253},"port":{"type":"integer","minimum":1,"maximum":65535},"account":{"type":"string","minLength":1,"maxLength":256},"password":{"type":"string","minLength":1,"maxLength":4096,"writeOnly":true},"mountPoint":{"type":"string","minLength":1,"maxLength":256}}}`
	relayPairingInputSchema              = `{"type":"object","additionalProperties":false,"required":["deviceSn","pairEnable","pairType"],"properties":{"deviceSn":{"type":"string","pattern":"` + identifierPattern + `"},"pairEnable":{"type":"boolean"},"pairType":{"type":"string","enum":["drone","relay"]}}}`
	activeProjectInputSchema             = `{"type":"object","additionalProperties":false,"required":["activeProjectUuid","deviceSn"],"properties":{"activeProjectUuid":{"type":"string","pattern":"` + identifierPattern + `"},"deviceSn":{"type":"string","pattern":"` + identifierPattern + `"}}}`
	projectControlStatusInputSchema      = `{"type":"object","additionalProperties":false,"required":["deviceControlMethod"],"properties":{"deviceControlMethod":{"type":"string","pattern":"` + identifierPattern + `"},"deviceSn":{"type":"string","pattern":"` + identifierPattern + `"}}}`
	controlOwnershipInputSchema          = `{"type":"object","additionalProperties":false,"required":["droneSn"],"anyOf":[{"required":["flight"],"properties":{"flight":{"const":true}}},{"required":["payloadIndex"],"properties":{"payloadIndex":{"minItems":1}}}],"properties":{"droneSn":{"type":"string","pattern":"` + identifierPattern + `"},"flight":{"type":"boolean"},"payloadIndex":{"type":"array","maxItems":32,"uniqueItems":true,"items":{"type":"string","pattern":"` + identifierPattern + `"}}}}`

	emptyControlOutputSchema         = `{"type":"object","additionalProperties":false,"maxProperties":0}`
	nullableEmptyControlOutputSchema = `{"oneOf":[{"type":"null"},{"type":"object","additionalProperties":false,"maxProperties":0}]}`
	controlStatusOutputSchema        = `{"type":"object","additionalProperties":false,"required":["device_control_status","device_control_user_id","device_control_user_project_callsign","device_control_user_organization_callsign"],"properties":{"device_control_status":{"type":"integer"},"device_control_user_id":{"type":"string","maxLength":256},"device_control_user_project_callsign":{"type":"string","maxLength":256},"device_control_user_organization_callsign":{"type":"string","maxLength":256}}}`
	commandStatusOutputSchema        = `{"type":"object","additionalProperties":false,"required":["list"],"properties":{"list":{"type":"array","maxItems":1000,"items":{"type":"object","additionalProperties":false,"required":["sn","services"],"properties":{"sn":{"type":"string","pattern":"` + identifierPattern + `"},"services":{"type":"object","maxProperties":128,"additionalProperties":{"type":"object","additionalProperties":false,"required":["bid","create_time","update_time","progress","device_code","ext"],"properties":{"bid":{"type":"string"},"create_time":{"type":"integer"},"update_time":{"type":"integer"},"progress":{"type":"object","additionalProperties":false,"required":["percent","current_step"],"properties":{"percent":{"type":"integer","minimum":0,"maximum":100},"current_step":{"type":"integer"}}},"device_code":{"type":"integer"},"ext":{}}}}}}}}}`
	tcaOutputSchema                  = `{"type":["array","null"],"maxItems":1000,"items":{"type":"object"},"x-vendor-schema-open":true}`
	cloudControlOutputSchema         = `{"type":"object","maxProperties":128,"x-vendor-schema-open":true}`
	rtkCalibrationOutputSchema       = `{"type":"object","additionalProperties":false,"required":["bid","status"],"properties":{"bid":{"type":"string","pattern":"` + identifierPattern + `"},"status":{"type":"string","enum":["ok","failure"]}}}`
	relayPairingOutputSchema         = `{"type":"object","additionalProperties":false,"required":["status","output","bid"],"properties":{"status":{"type":"string","enum":["ok","failed"]},"output":{"type":"object","additionalProperties":false,"required":["status"],"properties":{"status":{"type":"integer","enum":[200,1,2,3,65535]}}},"bid":{"type":"string","maxLength":256}}}`
	controlOwnershipOutputSchema     = `{"type":"object","additionalProperties":false,"required":["drone_sn","controls"],"properties":{"drone_sn":{"type":"string","pattern":"` + identifierPattern + `"},"controls":{"type":"array","maxItems":64,"items":{"type":"object","additionalProperties":false,"required":["type","gateway","user"],"properties":{"type":{"type":"string","enum":["flight","payload"]},"payload_index":{"type":"string","maxLength":256},"gateway":{"type":"object","additionalProperties":false,"required":["sn"],"properties":{"sn":{"type":"string","maxLength":256}}},"user":{"type":"object","additionalProperties":false,"required":["call_sign","user_id","type"],"properties":{"call_sign":{"type":"string","maxLength":256},"user_id":{"type":"string","maxLength":256},"type":{"type":"string","maxLength":256}}}}}}}}`
)

package flighthub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const StateMapperVersion = "dji-flighthub-state/v1"

type StateFieldDiagnostic struct {
	Name     string `json:"name"`
	JSONType string `json:"jsonType"`
}

type PositionState struct {
	Longitude           float64  `json:"longitude"`
	Latitude            float64  `json:"latitude"`
	HeightMeters        *float64 `json:"heightMeters,omitempty"`
	CoordinateReference string   `json:"coordinateReference"`
	Validity            string   `json:"validity"`
	Reason              string   `json:"reason,omitempty"`
}

type AttitudeState struct {
	HeadingDegrees *float64 `json:"headingDegrees,omitempty"`
	PitchDegrees   *float64 `json:"pitchDegrees,omitempty"`
	RollDegrees    *float64 `json:"rollDegrees,omitempty"`
}

type NetworkState struct {
	Quality string   `json:"quality,omitempty"`
	Type    string   `json:"type,omitempty"`
	RateKBs *float64 `json:"rateKBs,omitempty"`
}

type BatteryState struct {
	Percent     *float64 `json:"percent,omitempty"`
	Temperature *float64 `json:"temperatureCelsius,omitempty"`
	VoltageMV   *float64 `json:"voltageMillivolts,omitempty"`
	StoreMode   string   `json:"storeMode,omitempty"`
}

type EnvironmentState struct {
	TemperatureCelsius *float64 `json:"temperatureCelsius,omitempty"`
	HumidityPercent    *float64 `json:"humidityPercent,omitempty"`
	WindSpeedMPS       *float64 `json:"windSpeedMetersPerSecond,omitempty"`
	Rainfall           string   `json:"rainfall,omitempty"`
}

type LiveState struct {
	Available bool   `json:"available"`
	Active    bool   `json:"active"`
	Summary   string `json:"summary,omitempty"`
}

type MappedDeviceState struct {
	MapperVersion      string                 `json:"mapperVersion"`
	ModelKey           string                 `json:"modelKey"`
	DeviceKind         string                 `json:"deviceKind"`
	KnownModel         bool                   `json:"knownModel"`
	Position           *PositionState         `json:"position,omitempty"`
	Attitude           *AttitudeState         `json:"attitude,omitempty"`
	Mode               string                 `json:"mode,omitempty"`
	Network            *NetworkState          `json:"network,omitempty"`
	Battery            *BatteryState          `json:"battery,omitempty"`
	Environment        *EnvironmentState      `json:"environment,omitempty"`
	Live               *LiveState             `json:"live,omitempty"`
	HorizontalSpeedMPS *float64               `json:"horizontalSpeedMetersPerSecond,omitempty"`
	VerticalSpeedMPS   *float64               `json:"verticalSpeedMetersPerSecond,omitempty"`
	CapabilityEvidence []string               `json:"capabilityEvidence"`
	Diagnostics        []StateFieldDiagnostic `json:"diagnostics"`
}

type modelStateMapper struct {
	Kind         string
	KnownFields  map[string]struct{}
	Capabilities []string
}

func fields(names ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(names))
	for _, name := range names {
		result[name] = struct{}{}
	}
	return result
}

var modelStateMappers = map[string]modelStateMapper{
	"3-2-0": {
		Kind: "dock",
		KnownFields: fields(
			"acc_time", "activation_time", "backup_battery", "battery_store_mode", "drone_battery_maintenance_info",
			"environment_temperature", "height", "home_position_is_valid", "humidity", "latitude", "live_capacity",
			"live_status", "longitude", "mode_code", "network_state", "position_state", "rainfall", "silent_mode",
			"temperature", "wind_speed", "wireless_link", "wireless_link_topo",
		),
		Capabilities: []string{"device.state.read", "device.position.read", "device.health.read"},
	},
	"0-91-1": {
		Kind: "aircraft",
		KnownFields: fields(
			"activation_time", "attitude_head", "attitude_pitch", "attitude_roll", "battery", "best_link_gateway",
			"camera_watermark_settings", "cameras", "commander_flight_height", "commander_flight_mode",
			"commander_mode_lost_action", "current_commander_flight_mode", "current_rth_mode", "height", "height_limit",
			"home_latitude", "home_longitude", "horizontal_speed", "is_near_height_limit", "latitude", "live_status",
			"longitude", "low_battery_warning_threshold", "mode_code", "mode_code_reason", "position_state", "rth_mode",
			"serious_low_battery_warning_threshold", "total_flight_time", "vertical_speed", "wind_direction", "wind_speed",
			"wireless_link_topo",
		),
		Capabilities: []string{"device.state.read", "device.position.read", "device.health.read"},
	},
}

func MapDeviceState(snapshot DeviceStateSnapshot) MappedDeviceState {
	mapper, known := modelStateMappers[snapshot.Model.Key]
	result := MappedDeviceState{
		MapperVersion: StateMapperVersion,
		ModelKey:      snapshot.Model.Key,
		DeviceKind:    normalizedDeviceKind(snapshot.Model.Class),
		KnownModel:    known,
		Diagnostics:   make([]StateFieldDiagnostic, 0),
	}
	if known {
		result.DeviceKind = mapper.Kind
		result.CapabilityEvidence = append([]string(nil), mapper.Capabilities...)
		result.Position = mapPosition(snapshot.State)
		result.Attitude = mapAttitude(snapshot.State)
		result.Mode, _ = scalarString(snapshot.State["mode_code"])
		result.Network = mapNetwork(snapshot.State)
		result.Battery = mapBattery(snapshot.State, mapper.Kind)
		result.Environment = mapEnvironment(snapshot.State)
		result.Live = mapLive(snapshot.State)
		result.HorizontalSpeedMPS, _ = numberValue(snapshot.State["horizontal_speed"])
		result.VerticalSpeedMPS, _ = numberValue(snapshot.State["vertical_speed"])
	}
	for name, value := range snapshot.State {
		if _, recognized := mapper.KnownFields[name]; !known || !recognized {
			result.Diagnostics = append(result.Diagnostics, StateFieldDiagnostic{Name: name, JSONType: rawJSONType(value)})
		}
	}
	sort.Slice(result.Diagnostics, func(left, right int) bool { return result.Diagnostics[left].Name < result.Diagnostics[right].Name })
	return result
}

func normalizedDeviceKind(class string) string {
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "airport", "dock":
		return "dock"
	case "aircraft", "drone", "uav":
		return "aircraft"
	default:
		return "unknown"
	}
}

func mapPosition(state map[string]json.RawMessage) *PositionState {
	longitude, hasLongitude := numberValue(state["longitude"])
	latitude, hasLatitude := numberValue(state["latitude"])
	if !hasLongitude && !hasLatitude {
		return nil
	}
	position := &PositionState{CoordinateReference: "unverified", Validity: "invalid"}
	if !hasLongitude || !hasLatitude {
		position.Reason = "coordinate_missing"
		return position
	}
	position.Longitude, position.Latitude = *longitude, *latitude
	position.HeightMeters, _ = numberValue(state["height"])
	switch {
	case position.Longitude < -180 || position.Longitude > 180 || position.Latitude < -90 || position.Latitude > 90:
		position.Reason = "coordinate_out_of_range"
	case position.Longitude == 0 && position.Latitude == 0:
		position.Reason = "coordinate_zero_sentinel"
	default:
		position.Validity = "valid"
	}
	return position
}

func mapAttitude(state map[string]json.RawMessage) *AttitudeState {
	heading, hasHeading := numberValue(state["attitude_head"])
	pitch, hasPitch := numberValue(state["attitude_pitch"])
	roll, hasRoll := numberValue(state["attitude_roll"])
	if !hasHeading && !hasPitch && !hasRoll {
		return nil
	}
	if hasHeading {
		normalized := math.Mod(*heading, 360)
		if normalized < 0 {
			normalized += 360
		}
		heading = &normalized
	}
	if hasPitch && (*pitch < -180 || *pitch > 180) {
		pitch = nil
	}
	if hasRoll && (*roll < -180 || *roll > 180) {
		roll = nil
	}
	return &AttitudeState{HeadingDegrees: heading, PitchDegrees: pitch, RollDegrees: roll}
}

func mapNetwork(state map[string]json.RawMessage) *NetworkState {
	var raw map[string]json.RawMessage
	if json.Unmarshal(state["network_state"], &raw) != nil || raw == nil {
		return nil
	}
	quality, _ := scalarString(raw["quality"])
	typeName, _ := scalarString(raw["type"])
	rate, _ := numberValue(raw["rate"])
	return &NetworkState{Quality: quality, Type: typeName, RateKBs: rate}
}

func mapBattery(state map[string]json.RawMessage, kind string) *BatteryState {
	result := &BatteryState{}
	if kind == "aircraft" {
		var raw map[string]json.RawMessage
		if json.Unmarshal(state["battery"], &raw) == nil {
			result.Percent, _ = numberValue(raw["capacity_percent"])
		}
	} else {
		var raw map[string]json.RawMessage
		if json.Unmarshal(state["backup_battery"], &raw) == nil {
			result.Temperature, _ = numberValue(raw["temperature"])
			result.VoltageMV, _ = numberValue(raw["voltage"])
		}
		result.StoreMode, _ = scalarString(state["battery_store_mode"])
	}
	if result.Percent == nil && result.Temperature == nil && result.VoltageMV == nil && result.StoreMode == "" {
		return nil
	}
	if result.Percent != nil && (*result.Percent < 0 || *result.Percent > 100) {
		result.Percent = nil
	}
	return result
}

func mapEnvironment(state map[string]json.RawMessage) *EnvironmentState {
	temperature, hasTemperature := numberValue(state["environment_temperature"])
	if !hasTemperature {
		temperature, hasTemperature = numberValue(state["temperature"])
	}
	humidity, hasHumidity := numberValue(state["humidity"])
	wind, hasWind := numberValue(state["wind_speed"])
	rainfall, hasRainfall := scalarString(state["rainfall"])
	if !hasTemperature && !hasHumidity && !hasWind && !hasRainfall {
		return nil
	}
	if hasHumidity && (*humidity < 0 || *humidity > 100) {
		humidity = nil
	}
	return &EnvironmentState{TemperatureCelsius: temperature, HumidityPercent: humidity, WindSpeedMPS: wind, Rainfall: rainfall}
}

func mapLive(state map[string]json.RawMessage) *LiveState {
	value, exists := state["live_status"]
	if !exists || len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(value, &list) == nil {
		return &LiveState{Available: true, Active: len(list) > 0, Summary: fmt.Sprintf("streams:%d", len(list))}
	}
	scalar, valid := scalarString(value)
	if !valid {
		return &LiveState{Available: true, Summary: rawJSONType(value)}
	}
	normalized := strings.ToLower(scalar)
	active := normalized != "" && normalized != "0" && normalized != "false" && normalized != "idle" && normalized != "stopped" && normalized != "offline"
	return &LiveState{Available: true, Active: active, Summary: normalized}
}

func numberValue(raw json.RawMessage) (*float64, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, false
	}
	var number float64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return nil, false
		}
		number = parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return nil, false
		}
		number = parsed
	default:
		return nil, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return nil, false
	}
	return &number, true
}

func scalarString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), true
	case json.Number:
		return typed.String(), true
	case bool:
		return strconv.FormatBool(typed), true
	default:
		return "", false
	}
}

func rawJSONType(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "invalid"
	}
	switch trimmed[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

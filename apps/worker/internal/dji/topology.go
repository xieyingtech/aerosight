package dji

import (
	"encoding/json"
	"errors"
	"fmt"

	"aerosight/worker/internal/adapter"
)

type topology struct {
	Devices []struct {
		SN       string `json:"sn"`
		Name     string `json:"name"`
		Domain   string `json:"domain"`
		Type     string `json:"type"`
		Firmware string `json:"firmware_version"`
		ParentSN string `json:"parent_sn"`
	} `json:"devices"`
}

func MapTopology(projectID int, adapterID int64, payload []byte) ([]adapter.Discovery, error) {
	if projectID <= 0 || adapterID <= 0 {
		return nil, errors.New("invalid DJI adapter scope")
	}
	var input topology
	if err := json.Unmarshal(payload, &input); err != nil {
		return nil, fmt.Errorf("decode DJI topology: %w", err)
	}
	if len(input.Devices) == 0 {
		return nil, errors.New("DJI topology contains no devices")
	}
	seen := map[string]bool{}
	discoveries := make([]adapter.Discovery, 0, len(input.Devices))
	for _, device := range input.Devices {
		if device.SN == "" || seen[device.SN] {
			return nil, errors.New("DJI topology has missing or duplicate device identity")
		}
		seen[device.SN] = true
		deviceType, capabilities, err := mapCapabilities(device.Domain, device.Type)
		if err != nil {
			return nil, err
		}
		discoveries = append(discoveries, adapter.Discovery{ProjectID: projectID, AdapterID: adapterID,
			ExternalDeviceID: device.SN, ExternalDeviceType: deviceType, Identity: map[string]any{
				"name": device.Name, "domain": device.Domain, "model": device.Type, "firmware": device.Firmware,
				"parentExternalDeviceId": nullableString(device.ParentSN), "capabilities": capabilities, "mappingVersion": "dji-topology/v1",
			}})
	}
	return discoveries, nil
}

func mapCapabilities(domain, model string) (string, []string, error) {
	switch domain {
	case "dock":
		return "dock", []string{"dock.charge", "dock.environment", "camera.live", "live.video", "flight.dispatch"}, nil
	case "device":
		if model == "" {
			return "", nil, errors.New("DJI aircraft model is missing")
		}
		return "drone", []string{
			"flight.navigate", "flight.route", "flight.takeoff", "flight.land", "flight.return_home", "command.rth",
			"camera.capture", "camera.live", "camera.photo", "camera.video", "gimbal.control", "live.video",
		}, nil
	default:
		return "", nil, fmt.Errorf("unsupported DJI topology domain %q", domain)
	}
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

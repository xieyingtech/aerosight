package simulator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DJIProtocolTransport deliberately exposes only MQTT-shaped publication. The
// protocol simulator cannot mutate application state or acknowledge commands
// through an internal application API.
type DJIProtocolTransport interface {
	Publish(context.Context, string, []byte) error
}

type DJIProtocolConfig struct {
	GatewaySN         string
	AircraftSN        string
	SessionID         string
	Topology          json.RawMessage
	TelemetryInterval time.Duration
	Faults            DJIFaults
	Media             DJIMediaController
	Now               func() time.Time
}

type DJIFaults struct {
	NackMethods       map[string]int
	TimeoutMethods    map[string]bool
	UnknownCapability string
}

type DJIProtocolSimulator struct {
	config    DJIProtocolConfig
	transport DJIProtocolTransport
	sequence  int64
}

const (
	DJIProductDock2M3D  = "dock2-m3d"
	DJIProductDock2M3TD = "dock2-m3td"
	DJIProductDock3M4D  = "dock3-m4d"
	DJIProductDock3M4TD = "dock3-m4td"
)

func Dock2Scenario(product, gatewaySN, aircraftSN string, now time.Time) (DJIProtocolConfig, error) {
	return djiScenario(product, gatewaySN, aircraftSN, now)
}

func Dock3Scenario(product, gatewaySN, aircraftSN string, now time.Time) (DJIProtocolConfig, error) {
	return djiScenario(product, gatewaySN, aircraftSN, now)
}

func djiScenario(product, gatewaySN, aircraftSN string, now time.Time) (DJIProtocolConfig, error) {
	dockType, aircraftType, subtype, firmware := 0, 0, -1, ""
	switch product {
	case DJIProductDock2M3D:
		dockType, aircraftType, subtype, firmware = 2, 91, 0, "14.03.07.01"
	case DJIProductDock2M3TD:
		dockType, aircraftType, subtype, firmware = 2, 91, 1, "14.03.07.01"
	case DJIProductDock3M4D:
		dockType, aircraftType, subtype, firmware = 3, 100, 0, "14.03.00.03"
	case DJIProductDock3M4TD:
		dockType, aircraftType, subtype, firmware = 3, 100, 1, "14.03.00.03"
	default:
		return DJIProtocolConfig{}, fmt.Errorf("DJI_SIMULATOR_PRODUCT_UNSUPPORTED: %s", product)
	}
	if gatewaySN == "" || aircraftSN == "" || now.IsZero() {
		return DJIProtocolConfig{}, errors.New("DJI_SIMULATOR_SCENARIO_IDENTITY_REQUIRED")
	}
	topology, err := json.Marshal(map[string]any{
		"tid": "sim-topology-" + gatewaySN, "bid": "sim-discovery-" + gatewaySN,
		"method": "update_topo", "timestamp": now.UTC().UnixMilli(),
		"data": map[string]any{
			"domain": "3", "type": dockType, "sub_type": 0, "thing_version": "1.0.0", "firmware_version": firmware,
			"sub_devices": []map[string]any{{
				"sn": aircraftSN, "domain": "0", "type": aircraftType, "sub_type": subtype,
				"index": "A", "thing_version": "1.0.0", "firmware_version": firmware,
			}},
		},
	})
	if err != nil {
		return DJIProtocolConfig{}, err
	}
	return DJIProtocolConfig{
		GatewaySN: gatewaySN, AircraftSN: aircraftSN, Topology: topology,
		TelemetryInterval: time.Second, Now: time.Now,
	}, nil
}

func InjectUnknownFirmware(config DJIProtocolConfig, firmware string) (DJIProtocolConfig, error) {
	if strings.TrimSpace(firmware) == "" {
		return DJIProtocolConfig{}, errors.New("DJI_SIMULATOR_FIRMWARE_REQUIRED")
	}
	var envelope map[string]any
	if err := json.Unmarshal(config.Topology, &envelope); err != nil {
		return DJIProtocolConfig{}, err
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		return DJIProtocolConfig{}, errors.New("DJI_SIMULATOR_TOPOLOGY_DATA_INVALID")
	}
	data["firmware_version"] = firmware
	if children, ok := data["sub_devices"].([]any); ok {
		for _, child := range children {
			if object, ok := child.(map[string]any); ok {
				object["firmware_version"] = firmware
			}
		}
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return DJIProtocolConfig{}, err
	}
	config.Topology = raw
	return config, nil
}

type djiServiceRequest struct {
	TransactionID string          `json:"tid"`
	BusinessID    string          `json:"bid"`
	TimestampMS   int64           `json:"timestamp"`
	Method        string          `json:"method"`
	Data          json.RawMessage `json:"data"`
}

func NewDJIProtocol(config DJIProtocolConfig, transport DJIProtocolTransport) (*DJIProtocolSimulator, error) {
	if strings.TrimSpace(config.GatewaySN) == "" || transport == nil {
		return nil, errors.New("DJI_SIMULATOR_IDENTITY_OR_TRANSPORT_REQUIRED")
	}
	if len(config.Topology) == 0 || !json.Valid(config.Topology) {
		return nil, errors.New("DJI_SIMULATOR_TOPOLOGY_INVALID")
	}
	var topology struct {
		TransactionID string          `json:"tid"`
		TimestampMS   int64           `json:"timestamp"`
		Data          json.RawMessage `json:"data"`
	}
	if json.Unmarshal(config.Topology, &topology) != nil || topology.TransactionID == "" || topology.TimestampMS <= 0 || len(topology.Data) == 0 {
		return nil, errors.New("DJI_SIMULATOR_TOPOLOGY_ENVELOPE_INVALID")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.SessionID == "" {
		identity := make([]byte, 6)
		if _, err := rand.Read(identity); err != nil {
			return nil, errors.New("DJI_SIMULATOR_SESSION_ID_FAILED")
		}
		config.SessionID = hex.EncodeToString(identity)
	}
	return &DJIProtocolSimulator{config: config, transport: transport}, nil
}

func (simulator *DJIProtocolSimulator) ServiceTopic() string {
	return "thing/product/" + simulator.config.GatewaySN + "/services"
}

func (simulator *DJIProtocolSimulator) PublishTopology(ctx context.Context) error {
	return simulator.transport.Publish(ctx, "sys/product/"+simulator.config.GatewaySN+"/status", simulator.config.Topology)
}

func (simulator *DJIProtocolSimulator) PublishTelemetry(ctx context.Context) error {
	if simulator.config.AircraftSN == "" {
		return errors.New("DJI_SIMULATOR_AIRCRAFT_REQUIRED")
	}
	simulator.sequence++
	now := simulator.config.Now().UTC()
	dockData := map[string]any{
		"seq": simulator.sequence, "mode_code": 1, "cover_state": 0,
		"environment_temperature": 24.5 + float64(simulator.sequence%5)/10,
		"temperature":             31.2, "humidity": 58, "wind_speed": 3.2, "rainfall": 0,
	}
	aircraftData := map[string]any{
		"seq": simulator.sequence, "latitude": 31.2304 + float64(simulator.sequence%20)/100000,
		"longitude": 121.4737 + float64(simulator.sequence%20)/100000,
		"height":    42.0, "horizontal_speed": 2.4, "vertical_speed": 0.0,
		"battery": map[string]any{"capacity_percent": 86},
	}
	if simulator.config.Faults.UnknownCapability != "" {
		aircraftData["capabilities"] = []string{simulator.config.Faults.UnknownCapability}
	}
	for _, publication := range []struct {
		topic string
		data  map[string]any
	}{
		{"thing/product/" + simulator.config.GatewaySN + "/osd", dockData},
		{"thing/product/" + simulator.config.AircraftSN + "/osd", aircraftData},
	} {
		payload, err := json.Marshal(map[string]any{
			"tid": fmt.Sprintf("sim-osd-%s-%s-%d", simulator.config.GatewaySN, simulator.config.SessionID, simulator.sequence),
			"bid": "sim-telemetry-" + simulator.config.GatewaySN, "timestamp": now.UnixMilli(),
			"gateway": simulator.config.GatewaySN, "data": publication.data,
		})
		if err != nil {
			return err
		}
		if err := simulator.transport.Publish(ctx, publication.topic, payload); err != nil {
			return err
		}
	}
	return nil
}

func (simulator *DJIProtocolSimulator) RunTelemetry(ctx context.Context) error {
	if simulator.config.TelemetryInterval <= 0 || simulator.config.AircraftSN == "" {
		return nil
	}
	if err := simulator.PublishTelemetry(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(simulator.config.TelemetryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := simulator.PublishTelemetry(ctx); err != nil {
				return err
			}
		}
	}
}

func (simulator *DJIProtocolSimulator) HandleMessage(ctx context.Context, topic string, payload []byte) error {
	if topic != simulator.ServiceTopic() {
		return fmt.Errorf("DJI_SIMULATOR_TOPIC_UNSUPPORTED: %s", topic)
	}
	var request djiServiceRequest
	if json.Unmarshal(payload, &request) != nil || request.TransactionID == "" || request.BusinessID == "" ||
		request.Method == "" || request.TimestampMS <= 0 || len(request.Data) == 0 || !json.Valid(request.Data) {
		return errors.New("DJI_SIMULATOR_SERVICE_ENVELOPE_INVALID")
	}
	if simulator.config.Faults.TimeoutMethods[request.Method] {
		return nil
	}
	result := 0
	if configured, exists := simulator.config.Faults.NackMethods[request.Method]; exists {
		result = configured
		if result == 0 {
			result = 1
		}
	}
	if result == 0 && (request.Method == "live_start_push" || request.Method == "live_stop_push") {
		var mediaRequest struct {
			URL     string `json:"url"`
			VideoID string `json:"video_id"`
		}
		if json.Unmarshal(request.Data, &mediaRequest) != nil || mediaRequest.VideoID == "" || simulator.config.Media == nil {
			result = 1
		} else if request.Method == "live_start_push" {
			if err := simulator.config.Media.Start(ctx, mediaRequest.VideoID, mediaRequest.URL); err != nil {
				result = 1
			}
		} else if err := simulator.config.Media.Stop(mediaRequest.VideoID); err != nil {
			result = 1
		}
	}
	reply, err := json.Marshal(map[string]any{
		"tid": request.TransactionID, "bid": request.BusinessID,
		"timestamp": simulator.config.Now().UTC().UnixMilli(), "gateway": simulator.config.GatewaySN,
		"method": request.Method, "data": map[string]any{"result": result},
	})
	if err != nil {
		return err
	}
	return simulator.transport.Publish(ctx, "thing/product/"+simulator.config.GatewaySN+"/services_reply", reply)
}

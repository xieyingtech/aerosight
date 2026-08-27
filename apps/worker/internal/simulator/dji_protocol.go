package simulator

import (
	"context"
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
	Topology          json.RawMessage
	TelemetryInterval time.Duration
	Now               func() time.Time
}

type DJIProtocolSimulator struct {
	config    DJIProtocolConfig
	transport DJIProtocolTransport
	sequence  int64
}

const (
	DJIProductDock2M3D  = "dock2-m3d"
	DJIProductDock2M3TD = "dock2-m3td"
)

func Dock2Scenario(product, gatewaySN, aircraftSN string, now time.Time) (DJIProtocolConfig, error) {
	subtype := -1
	switch product {
	case DJIProductDock2M3D:
		subtype = 0
	case DJIProductDock2M3TD:
		subtype = 1
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
			"domain": "3", "type": 2, "sub_type": 0, "thing_version": "14.03.07.01",
			"sub_devices": []map[string]any{{
				"sn": aircraftSN, "domain": "0", "type": 91, "sub_type": subtype,
				"index": "A", "thing_version": "14.03.07.01",
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
	for _, publication := range []struct {
		topic string
		data  map[string]any
	}{
		{"thing/product/" + simulator.config.GatewaySN + "/osd", dockData},
		{"thing/product/" + simulator.config.AircraftSN + "/osd", aircraftData},
	} {
		payload, err := json.Marshal(map[string]any{
			"tid": fmt.Sprintf("sim-osd-%s-%d", simulator.config.GatewaySN, simulator.sequence),
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
	reply, err := json.Marshal(map[string]any{
		"tid": request.TransactionID, "bid": request.BusinessID,
		"timestamp": simulator.config.Now().UTC().UnixMilli(), "gateway": simulator.config.GatewaySN,
		"method": request.Method, "data": map[string]any{"result": 0},
	})
	if err != nil {
		return err
	}
	return simulator.transport.Publish(ctx, "thing/product/"+simulator.config.GatewaySN+"/services_reply", reply)
}

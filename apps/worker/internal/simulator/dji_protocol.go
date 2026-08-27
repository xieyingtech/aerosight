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
	GatewaySN string
	Topology  json.RawMessage
	Now       func() time.Time
}

type DJIProtocolSimulator struct {
	config    DJIProtocolConfig
	transport DJIProtocolTransport
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

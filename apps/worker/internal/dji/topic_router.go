package dji

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aerosight/worker/internal/adapter"
)

type RouteKind string

const (
	RouteTopology     RouteKind = "topology"
	RouteState        RouteKind = "state"
	RouteTelemetry    RouteKind = "telemetry"
	RouteEvent        RouteKind = "event"
	RouteRequest      RouteKind = "request"
	RouteServiceReply RouteKind = "service_reply"
)

type RouteContext struct {
	ProjectID         int
	AdapterID         int64
	AllowedGatewaySNs map[string]bool
}

type djiMessageEnvelope struct {
	TransactionID string          `json:"tid"`
	BusinessID    string          `json:"bid"`
	TimestampMS   int64           `json:"timestamp"`
	GatewaySN     string          `json:"gateway"`
	Method        string          `json:"method"`
	Data          json.RawMessage `json:"data"`
}

type RoutedMessage struct {
	Kind          RouteKind
	Topic         string
	GatewaySN     string
	DeviceSN      string
	TransactionID string
	BusinessID    string
	Method        string
	TimestampMS   int64
	Sequence      *int64
	QoS           byte
	Duplicate     bool
	RawPayload    json.RawMessage
	Envelope      adapter.UpstreamEnvelope
}

func parseTopic(topic string) (RouteKind, string, bool, error) {
	parts := strings.Split(topic, "/")
	if len(parts) != 4 || parts[1] != "product" || parts[2] == "" {
		return "", "", false, errors.New("DJI_TOPIC_UNSUPPORTED")
	}
	identifier := parts[2]
	if parts[0] == "sys" && parts[3] == "status" {
		return RouteTopology, identifier, true, nil
	}
	if parts[0] != "thing" {
		return "", "", false, errors.New("DJI_TOPIC_UNSUPPORTED")
	}
	switch parts[3] {
	case "state":
		return RouteState, identifier, false, nil
	case "osd":
		return RouteTelemetry, identifier, false, nil
	case "events":
		return RouteEvent, identifier, true, nil
	case "requests":
		return RouteRequest, identifier, true, nil
	case "services_reply":
		return RouteServiceReply, identifier, true, nil
	default:
		return "", "", false, errors.New("DJI_TOPIC_UNSUPPORTED")
	}
}

func routeEventType(kind RouteKind) string {
	switch kind {
	case RouteTopology:
		return "device.topology"
	case RouteState:
		return "device.state"
	case RouteTelemetry:
		return "device.telemetry"
	case RouteEvent:
		return "device.event"
	case RouteRequest:
		return "device.request"
	case RouteServiceReply:
		return "command.reply"
	default:
		return ""
	}
}

func sequenceFromData(raw json.RawMessage) *int64 {
	var data struct {
		Sequence *int64 `json:"seq"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return nil
	}
	return data.Sequence
}

func RouteMQTTMessage(scope RouteContext, message MQTTMessage) (RoutedMessage, error) {
	if scope.ProjectID <= 0 || scope.AdapterID <= 0 || len(scope.AllowedGatewaySNs) == 0 {
		return RoutedMessage{}, errors.New("DJI_ROUTE_SCOPE_INVALID")
	}
	kind, topicIdentity, topicIsGateway, err := parseTopic(message.Topic)
	if err != nil {
		return RoutedMessage{}, err
	}
	var vendor djiMessageEnvelope
	if err := json.Unmarshal(message.Payload, &vendor); err != nil {
		return RoutedMessage{}, errors.New("DJI_MESSAGE_JSON_INVALID")
	}
	if vendor.TransactionID == "" || vendor.TimestampMS <= 0 || len(vendor.Data) == 0 || !json.Valid(vendor.Data) {
		return RoutedMessage{}, errors.New("DJI_MESSAGE_ENVELOPE_INVALID")
	}
	gatewaySN := vendor.GatewaySN
	if topicIsGateway {
		gatewaySN = topicIdentity
		if vendor.GatewaySN != "" && vendor.GatewaySN != gatewaySN {
			return RoutedMessage{}, errors.New("DJI_MESSAGE_GATEWAY_MISMATCH")
		}
	} else if gatewaySN == "" {
		return RoutedMessage{}, errors.New("DJI_MESSAGE_GATEWAY_REQUIRED")
	}
	if !scope.AllowedGatewaySNs[gatewaySN] {
		return RoutedMessage{}, errors.New("DJI_MESSAGE_GATEWAY_NOT_CLAIMED")
	}
	deviceSN := topicIdentity
	if topicIsGateway {
		deviceSN = gatewaySN
	}
	canonicalPayload, err := json.Marshal(map[string]any{
		"protocol": "dji-cloud-api", "routeKind": kind, "transactionId": vendor.TransactionID,
		"businessId": vendor.BusinessID, "method": vendor.Method, "data": vendor.Data,
	})
	if err != nil {
		return RoutedMessage{}, err
	}
	signatureContext, _ := json.Marshal(map[string]any{
		"gatewaySn": gatewaySN, "topic": message.Topic, "qos": message.QoS, "duplicate": message.Duplicate,
	})
	topicHash := sha256.Sum256([]byte(message.Topic))
	eventID := fmt.Sprintf("dji:%d:%s:%s", scope.AdapterID, hex.EncodeToString(topicHash[:8]), vendor.TransactionID)
	capturedAt := time.UnixMilli(vendor.TimestampMS).UTC()
	return RoutedMessage{
		Kind: kind, Topic: message.Topic, GatewaySN: gatewaySN, DeviceSN: deviceSN,
		TransactionID: vendor.TransactionID, BusinessID: vendor.BusinessID, Method: vendor.Method,
		TimestampMS: vendor.TimestampMS, Sequence: sequenceFromData(vendor.Data), QoS: message.QoS,
		Duplicate: message.Duplicate, RawPayload: append(json.RawMessage(nil), message.Payload...),
		Envelope: adapter.UpstreamEnvelope{
			SchemaVersion: adapter.SchemaVersionV1, EventID: eventID, AdapterID: scope.AdapterID,
			ProjectID: scope.ProjectID, ExternalDeviceID: deviceSN, EventType: routeEventType(kind),
			CapturedAt: capturedAt, ReceivedAt: message.ReceivedAt, Sequence: sequenceFromData(vendor.Data),
			Payload: canonicalPayload, SignatureContext: signatureContext,
		},
	}, nil
}

func (message RoutedMessage) Ordered() bool {
	return message.Kind == RouteTopology || message.Kind == RouteState || message.Kind == RouteTelemetry
}

func (message RoutedMessage) RouteKey() string {
	return string(message.Kind) + ":" + message.DeviceSN
}

package dji

import (
	"context"
	"encoding/json"
	"errors"

	"aerosight/worker/internal/driver"
)

const (
	DriverKey     = "dji.cloud"
	DriverVersion = "1.0.0"
)

var commandCapabilities = []driver.CapabilityDefinition{
	{Code: "mission.execute", Kind: driver.CapabilityCommand, Risk: driver.RiskHigh, InputSchema: json.RawMessage(`{"type":"object"}`)},
	{Code: "mission.cancel", Kind: driver.CapabilityCommand, Risk: driver.RiskHigh, InputSchema: json.RawMessage(`{"type":"object"}`)},
	{Code: "flight.return_home", Kind: driver.CapabilityCommand, Risk: driver.RiskCritical, InputSchema: json.RawMessage(`{"type":"object"}`)},
	{Code: "stream.video.control", Kind: driver.CapabilityCommand, Risk: driver.RiskMedium, InputSchema: json.RawMessage(`{"type":"object"}`)},
	{Code: "dock.debug.control", Kind: driver.CapabilityCommand, Risk: driver.RiskCritical, InputSchema: json.RawMessage(`{"type":"object"}`)},
}

var streamCapabilities = []driver.CapabilityDefinition{
	{Code: "stream.telemetry.read", Kind: driver.CapabilityStream, Risk: driver.RiskLow, OutputSchema: json.RawMessage(`{"type":"object"}`)},
	{Code: "stream.sensor.read", Kind: driver.CapabilityStream, Risk: driver.RiskLow, OutputSchema: json.RawMessage(`{"type":"object"}`)},
	{Code: "stream.video.read", Kind: driver.CapabilityStream, Risk: driver.RiskLow, OutputSchema: json.RawMessage(`{"type":"object"}`)},
	{Code: "stream.events.read", Kind: driver.CapabilityStream, Risk: driver.RiskLow, OutputSchema: json.RawMessage(`{"type":"object"}`)},
}

func Manifest() driver.Manifest {
	capabilities := []driver.CapabilityDefinition{{
		Code: "state.read", Kind: driver.CapabilityRead, Risk: driver.RiskLow,
		OutputSchema: json.RawMessage(`{"type":"object"}`),
	}}
	capabilities = append(capabilities, commandCapabilities...)
	capabilities = append(capabilities, streamCapabilities...)
	return driver.Manifest{
		DriverKey: DriverKey, Version: DriverVersion, DisplayName: "DJI Cloud API",
		Protocols: []string{"mqtt5"}, Capabilities: capabilities,
		Streams: []driver.StreamDefinition{
			{ChannelKey: "telemetry.primary", CapabilityCode: "stream.telemetry.read", DataType: driver.StreamTelemetry, Unit: "mixed", Schema: json.RawMessage(`{"type":"object","properties":{"seq":{"type":"integer"},"latitude":{"type":"number","x-unit":"degree"},"longitude":{"type":"number","x-unit":"degree"},"height":{"type":"number","x-unit":"m"},"horizontal_speed":{"type":"number","x-unit":"m/s"},"vertical_speed":{"type":"number","x-unit":"m/s"}}}`)},
			{ChannelKey: "sensor.primary", CapabilityCode: "stream.sensor.read", DataType: driver.StreamSensor, Unit: "mixed", Schema: json.RawMessage(`{"type":"object","properties":{"samples":{"type":"object","additionalProperties":{"type":"object","required":["value","unit"],"properties":{"value":{},"unit":{"type":"string"}}}}},"required":["samples"]}`)},
			{ChannelKey: "video.primary", CapabilityCode: "stream.video.read", DataType: driver.StreamVideo, Schema: json.RawMessage(`{"type":"object","properties":{"sessionId":{"type":"string"},"state":{"type":"string"},"playback":{"type":"object"}}}`)},
			{ChannelKey: "events.primary", CapabilityCode: "stream.events.read", DataType: driver.StreamEvents, Schema: json.RawMessage(`{"type":"object"}`)},
		},
	}
}

func RegisterDriver(registry *driver.Registry, protocolHandler driver.ProtocolHandler) error {
	if registry == nil || protocolHandler == nil {
		return errors.New("DJI driver registry and MQTT 5 handler are required")
	}
	unsupportedCommand := func(context.Context, driver.Command) (driver.CommandResult, error) {
		return driver.CommandResult{}, errors.New("DJI_COMMAND_MAPPING_NOT_REGISTERED")
	}
	unsupportedStream := func(context.Context, driver.StreamRequest) error {
		return errors.New("DJI_STREAM_COORDINATOR_NOT_REGISTERED")
	}
	runtime := driver.Runtime{
		Manifest:         Manifest(),
		ProtocolHandlers: map[string]driver.ProtocolHandler{"mqtt5": protocolHandler},
		CommandHandlers:  make(map[string]driver.CommandHandler),
		StreamHandlers:   make(map[string]driver.StreamHandler),
	}
	for _, capability := range commandCapabilities {
		runtime.CommandHandlers[capability.Code] = unsupportedCommand
	}
	for _, stream := range runtime.Manifest.Streams {
		runtime.StreamHandlers[stream.ChannelKey] = unsupportedStream
	}
	return registry.Register(runtime)
}

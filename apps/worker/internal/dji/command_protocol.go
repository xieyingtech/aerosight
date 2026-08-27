package dji

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const CommandMappingVersion = "dji-cloud-api/2026-08-v1"

type CommandMapping struct {
	CapabilityCode string
	CommandKey     string
	Method         string
	Validate       func(json.RawMessage) error
}

type ServiceCommand struct {
	MappingVersion string
	Topic          string
	TransactionID  string
	BusinessID     string
	Method         string
	Payload        json.RawMessage
}

type ServiceReply struct {
	TransactionID string
	BusinessID    string
	Method        string
	Result        int
	Output        json.RawMessage
}

type ReplyOutcome string

const (
	ReplyAcknowledged ReplyOutcome = "acknowledged"
	ReplyNacked       ReplyOutcome = "nacked"
)

var commandMappings = []CommandMapping{
	{CapabilityCode: "mission.execute", CommandKey: "prepare", Method: "flighttask_prepare", Validate: validateFlightTaskPrepare},
	{CapabilityCode: "mission.execute", CommandKey: "execute", Method: "flighttask_execute", Validate: requireString("flight_id")},
	{CapabilityCode: "mission.cancel", CommandKey: "cancel", Method: "flighttask_undo", Validate: requireStringArray("flight_ids")},
	{CapabilityCode: "flight.return_home", CommandKey: "return_home", Method: "return_home", Validate: requireObject},
	{CapabilityCode: "flight.return_home", CommandKey: "flight.return_home", Method: "return_home", Validate: requireObject},
}

func validateFlightTaskPrepare(raw json.RawMessage) error {
	var parameters struct {
		FlightID    string `json:"flight_id"`
		ExecuteTime *int64 `json:"execute_time"`
		TaskType    *int   `json:"task_type"`
		File        struct {
			URL         string `json:"url"`
			Fingerprint string `json:"fingerprint"`
		} `json:"file"`
	}
	if json.Unmarshal(raw, &parameters) != nil {
		return errors.New("DJI_COMMAND_PARAMETERS_OBJECT_REQUIRED")
	}
	if strings.TrimSpace(parameters.FlightID) == "" || parameters.TaskType == nil || *parameters.TaskType < 0 || *parameters.TaskType > 2 ||
		strings.TrimSpace(parameters.File.URL) == "" || strings.TrimSpace(parameters.File.Fingerprint) == "" {
		return errors.New("DJI_FLIGHTTASK_PREPARE_PARAMETERS_REQUIRED")
	}
	if (*parameters.TaskType == 0 || *parameters.TaskType == 1) && parameters.ExecuteTime == nil {
		return errors.New("DJI_FLIGHTTASK_EXECUTE_TIME_REQUIRED")
	}
	return nil
}

func ResolveCommandMapping(capabilityCode, commandKey string) (CommandMapping, bool) {
	for _, mapping := range commandMappings {
		if mapping.CapabilityCode == capabilityCode && mapping.CommandKey == commandKey {
			return mapping, true
		}
	}
	return CommandMapping{}, false
}

func BuildServiceCommand(gatewaySN, commandID, businessID, capabilityCode, commandKey string, parameters json.RawMessage, now time.Time) (ServiceCommand, error) {
	if strings.TrimSpace(gatewaySN) == "" || strings.TrimSpace(commandID) == "" || strings.TrimSpace(businessID) == "" || now.IsZero() {
		return ServiceCommand{}, errors.New("DJI_COMMAND_IDENTITY_REQUIRED")
	}
	mapping, exists := ResolveCommandMapping(capabilityCode, commandKey)
	if !exists {
		return ServiceCommand{}, errors.New("DJI_COMMAND_MAPPING_UNKNOWN")
	}
	if err := mapping.Validate(parameters); err != nil {
		return ServiceCommand{}, err
	}
	payload, err := json.Marshal(map[string]any{
		"tid": commandID, "bid": businessID, "timestamp": now.UnixMilli(),
		"method": mapping.Method, "data": json.RawMessage(parameters),
	})
	if err != nil {
		return ServiceCommand{}, err
	}
	return ServiceCommand{
		MappingVersion: CommandMappingVersion, Topic: "thing/product/" + gatewaySN + "/services",
		TransactionID: commandID, BusinessID: businessID, Method: mapping.Method, Payload: payload,
	}, nil
}

func DecodeServiceReply(data json.RawMessage, transactionID, businessID, method string) (ServiceReply, error) {
	if transactionID == "" || businessID == "" || method == "" {
		return ServiceReply{}, errors.New("DJI_REPLY_CORRELATION_REQUIRED")
	}
	var body struct {
		Result *int            `json:"result"`
		Output json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(data, &body); err != nil || body.Result == nil {
		return ServiceReply{}, errors.New("DJI_REPLY_RESULT_INVALID")
	}
	return ServiceReply{TransactionID: transactionID, BusinessID: businessID, Method: method, Result: *body.Result, Output: body.Output}, nil
}

func (reply ServiceReply) Outcome() ReplyOutcome {
	if reply.Result == 0 {
		return ReplyAcknowledged
	}
	return ReplyNacked
}

func requireObject(raw json.RawMessage) error {
	var object map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &object) != nil {
		return errors.New("DJI_COMMAND_PARAMETERS_OBJECT_REQUIRED")
	}
	return nil
}

func requireString(field string) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			return errors.New("DJI_COMMAND_PARAMETERS_OBJECT_REQUIRED")
		}
		var value string
		if json.Unmarshal(object[field], &value) != nil || strings.TrimSpace(value) == "" {
			return fmt.Errorf("DJI_COMMAND_PARAMETER_REQUIRED: %s", field)
		}
		return nil
	}
}

func requireStringArray(field string) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			return errors.New("DJI_COMMAND_PARAMETERS_OBJECT_REQUIRED")
		}
		var values []string
		if json.Unmarshal(object[field], &values) != nil || len(values) == 0 {
			return fmt.Errorf("DJI_COMMAND_PARAMETER_REQUIRED: %s", field)
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("DJI_COMMAND_PARAMETER_REQUIRED: %s", field)
			}
		}
		return nil
	}
}

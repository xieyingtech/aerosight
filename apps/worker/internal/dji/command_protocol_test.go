package dji

import (
	"encoding/json"
	"testing"
	"time"
)

func TestVersionedCommandMappingsBuildOfficialServiceMessages(t *testing.T) {
	now := time.UnixMilli(1787821300000).UTC()
	tests := []struct {
		capability string
		commandKey string
		parameters string
		method     string
	}{
		{"mission.execute", "prepare", `{"flight_id":"flight-1","task_type":0,"execute_time":1787821305000,"file":{"url":"https://example.invalid/route.kmz","fingerprint":"sha256:demo"}}`, "flighttask_prepare"},
		{"mission.execute", "execute", `{"flight_id":"flight-1"}`, "flighttask_execute"},
		{"mission.cancel", "cancel", `{"flight_ids":["flight-1"]}`, "flighttask_undo"},
		{"flight.return_home", "return_home", `{}`, "return_home"},
	}
	for _, fixture := range tests {
		command, err := BuildServiceCommand("DOCK2-DEMO-001", "tid-1", "bid-1", "dock2", fixture.capability, fixture.commandKey, json.RawMessage(fixture.parameters), now)
		if err != nil {
			t.Fatal(err)
		}
		if command.MappingVersion != CommandMappingVersion || command.Topic != "thing/product/DOCK2-DEMO-001/services" || command.Method != fixture.method {
			t.Fatalf("unexpected service command: %+v", command)
		}
		var payload struct {
			TransactionID string `json:"tid"`
			BusinessID    string `json:"bid"`
			Method        string `json:"method"`
		}
		if json.Unmarshal(command.Payload, &payload) != nil || payload.TransactionID != "tid-1" || payload.BusinessID != "bid-1" || payload.Method != fixture.method {
			t.Fatalf("correlation fields were not retained: %s", command.Payload)
		}
	}
}

func TestCommandMappingRejectsUnknownOrMalformedCommands(t *testing.T) {
	now := time.Now().UTC()
	for _, fixture := range []struct{ capability, commandKey, parameters string }{
		{"dock.debug.control", "open_cover", `{}`},
		{"mission.execute", "execute", `{}`},
		{"mission.cancel", "cancel", `{"flight_ids":[]}`},
	} {
		if _, err := BuildServiceCommand("dock", "tid", "bid", "dock2", fixture.capability, fixture.commandKey, json.RawMessage(fixture.parameters), now); err == nil {
			t.Fatalf("unsafe mapping unexpectedly accepted: %+v", fixture)
		}
	}
}

func TestServiceReplyClassifiesAckNackAndErrors(t *testing.T) {
	ack, err := DecodeServiceReply(json.RawMessage(`{"result":0,"output":{"status":"ok"}}`), "tid", "bid", "return_home")
	if err != nil || ack.Outcome() != ReplyAcknowledged {
		t.Fatalf("ACK not classified: reply=%+v err=%v", ack, err)
	}
	nack, err := DecodeServiceReply(json.RawMessage(`{"result":326108}`), "tid", "bid", "return_home")
	if err != nil || nack.Outcome() != ReplyNacked {
		t.Fatalf("NACK not classified: reply=%+v err=%v", nack, err)
	}
	if _, err := DecodeServiceReply(json.RawMessage(`{"output":{}}`), "tid", "bid", "return_home"); err == nil {
		t.Fatal("reply without result was accepted")
	}
	if _, err := DecodeServiceReply(json.RawMessage(`{"result":0}`), "", "bid", "return_home"); err == nil {
		t.Fatal("reply without complete correlation was accepted")
	}
}

func TestDock2AndDock3DebugWhitelistMapsOnlySupportedActions(t *testing.T) {
	now := time.UnixMilli(1787821300000).UTC()
	actions := []struct {
		key        string
		parameters string
		method     string
	}{
		{"debug.open", `{}`, "debug_mode_open"},
		{"debug.close", `{}`, "debug_mode_close"},
		{"cover.open", `{}`, "cover_open"},
		{"cover.close", `{}`, "cover_close"},
		{"aircraft.power_on", `{}`, "drone_open"},
		{"aircraft.power_off", `{}`, "drone_close"},
		{"charge.start", `{}`, "charge_open"},
		{"charge.stop", `{}`, "charge_close"},
		{"alarm.enable", `{"action":1}`, "alarm_state_switch"},
		{"alarm.disable", `{"action":0}`, "alarm_state_switch"},
		{"reboot", `{}`, "device_reboot"},
	}
	for _, family := range []string{"dock2", "dock3"} {
		for _, action := range actions {
			command, err := BuildServiceCommand("dock", "tid", "bid", family, "dock.debug.control", action.key, json.RawMessage(action.parameters), now)
			if err != nil || command.Method != action.method {
				t.Fatalf("%s action %s did not map to %s: command=%+v err=%v", family, action.key, action.method, command, err)
			}
		}
	}
	for _, fixture := range []struct {
		family, key, parameters string
	}{
		{"dock1", "cover.open", `{}`},
		{"dock2", "shell.execute", `{}`},
		{"dock2", "alarm.enable", `{"action":0}`},
		{"dock3", "reboot", `{"force":true}`},
	} {
		if _, err := BuildServiceCommand("dock", "tid", "bid", fixture.family, "dock.debug.control", fixture.key, json.RawMessage(fixture.parameters), now); err == nil {
			t.Fatalf("non-whitelisted debug action accepted: %+v", fixture)
		}
	}
}

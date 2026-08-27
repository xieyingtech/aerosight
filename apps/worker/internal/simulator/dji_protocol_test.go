package simulator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type protocolPublication struct {
	topic   string
	payload []byte
}

type protocolTransportFixture struct{ publications []protocolPublication }

func (fixture *protocolTransportFixture) Publish(_ context.Context, topic string, payload []byte) error {
	fixture.publications = append(fixture.publications, protocolPublication{topic: topic, payload: append([]byte(nil), payload...)})
	return nil
}

func TestDJIProtocolPublishesTopologyAndCorrelatedReplyThroughTransport(t *testing.T) {
	topology, err := os.ReadFile("../../testdata/dji/dock2-m3td-topology.json")
	if err != nil {
		t.Fatal(err)
	}
	transport := &protocolTransportFixture{}
	protocol, err := NewDJIProtocol(DJIProtocolConfig{
		GatewaySN: "DOCK2-DEMO-001", Topology: topology,
		Now: func() time.Time { return time.UnixMilli(1_787_821_300_200).UTC() },
	}, transport)
	if err != nil {
		t.Fatal(err)
	}
	if protocol.ServiceTopic() != "thing/product/DOCK2-DEMO-001/services" {
		t.Fatalf("unexpected subscription topic %q", protocol.ServiceTopic())
	}
	if err := protocol.PublishTopology(context.Background()); err != nil {
		t.Fatal(err)
	}
	command := []byte(`{"tid":"command-1","bid":"business-1","timestamp":1787821300100,"method":"return_home","data":{}}`)
	if err := protocol.HandleMessage(context.Background(), protocol.ServiceTopic(), command); err != nil {
		t.Fatal(err)
	}
	if len(transport.publications) != 2 || transport.publications[0].topic != "sys/product/DOCK2-DEMO-001/status" ||
		transport.publications[1].topic != "thing/product/DOCK2-DEMO-001/services_reply" {
		t.Fatalf("protocol did not use MQTT publications: %+v", transport.publications)
	}
	var reply struct {
		TransactionID string `json:"tid"`
		BusinessID    string `json:"bid"`
		GatewaySN     string `json:"gateway"`
		Method        string `json:"method"`
		Data          struct {
			Result int `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(transport.publications[1].payload, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.TransactionID != "command-1" || reply.BusinessID != "business-1" || reply.GatewaySN != "DOCK2-DEMO-001" || reply.Method != "return_home" || reply.Data.Result != 0 {
		t.Fatalf("reply lost correlation or result: %+v", reply)
	}
}

func TestDock2ScenariosPublishTypedTopologyAndRealtimeData(t *testing.T) {
	for _, fixture := range []struct {
		product  string
		aircraft string
		subtype  int
	}{
		{DJIProductDock2M3D, "M3D-DEMO-001", 0},
		{DJIProductDock2M3TD, "M3TD-DEMO-001", 1},
	} {
		t.Run(fixture.product, func(t *testing.T) {
			now := time.UnixMilli(1_787_821_200_000).UTC()
			config, err := Dock2Scenario(fixture.product, "DOCK2-DEMO-001", fixture.aircraft, now)
			if err != nil {
				t.Fatal(err)
			}
			config.Now = func() time.Time { return now.Add(time.Second) }
			transport := &protocolTransportFixture{}
			protocol, err := NewDJIProtocol(config, transport)
			if err != nil {
				t.Fatal(err)
			}
			if err := protocol.PublishTopology(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := protocol.PublishTelemetry(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(transport.publications) != 3 {
				t.Fatalf("got %d publications, want topology plus dock and aircraft OSD", len(transport.publications))
			}
			var topology struct {
				Data struct {
					Type       int `json:"type"`
					SubDevices []struct {
						SN      string `json:"sn"`
						Subtype int    `json:"sub_type"`
					} `json:"sub_devices"`
				} `json:"data"`
			}
			if err := json.Unmarshal(transport.publications[0].payload, &topology); err != nil {
				t.Fatal(err)
			}
			if topology.Data.Type != 2 || len(topology.Data.SubDevices) != 1 ||
				topology.Data.SubDevices[0].SN != fixture.aircraft || topology.Data.SubDevices[0].Subtype != fixture.subtype {
				t.Fatalf("scenario emitted wrong product topology: %+v", topology)
			}
			if transport.publications[1].topic != "thing/product/DOCK2-DEMO-001/osd" ||
				transport.publications[2].topic != "thing/product/"+fixture.aircraft+"/osd" {
				t.Fatalf("scenario emitted wrong realtime topics: %+v", transport.publications)
			}
			if !strings.Contains(string(transport.publications[1].payload), "environment_temperature") ||
				!strings.Contains(string(transport.publications[2].payload), "latitude") {
				t.Fatal("dock sensor or aircraft telemetry sample is missing")
			}
		})
	}
}

func TestDock3ScenariosAndFaultInjection(t *testing.T) {
	now := time.UnixMilli(1_787_821_200_000).UTC()
	for _, fixture := range []struct {
		product  string
		aircraft string
		subtype  int
	}{
		{DJIProductDock3M4D, "M4D-DEMO-001", 0},
		{DJIProductDock3M4TD, "M4TD-DEMO-001", 1},
	} {
		config, err := Dock3Scenario(fixture.product, "DOCK3-DEMO-001", fixture.aircraft, now)
		if err != nil {
			t.Fatal(err)
		}
		var topology struct {
			Data struct {
				Type       int `json:"type"`
				SubDevices []struct {
					Subtype int `json:"sub_type"`
				} `json:"sub_devices"`
			} `json:"data"`
		}
		if err := json.Unmarshal(config.Topology, &topology); err != nil {
			t.Fatal(err)
		}
		if topology.Data.Type != 3 || topology.Data.SubDevices[0].Subtype != fixture.subtype {
			t.Fatalf("wrong Dock 3 scenario topology: %+v", topology)
		}
	}

	config, err := Dock3Scenario(DJIProductDock3M4TD, "DOCK3-DEMO-001", "M4TD-DEMO-001", now)
	if err != nil {
		t.Fatal(err)
	}
	config, err = InjectUnknownFirmware(config, "99.99.99.99")
	if err != nil {
		t.Fatal(err)
	}
	config.Now = func() time.Time { return now.Add(time.Second) }
	config.Faults = DJIFaults{
		NackMethods:       map[string]int{"return_home": 314001},
		TimeoutMethods:    map[string]bool{"cover_open": true},
		UnknownCapability: "future.autonomy.execute",
	}
	transport := &protocolTransportFixture{}
	protocol, err := NewDJIProtocol(config, transport)
	if err != nil {
		t.Fatal(err)
	}
	nack := []byte(`{"tid":"nack-1","bid":"faults","timestamp":1787821202000,"method":"return_home","data":{}}`)
	if err := protocol.HandleMessage(context.Background(), protocol.ServiceTopic(), nack); err != nil {
		t.Fatal(err)
	}
	timeout := []byte(`{"tid":"timeout-1","bid":"faults","timestamp":1787821202001,"method":"cover_open","data":{}}`)
	if err := protocol.HandleMessage(context.Background(), protocol.ServiceTopic(), timeout); err != nil {
		t.Fatal(err)
	}
	if err := protocol.PublishTelemetry(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(transport.publications) != 3 {
		t.Fatalf("timeout emitted a reply or telemetry was lost: %+v", transport.publications)
	}
	if !strings.Contains(string(transport.publications[0].payload), `"result":314001`) {
		t.Fatalf("NACK result not injected: %s", transport.publications[0].payload)
	}
	if !strings.Contains(string(transport.publications[2].payload), "future.autonomy.execute") {
		t.Fatalf("unknown capability not injected: %s", transport.publications[2].payload)
	}
	if !strings.Contains(string(config.Topology), "99.99.99.99") {
		t.Fatal("unknown firmware not injected into topology")
	}
}

func TestDJIProtocolPackageHasNoDatabaseOrInternalSuccessBypass(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(source)
		for _, forbidden := range []string{"database/sql", "internal/database", "command.ack", "pretend.success"} {
			if strings.Contains(text, forbidden) && !strings.HasSuffix(file, "_test.go") {
				t.Fatalf("%s contains forbidden simulator bypass %q", file, forbidden)
			}
		}
	}
}

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

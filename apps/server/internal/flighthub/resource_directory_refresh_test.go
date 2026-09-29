package flighthub

import (
	"aerosight/server/internal/connector"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

type resourceDirectoryClient struct {
	*resourceClientFixture
	online bool
}

func (c *resourceDirectoryClient) ListDevices(context.Context, string, string) ([]Topology, error) {
	return []Topology{{Drone: &Device{SN: "AIRCRAFT_REDACTED", Online: c.online, CameraList: json.RawMessage(`[{"camera_index":"camera"}]`)}}}, nil
}
func TestDeviceStateRefreshesDirectoryAfterAircraftPowerOn(t *testing.T) {
	now := time.Now().UTC()
	store := &resourceStoreFixture{devices: []connector.ManagedConnectorDevice{{DeviceID: 1, TeamID: 2, Serial: "AIRCRAFT_REDACTED", Class: "drone", Online: false}}}
	client := &resourceDirectoryClient{resourceClientFixture: &resourceClientFixture{}, online: true}
	sink := &resourceSinkFixture{stateApplied: make(chan DeviceStatePoll, 2)}
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "token"}, store, sink, ResourceStreamConfig{OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: time.Minute, CatalogInterval: time.Minute, MaxBackoff: time.Minute, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	instance := resourceStreamInstance()
	coordinator.nextPoll[fmt.Sprintf("%d/1", instance.ID)] = now.Add(time.Minute)
	if _, _, err := coordinator.pollDeviceStates(context.Background(), instance, "token"); err != nil {
		t.Fatal(err)
	}
	select {
	case poll := <-sink.stateApplied:
		if !poll.Device.Online || len(poll.Snapshot.CameraList) == 0 {
			t.Fatal("fresh online presence was not used", poll)
		}
	default:
		t.Fatal("newly online aircraft remained deferred by offline timer")
	}
	client.online = false
	now = now.Add(16 * time.Second)
	if _, _, err := coordinator.pollDeviceStates(context.Background(), instance, "token"); err != nil {
		t.Fatal(err)
	}
	select {
	case poll := <-sink.stateApplied:
		if poll.Device.Online {
			t.Fatal("cached state marked powered-off aircraft online")
		}
	default:
		t.Fatal("offline state was not polled")
	}
}

package flighthub

import (
	"aerosight/server/internal/connector"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestInspectionWaylineVersionComparison(t *testing.T) {
	original := WaylineSummary{ID: "route", UpdatedAt: 1000, SizeBytes: 200}
	frozen, err := FreezeInspectionWayline(original)
	if err != nil {
		t.Fatal(err)
	}
	if err = VerifyInspectionWayline(frozen, original); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []WaylineSummary{{ID: "other", UpdatedAt: 1000, SizeBytes: 200}, {ID: "route", UpdatedAt: 2000, SizeBytes: 200}, {ID: "route", UpdatedAt: 1000, SizeBytes: 201}, {ID: "route", SizeBytes: 200}, {ID: "route", UpdatedAt: 1000}} {
		if VerifyInspectionWayline(frozen, changed) == nil {
			t.Fatal("changed or unknown version accepted", changed)
		}
	}
	frozen.RemoteVersion = "forged"
	if VerifyInspectionWayline(frozen, original) == nil {
		t.Fatal("forged version accepted")
	}
}

type inspectionWaylineReaderFixture struct {
	item           WaylineDetail
	err            error
	calls          int
	project, route string
}

func (f *inspectionWaylineReaderFixture) GetWayline(_ context.Context, _, project, route string) (WaylineDetail, error) {
	f.calls++
	f.project = project
	f.route = route
	return f.item, f.err
}
func TestInspectionWaylineRemoteRevalidation(t *testing.T) {
	original := WaylineSummary{ID: "route", UpdatedAt: 1000, SizeBytes: 200}
	frozen, _ := FreezeInspectionWayline(original)
	instance := connector.Instance{DiscoveryScope: json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"fixture"}`)}
	client := &inspectionWaylineReaderFixture{item: WaylineDetail{WaylineSummary: original}}
	verify := func() error {
		return RevalidateInspectionWayline(context.Background(), client, controlSessionTokenResolverFixture{}, instance, frozen)
	}
	if err := verify(); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 || client.project != runtimeProjectUUID || client.route != "route" {
		t.Fatal("remote scope lost", client)
	}
	client.item.DownloadURL = "https://example.invalid/new-temporary-url"
	if err := verify(); err != nil {
		t.Fatal("temporary URL changed version", err)
	}
	client.item.UpdatedAt++
	if err := verify(); err == nil {
		t.Fatal("changed remote accepted")
	}
	client.err = errors.New("remote unavailable")
	if err := verify(); !errors.Is(err, client.err) {
		t.Fatal("remote error swallowed", err)
	}
	before := client.calls
	frozen.RemoteVersion = "forged"
	if err := verify(); err == nil || client.calls != before {
		t.Fatal("forged snapshot reached remote")
	}
}

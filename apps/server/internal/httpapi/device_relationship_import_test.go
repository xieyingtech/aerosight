package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"encoding/json"
	"testing"
)

func TestDiscoveryRelationshipRepairsChildImportedBeforeParent(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	adapter, child := f.device(t, team, pid)
	var parent int32
	if err := f.db.QueryRow(`insert into devices(project_id,name,type,status,device_type_id) select project_id,'Dock',type,'online',device_type_id from devices where id=$1 returning id`, child).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`insert into device_external_identities(project_id,team_id,adapter_id,device_id,external_device_id,discovery_status,identity_json) values($1,$2,$3,$4,'child','managed','{"parentExternalId":"parent"}')`, pid, team, adapter, child); err != nil {
		t.Fatal(err)
	}
	q := sqlcgen.New(f.db)
	args := sqlcgen.DiscoveryRelationshipParams{P1: int32(pid), P2: int32(team), P3: int32(child), P4: int64(adapter), P5: "parent", P6: json.RawMessage(`{"source":"review-onboarding"}`)}
	if err := q.DiscoveryRelationship(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`insert into device_external_identities(project_id,team_id,adapter_id,device_id,external_device_id,discovery_status,identity_json) values($1,$2,$3,$4,'parent','managed','{}')`, pid, team, adapter, parent); err != nil {
		t.Fatal(err)
	}
	args.P3 = parent
	args.P5 = ""
	for range 2 {
		if err := q.DiscoveryRelationship(context.Background(), args); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := f.db.QueryRow(`select count(*) from device_relationships where project_id=$1 and from_device_id=$2 and to_device_id=$3 and relation_type='contains' and valid_until is null`, pid, parent, child).Scan(&count); err != nil || count != 1 {
		t.Fatalf("missing or duplicate relationship: %d %v", count, err)
	}
	snapshot, err := f.server.readSnapshot(context.Background(), 1, int32(pid))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["deviceRelations"] == nil {
		t.Fatal("snapshot omitted device topology")
	}
}

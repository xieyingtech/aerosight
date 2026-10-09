package httpapi

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestInlineAssetReferencesStayOutOfPublicProjections(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	adapter, _ := f.device(t, team, pid)
	var resource, asset int
	if err := f.db.QueryRow(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id) values($1,$2,$3,'flight-media','private-locator-marker') returning id`, pid, team, adapter).Scan(&resource); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`insert into assets(project_id,team_id,kind,mime_type,storage_key,logical_key,captured_at,metadata_json,remote_connector_id,remote_resource_id,remote_access_kind,remote_reference_digest,remote_credential_envelope_json,remote_reference_created_at,remote_reference_updated_at) values($1,$2,'image','image/jpeg','public-image.jpg','public-image',$5,'{"name":"public-image","longitude":120,"latitude":30}',$3,$4,'flight-media',repeat('a',64),'{"ciphertext":"private-envelope-marker"}',now(),now()) returning id`, pid, team, adapter, resource, time.Now().UTC().Add(-time.Minute)).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	// Also exercise an explicitly non-UTC window regardless of the host timezone.
	window := time.Now().In(time.FixedZone("fixture UTC+8", 8*60*60))
	offsetReplay := fmt.Sprintf("/api/projects/%d/replay?from=%s&to=%s", pid, url.QueryEscape(window.Add(-30*time.Minute).Format(time.RFC3339)), url.QueryEscape(window.Add(time.Minute).Format(time.RFC3339)))
	for _, path := range []string{fmt.Sprintf("/api/projects/%d/assets", pid), fmt.Sprintf("/api/projects/%d/snapshot", pid), fmt.Sprintf("/api/projects/%d/replay", pid), offsetReplay} {
		res := f.request(t, "GET", path, "")
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("%s: %d %s / %v", path, res.StatusCode, body, err)
		}
		for _, marker := range []string{"private-envelope-marker", "private-locator-marker", strings.Repeat("a", 64), "remote_credential", "credentialEnvelope", "referenceDigest", "remoteConnectorId"} {
			if strings.Contains(string(body), marker) {
				t.Fatalf("%s exposed private field %q", path, marker)
			}
		}
		// Ensure the tested projection actually includes the fixture asset.
		if !strings.Contains(string(body), "public-image") {
			t.Fatalf("%s did not include fixture: %s", path, body)
		}
	}
}

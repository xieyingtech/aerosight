package algorithm

import (
	"database/sql"
	"testing"
	"time"
)

type fixedAssetIssuer struct{ url string }

func (issuer fixedAssetIssuer) IssueAssetURL(int, int, int, time.Time) (string, error) {
	return issuer.url, nil
}

func TestRepeatedMediaEventUsesOneStableTaskAssetVersionKey(t *testing.T) {
	asset := triggerAsset{ID: 41, ProjectID: 2, TeamID: 3, Version: 7, TaskRunID: sql.NullInt64{Int64: 19, Valid: true}}
	definition := triggerDefinition{VersionID: 11}
	first := triggerIdempotencyKey(asset, definition)
	second := triggerIdempotencyKey(asset, definition)
	if first != second {
		t.Fatalf("duplicate media event changed key: %s != %s", first, second)
	}
	seen := map[string]bool{}
	for _, key := range []string{first, second} {
		seen[key] = true
	}
	if len(seen) != 1 {
		t.Fatalf("duplicate event created %d effective runs", len(seen))
	}
	asset.Version++
	if triggerIdempotencyKey(asset, definition) == first {
		t.Fatal("new asset version reused previous run key")
	}
}

func TestTriggeredInputPinsAssetTaskAndDefinitionVersions(t *testing.T) {
	asset := triggerAsset{ID: 41, ProjectID: 2, TeamID: 3, Version: 7, TaskRunID: sql.NullInt64{Int64: 19, Valid: true},
		DeviceID: sql.NullInt64{Int64: 5, Valid: true}, Kind: "image", MIMEType: "image/jpeg", Checksum: string(make([]byte, 64)), CapturedAt: time.Unix(1_800_000_000, 0)}
	definition := triggerDefinition{VersionID: 11, ProviderType: "http-json", Model: "construction-v2", ExecutionMode: "synchronous", MappingVersion: "suspected-construction/v1"}
	expires := time.Unix(1_800_000_300, 0)
	input := buildTriggeredInput("00000000-0000-4000-8000-000000000001", asset, definition, "https://assets.example.test/signed", expires)
	if input.InputAsset.AssetID != 41 || input.InputAsset.Version != 7 || input.Definition.DefinitionVersionID != 11 || input.Context["taskRunId"] != int64(19) {
		t.Fatalf("trigger input lost lineage: %+v", input)
	}
}

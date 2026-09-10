package perception

import (
	"testing"
	"time"
)

func TestAdjacentFramesWithReliableOverlapJoinOneGroup(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	firstBounds := &Bounds{MinX: 120, MinY: 30, MaxX: 120.001, MaxY: 30.001}
	groups := ApplyGroupDecision(DetectionCandidate{ProjectID: 2, AssetID: 10, Label: "suspected-construction", LocationQuality: "estimated", CapturedAt: now, GeographicBounds: firstBounds}, nil, GroupDecision{CreateNew: true})
	secondBounds := &Bounds{MinX: 120.0001, MinY: 30.0001, MaxX: 120.0011, MaxY: 30.0011}
	decision, err := SelectDetectionGroup(DetectionCandidate{ProjectID: 2, AssetID: 11, Label: "suspected-construction", LocationQuality: "estimated", CapturedAt: now.Add(5 * time.Second), GeographicBounds: secondBounds}, groups, AggregationPolicy{TimeWindow: time.Minute, MinimumIoU: 0.2})
	if err != nil || decision.CreateNew || decision.GroupIndex != 0 {
		t.Fatalf("adjacent frame did not aggregate: %+v %v", decision, err)
	}
}

func TestSameTargetAcrossInspectionPeriodsCreatesNewGroup(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	bounds := &Bounds{MinX: 120, MinY: 30, MaxX: 120.001, MaxY: 30.001}
	groups := ApplyGroupDecision(DetectionCandidate{ProjectID: 2, AssetID: 10, Label: "suspected-construction", LocationQuality: "surveyed", CapturedAt: now, GeographicBounds: bounds}, nil, GroupDecision{CreateNew: true})
	decision, _ := SelectDetectionGroup(DetectionCandidate{ProjectID: 2, AssetID: 50, Label: "suspected-construction", LocationQuality: "surveyed", CapturedAt: now.Add(25 * time.Hour), GeographicBounds: bounds}, groups, AggregationPolicy{TimeWindow: time.Hour, MinimumIoU: 0.2})
	if !decision.CreateNew {
		t.Fatal("cross-period target rewrote an earlier inspection group")
	}
}

func TestLowQualityLocationDoesNotMergeAcrossAssets(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	groups := ApplyGroupDecision(DetectionCandidate{ProjectID: 2, AssetID: 10, Label: "suspected-construction", LocationQuality: "unavailable", CapturedAt: now, PixelBounds: Bounds{0, 0, 100, 100}}, nil, GroupDecision{CreateNew: true})
	decision, _ := SelectDetectionGroup(DetectionCandidate{ProjectID: 2, AssetID: 11, Label: "suspected-construction", LocationQuality: "low", CapturedAt: now.Add(time.Second), PixelBounds: Bounds{0, 0, 100, 100}}, groups, AggregationPolicy{TimeWindow: time.Minute})
	if !decision.CreateNew {
		t.Fatal("low-quality locations were guessed to be the same target across assets")
	}
	decision, _ = SelectDetectionGroup(DetectionCandidate{ProjectID: 2, AssetID: 10, Label: "suspected-construction", LocationQuality: "unavailable", CapturedAt: now.Add(time.Second), PixelBounds: Bounds{0, 0, 100, 100}}, groups, AggregationPolicy{TimeWindow: time.Minute})
	if decision.CreateNew {
		t.Fatal("duplicate image-level detection within one asset was not deduplicated")
	}
}

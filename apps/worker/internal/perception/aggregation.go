package perception

import (
	"errors"
	"math"
	"time"
)

type Bounds struct{ MinX, MinY, MaxX, MaxY float64 }

type DetectionCandidate struct {
	ID                     int64
	ProjectID, AssetID     int
	TaskRunID              *int64
	Label, LocationQuality string
	CapturedAt             time.Time
	GeographicBounds       *Bounds
	PixelBounds            Bounds
}

type DetectionGroup struct {
	ID                              int64
	ProjectID                       int
	Label, LocationQuality          string
	FirstDetectedAt, LastDetectedAt time.Time
	MemberCount                     int
	GeographicBounds                *Bounds
	AssetIDs                        map[int]bool
}

type AggregationPolicy struct {
	TimeWindow time.Duration
	MinimumIoU float64
}

type GroupDecision struct {
	GroupIndex int
	CreateNew  bool
	Reason     string
}

func SelectDetectionGroup(candidate DetectionCandidate, groups []DetectionGroup, policy AggregationPolicy) (GroupDecision, error) {
	if candidate.ProjectID <= 0 || candidate.AssetID <= 0 || candidate.Label == "" || candidate.CapturedAt.IsZero() {
		return GroupDecision{}, errors.New("invalid detection candidate")
	}
	if policy.TimeWindow <= 0 {
		policy.TimeWindow = time.Hour
	}
	if policy.MinimumIoU <= 0 {
		policy.MinimumIoU = 0.2
	}
	bestIndex, bestScore := -1, -1.0
	for index, group := range groups {
		if group.ProjectID != candidate.ProjectID || group.Label != candidate.Label {
			continue
		}
		delta := candidate.CapturedAt.Sub(group.LastDetectedAt)
		if delta < 0 {
			delta = -delta
		}
		if delta > policy.TimeWindow {
			continue
		}
		reliable := reliableLocation(candidate.LocationQuality) && reliableLocation(group.LocationQuality) && candidate.GeographicBounds != nil && group.GeographicBounds != nil
		if reliable {
			score := intersectionOverUnion(*candidate.GeographicBounds, *group.GeographicBounds)
			if score >= policy.MinimumIoU && score > bestScore {
				bestIndex, bestScore = index, score
			}
			continue
		}
		// Image-only evidence may be deduplicated within one source asset, never guessed across assets.
		if group.AssetIDs[candidate.AssetID] {
			score := intersectionOverUnion(candidate.PixelBounds, candidate.PixelBounds)
			if score > bestScore {
				bestIndex, bestScore = index, score
			}
		}
	}
	if bestIndex < 0 {
		return GroupDecision{GroupIndex: -1, CreateNew: true, Reason: "no compatible group in spatial and temporal window"}, nil
	}
	return GroupDecision{GroupIndex: bestIndex, Reason: "matched location quality, time window, and overlap policy"}, nil
}

func ApplyGroupDecision(candidate DetectionCandidate, groups []DetectionGroup, decision GroupDecision) []DetectionGroup {
	if decision.CreateNew {
		assets := map[int]bool{candidate.AssetID: true}
		return append(groups, DetectionGroup{ProjectID: candidate.ProjectID, Label: candidate.Label,
			LocationQuality: candidate.LocationQuality, FirstDetectedAt: candidate.CapturedAt, LastDetectedAt: candidate.CapturedAt,
			MemberCount: 1, GeographicBounds: candidate.GeographicBounds, AssetIDs: assets})
	}
	group := &groups[decision.GroupIndex]
	if candidate.CapturedAt.Before(group.FirstDetectedAt) {
		group.FirstDetectedAt = candidate.CapturedAt
	}
	if candidate.CapturedAt.After(group.LastDetectedAt) {
		group.LastDetectedAt = candidate.CapturedAt
	}
	group.MemberCount++
	if group.AssetIDs == nil {
		group.AssetIDs = map[int]bool{}
	}
	group.AssetIDs[candidate.AssetID] = true
	if group.GeographicBounds != nil && candidate.GeographicBounds != nil {
		merged := unionBounds(*group.GeographicBounds, *candidate.GeographicBounds)
		group.GeographicBounds = &merged
	}
	return groups
}

func reliableLocation(quality string) bool { return quality == "surveyed" || quality == "estimated" }

func intersectionOverUnion(a, b Bounds) float64 {
	intersectionWidth := math.Max(0, math.Min(a.MaxX, b.MaxX)-math.Max(a.MinX, b.MinX))
	intersectionHeight := math.Max(0, math.Min(a.MaxY, b.MaxY)-math.Max(a.MinY, b.MinY))
	intersection := intersectionWidth * intersectionHeight
	areaA := math.Max(0, a.MaxX-a.MinX) * math.Max(0, a.MaxY-a.MinY)
	areaB := math.Max(0, b.MaxX-b.MinX) * math.Max(0, b.MaxY-b.MinY)
	union := areaA + areaB - intersection
	if union <= 0 {
		return 0
	}
	return intersection / union
}

func unionBounds(a, b Bounds) Bounds {
	return Bounds{MinX: math.Min(a.MinX, b.MinX), MinY: math.Min(a.MinY, b.MinY), MaxX: math.Max(a.MaxX, b.MaxX), MaxY: math.Max(a.MaxY, b.MaxY)}
}

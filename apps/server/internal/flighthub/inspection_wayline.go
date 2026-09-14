package flighthub

import (
	"aerosight/server/internal/connector"
	"context"
	"errors"
	"fmt"
	"strings"
)

// InspectionWaylineVersion uses the documented update timestamp and size,
// matching the catalogue's version convention. Temporary download URLs are
// deliberately excluded. This is a comparable remote version, not a file hash.
type InspectionWaylineVersion struct {
	WaylineID     string `json:"waylineId"`
	UpdatedAt     int64  `json:"updatedAt"`
	SizeBytes     int64  `json:"sizeBytes"`
	RemoteVersion string `json:"remoteVersion"`
}

func FreezeInspectionWayline(item WaylineSummary) (InspectionWaylineVersion, error) {
	if strings.TrimSpace(item.ID) == "" || item.UpdatedAt <= 0 || item.SizeBytes <= 0 {
		return InspectionWaylineVersion{}, errors.New("INSPECTION_WAYLINE_VERSION_UNVERIFIABLE")
	}
	return InspectionWaylineVersion{WaylineID: item.ID, UpdatedAt: item.UpdatedAt, SizeBytes: item.SizeBytes, RemoteVersion: fmt.Sprintf("%d:%d", item.UpdatedAt, item.SizeBytes)}, nil
}

func VerifyInspectionWayline(frozen InspectionWaylineVersion, current WaylineSummary) error {
	expected, err := FreezeInspectionWayline(WaylineSummary{ID: frozen.WaylineID, UpdatedAt: frozen.UpdatedAt, SizeBytes: frozen.SizeBytes})
	if err != nil || expected.RemoteVersion != frozen.RemoteVersion {
		return errors.New("INSPECTION_WAYLINE_VERSION_UNVERIFIABLE")
	}
	actual, err := FreezeInspectionWayline(current)
	if err != nil {
		return err
	}
	if actual != frozen {
		return errors.New("INSPECTION_WAYLINE_VERSION_CHANGED")
	}
	return nil
}

// InspectionWaylineReader performs only a fresh remote read. The caller must
// load the instance under the same project/team authorization as the Run.
type InspectionWaylineReader interface {
	GetWayline(context.Context, string, string, string) (WaylineDetail, error)
}

func RevalidateInspectionWayline(ctx context.Context, client InspectionWaylineReader, resolver TokenResolver, instance connector.Instance, frozen InspectionWaylineVersion) error {
	// Reject malformed snapshots before resolving credentials or making requests.
	if err := VerifyInspectionWayline(frozen, WaylineSummary{ID: frozen.WaylineID, UpdatedAt: frozen.UpdatedAt, SizeBytes: frozen.SizeBytes}); err != nil {
		return err
	}
	if client == nil || resolver == nil {
		return errors.New("INSPECTION_WAYLINE_READER_UNAVAILABLE")
	}
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return err
	}
	token, err := resolver.ResolveToken(ctx, instance)
	if err != nil {
		return err
	}
	current, err := client.GetWayline(ctx, token, scope.ProjectUUID, frozen.WaylineID)
	if err != nil {
		return err
	}
	return VerifyInspectionWayline(frozen, current.WaylineSummary)
}

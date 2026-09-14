package flighthub

import (
	"context"
	"database/sql"
	"errors"

	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/connector"
)

// ReadAlgorithmAsset never locks a business Run: a synchronous Provider can
// request these bytes while its dispatch transaction holds that Run's lock.
// Remote failures are handled failures and must not fall back to local storage.
func (service *FlightAssetAccessService) ReadAlgorithmAsset(ctx context.Context, projectID, assetID, version int) (algorithm.AlgorithmAsset, bool, error) {
	var result algorithm.AlgorithmAsset
	var exists bool
	if err := service.db.QueryRowContext(ctx, `select exists(select 1 from connector_asset_access_refs where project_id=$1 and id=$2)`, projectID, assetID).Scan(&exists); err != nil {
		return result, true, err
	}
	if !exists {
		return result, false, nil
	}
	var connectorID int64
	var flightID, objectVersion, mime string
	err := service.db.QueryRowContext(ctx, `select reference.connector_instance_id,flight.remote_id,asset.object_version,coalesce(asset.mime_type,'')
 from connector_asset_access_refs reference
 join assets asset on asset.id=reference.id and asset.project_id=reference.project_id and asset.team_id=reference.team_id
 join device_adapters adapter on adapter.id=reference.connector_instance_id and adapter.project_id=reference.project_id and adapter.team_id=reference.team_id
 join connector_remote_resources media on media.id=reference.remote_resource_id and media.project_id=reference.project_id and media.connector_instance_id=reference.connector_instance_id and media.resource_kind='flight-media' and media.status='active'
 join connector_remote_resources flight on flight.project_id=reference.project_id and flight.connector_instance_id=reference.connector_instance_id and flight.resource_kind='flight-task' and flight.status='active' and flight.canonical_target_type='task_run' and flight.canonical_target_id=asset.task_run_id::text
 where reference.project_id=$1 and reference.id=$2 and asset.version=$3 and asset.status='available' and asset.deleted_at is null and asset.kind='image'
 and reference.access_kind='flight-media' and adapter.adapter_type='dji-flighthub2' and adapter.status in('connecting','connected','degraded')`, projectID, assetID, version).Scan(&connectorID, &flightID, &objectVersion, &mime)
	if errors.Is(err, sql.ErrNoRows) {
		err = connector.ErrRemoteResourceUnavailable
	}
	if err != nil {
		return result, true, err
	}
	reader, ok := service.client.(interface {
		ReadInspectionMedia(context.Context, TemporaryDownload, int64) ([]byte, error)
	})
	if !ok {
		return result, true, errors.New("INSPECTION_MEDIA_READER_UNAVAILABLE")
	}
	instance := connector.Instance{ID: connectorID, ProjectID: projectID}
	for attempt := 0; attempt < 2; attempt++ {
		var download TemporaryDownload
		download, err = service.RefreshInspectionVersionDownload(ctx, instance, assetID, flightID, objectVersion)
		if err == nil {
			result.Body, err = reader.ReadInspectionMedia(ctx, download, 64<<20)
		}
		if !IsSafeCode(err, "temporary_link_expired") {
			break
		}
	}
	if err != nil {
		return algorithm.AlgorithmAsset{}, true, err
	}
	result.ContentType = mime
	return result, true, nil
}

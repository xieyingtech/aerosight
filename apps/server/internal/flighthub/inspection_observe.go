package flighthub

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"aerosight/server/internal/connector"
	"aerosight/server/internal/inspection"
	"aerosight/server/internal/mission"
	"github.com/google/uuid"
)

type InspectionFlightClient interface {
	GetFlightTask(context.Context, string, string, string) (FlightTask, error)
	ListFlightTaskMedia(context.Context, string, string, string) ([]FlightTaskMedia, error)
	HashInspectionMedia(context.Context, TemporaryDownload, int64) (string, error)
}

type InspectionFlightObserver struct {
	client   InspectionFlightClient
	access   *FlightAssetAccessService
	resolver TokenResolver
}

func NewInspectionFlightObserver(client InspectionFlightClient, access *FlightAssetAccessService, resolver TokenResolver) *InspectionFlightObserver {
	return &InspectionFlightObserver{client: client, access: access, resolver: resolver}
}

func (service *InspectionFlightObserver) Observe(ctx context.Context, tx *sql.Tx, step mission.PreparedStep, input inspection.ObserveInput) (inspection.Observation, error) {
	var observation inspection.Observation
	fail := func(code string) (inspection.Observation, error) { return observation, errors.New(code) }
	if input.ConnectorID <= 0 || strings.TrimSpace(input.FlightUUID) == "" {
		return fail("INSPECTION_FLIGHT_INPUT_INVALID")
	}
	if input.MaxImages == 0 {
		input.MaxImages = 64
	}
	if input.MaxImages < 1 || input.MaxImages > 1000 || len(input.AssetIDs) > input.MaxImages {
		return fail("INSPECTION_ASSET_LIMIT_INVALID")
	}
	if input.ConfirmLimitedScope && (len(input.AssetIDs) == 0 || strings.TrimSpace(input.ScopeDescription) == "") {
		return fail("INSPECTION_FINITE_SCOPE_REQUIRED")
	}
	if service.client == nil || service.access == nil || service.resolver == nil {
		return fail("INSPECTION_FLIGHT_READER_UNAVAILABLE")
	}
	// This is the same lock as the projector and alert policy. No projected Run
	// is changed; the canonical target is only used as source provenance.
	if _, err := inspection.LockAlertConnector(ctx, tx, step.ProjectID, input.ConnectorID); err != nil {
		return observation, err
	}
	instance := connector.Instance{ID: input.ConnectorID, ProjectID: step.ProjectID}
	var projectedRun int64
	err := tx.QueryRowContext(ctx, `select coalesce(adapter.credential_envelope_json,'{}'::jsonb),adapter.discovery_scope_json,run.id
 from device_adapters adapter join connector_remote_resources resource
 on resource.project_id=adapter.project_id and resource.connector_instance_id=adapter.id and resource.resource_kind='flight-task' and resource.remote_id=$4 and resource.status='active'
 join task_runs run on run.project_id=resource.project_id and resource.canonical_target_type='task_run' and resource.canonical_target_id=run.id::text
 where adapter.project_id=$1 and adapter.team_id=$2 and adapter.id=$3 and adapter.status in('connecting','connected','degraded') and run.id<>$5`, step.ProjectID, step.TeamID, input.ConnectorID, input.FlightUUID, step.RunID).Scan(&instance.CredentialEnvelope, &instance.DiscoveryScope, &projectedRun)
	if errors.Is(err, sql.ErrNoRows) {
		return fail("INSPECTION_FLIGHT_SCOPE_INVALID")
	}
	if err != nil {
		return observation, err
	}
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return observation, err
	}
	token, err := service.resolver.ResolveToken(ctx, instance)
	if err != nil {
		return observation, err
	}
	task, err := service.client.GetFlightTask(ctx, token, scope.ProjectUUID, input.FlightUUID)
	if err != nil {
		return observation, err
	}
	if task.UUID != input.FlightUUID {
		return fail("INSPECTION_FLIGHT_SCOPE_INVALID")
	}
	if task.Status != "success" && task.Status != "failed" && task.Status != "canceled" {
		return fail("INSPECTION_FLIGHT_NOT_FINISHED")
	}
	from, err := time.Parse(time.RFC3339Nano, task.BeginAt)
	if err != nil {
		return fail("INSPECTION_FLIGHT_TIME_INVALID")
	}
	to, err := time.Parse(time.RFC3339Nano, task.EndAt)
	if err != nil || to.Before(from) {
		return fail("INSPECTION_FLIGHT_TIME_INVALID")
	}
	media, err := service.client.ListFlightTaskMedia(ctx, token, scope.ProjectUUID, input.FlightUUID)
	token = ""
	if err != nil {
		return observation, err
	} // A truncated list is never silently used.
	localScope := inspection.Scope{ProjectID: step.ProjectID, TeamID: step.TeamID}
	flight := &inspection.FlightRef{Scope: localScope, ConnectorID: input.ConnectorID, FlightUUID: input.FlightUUID, ProjectedRunID: &projectedRun}
	observation = inspection.Observation{ID: uuid.NewString(), ContractVersion: inspection.ContractVersion, Run: inspection.RunRef{Scope: localScope, RunID: int64(step.RunID), StepID: step.StepID}, Mode: inspection.ExistingFlight, Flight: flight, Assets: []inspection.AssetRef{}, ObservedFrom: from.UTC(), ObservedTo: to.UTC(), ScopeDescription: input.ScopeDescription, DataGaps: []string{}}
	if observation.ScopeDescription == "" {
		observation.ScopeDescription = "司空已完成飞行的图片清单"
	}
	selected := map[int64]bool{}
	for _, assetID := range input.AssetIDs {
		if assetID <= 0 || selected[assetID] {
			return fail("INSPECTION_ASSET_SELECTION_INVALID")
		}
		selected[assetID] = true
	}
	seen := map[string]FlightTaskMedia{}
	accessible := 0
	for _, item := range media {
		if previous, duplicate := seen[item.UUID]; duplicate {
			if inspectionMediaVersion(previous) != inspectionMediaVersion(item) {
				return fail("INSPECTION_MEDIA_DUPLICATE")
			}
			continue
		}
		seen[item.UUID] = item
		if item.FileType != "image" {
			observation.DataGaps = append(observation.DataGaps, "non_image_media_not_analyzed")
			continue
		}
		var assetID int64
		var version int
		var objectVersion string
		err = tx.QueryRowContext(ctx, `select asset.id,asset.version,coalesce(asset.object_version,'') from connector_remote_resources resource
 join connector_asset_access_refs reference on reference.project_id=resource.project_id and reference.remote_resource_id=resource.id and reference.connector_instance_id=resource.connector_instance_id
 join assets asset on asset.id=reference.id and asset.project_id=reference.project_id
 where resource.project_id=$1 and resource.connector_instance_id=$2 and resource.resource_kind='flight-media' and resource.remote_id=$3 and resource.status='active'
 and asset.team_id=$4 and asset.task_run_id=$5 and asset.kind='image' and asset.status='available' for share of asset`, step.ProjectID, input.ConnectorID, item.UUID, step.TeamID, projectedRun).Scan(&assetID, &version, &objectVersion)
		if errors.Is(err, sql.ErrNoRows) {
			observation.DataGaps = append(observation.DataGaps, "media_not_projected")
			continue
		}
		if err != nil {
			return observation, err
		}
		if len(selected) > 0 && !selected[assetID] {
			continue
		}
		if len(observation.Assets) >= input.MaxImages {
			return fail("INSPECTION_ASSET_LIMIT_INVALID")
		}
		remoteVersion := inspectionMediaVersion(item)
		if objectVersion != remoteVersion {
			return fail("INSPECTION_MEDIA_VERSION_CHANGED")
		}
		var digest string
		for attempt := 0; attempt < 2; attempt++ {
			download, readErr := service.access.RefreshInspectionVersionDownload(ctx, instance, int(assetID), input.FlightUUID, objectVersion)
			err = readErr
			if err == nil {
				digest, err = service.client.HashInspectionMedia(ctx, download, 64<<20)
			}
			if !IsSafeCode(err, "temporary_link_expired") {
				break
			}
		}
		if IsSafeCode(err, "media_version_changed") {
			return fail("INSPECTION_MEDIA_VERSION_CHANGED")
		}
		if err != nil {
			observation.DataGaps = append(observation.DataGaps, "media_unreadable")
			continue
		}
		accessible++
		observation.Assets = append(observation.Assets, inspection.AssetRef{Scope: localScope, AssetID: assetID, Version: version, ObjectVersion: objectVersion, ChecksumSHA256: digest, SourceRunID: &projectedRun, Flight: flight})
	}
	if len(selected) > 0 {
		for _, asset := range observation.Assets {
			delete(selected, asset.AssetID)
		}
		if len(selected) > 0 {
			return fail("INSPECTION_SELECTED_MEDIA_UNAVAILABLE_OR_WRONG_FLIGHT")
		}
	}
	status := InspectionMediaStatus(task, len(seen), accessible, false)
	if !status.CountsComparable {
		observation.DataGaps = append(observation.DataGaps, "media_counts_unknown")
	}
	if status.Expected != nil && status.Uploaded != nil && (*status.Expected != *status.Uploaded || *status.Expected != status.Listed) {
		observation.DataGaps = append(observation.DataGaps, "media_count_mismatch")
	}
	if accessible != len(seen) {
		observation.DataGaps = append(observation.DataGaps, "selected_scope_does_not_cover_all_media")
	}
	observation.MediaStatus = &status
	observation.Completeness = status.Completeness()
	if observation.Completeness != inspection.Complete {
		if !input.ConfirmLimitedScope {
			return fail("INSPECTION_MEDIA_INCOMPLETE_CONFIRM_FINITE_SCOPE")
		}
		userID := int64(step.UserID)
		observation.LimitedScopeConfirmedBy = &userID
		observation.Completeness = inspection.Partial
	}
	if err = inspection.ClaimAlertFlight(ctx, tx, step.ProjectID, input.ConnectorID, input.FlightUUID, int64(step.RunID)); err != nil {
		return observation, err
	}
	return observation, observation.Validate()
}

// ReadAsset resolves a selected remote image through its original flight. It
// does not switch the observation into full-flight mode or change asset ownership.
func (service *InspectionFlightObserver) ReadAsset(ctx context.Context, tx *sql.Tx, step mission.PreparedStep, asset inspection.AssetRef) (string, *inspection.FlightRef, error) {
	if service == nil || service.access == nil || service.client == nil || asset.ObjectVersion == "" || asset.SourceRunID == nil {
		return "", nil, errors.New("INSPECTION_REMOTE_ASSET_SOURCE_INVALID")
	}
	var connectorID int64
	var flightID string
	err := tx.QueryRowContext(ctx, `select reference.connector_instance_id,flight.remote_id from connector_asset_access_refs reference
 join device_adapters adapter on adapter.id=reference.connector_instance_id and adapter.project_id=reference.project_id and adapter.team_id=reference.team_id
 join connector_remote_resources media on media.id=reference.remote_resource_id and media.project_id=reference.project_id and media.connector_instance_id=reference.connector_instance_id and media.resource_kind='flight-media' and media.status='active'
 join connector_remote_resources flight on flight.project_id=reference.project_id and flight.connector_instance_id=reference.connector_instance_id and flight.resource_kind='flight-task' and flight.status='active' and flight.canonical_target_type='task_run' and flight.canonical_target_id=$4
 where reference.project_id=$1 and reference.team_id=$2 and reference.id=$3 and reference.access_kind='flight-media' and adapter.adapter_type='dji-flighthub2' and adapter.status in('connecting','connected','degraded')`, step.ProjectID, step.TeamID, asset.AssetID, strconv.FormatInt(*asset.SourceRunID, 10)).Scan(&connectorID, &flightID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, errors.New("INSPECTION_REMOTE_ASSET_SOURCE_INVALID")
	}
	if err != nil {
		return "", nil, err
	}
	instance := connector.Instance{ID: connectorID, ProjectID: step.ProjectID}
	for attempt := 0; attempt < 2; attempt++ {
		download, readErr := service.access.RefreshInspectionVersionDownload(ctx, instance, int(asset.AssetID), flightID, asset.ObjectVersion)
		err = readErr
		var digest string
		if err == nil {
			digest, err = service.client.HashInspectionMedia(ctx, download, 64<<20)
		}
		if err == nil {
			return digest, &inspection.FlightRef{Scope: asset.Scope, ConnectorID: connectorID, FlightUUID: flightID, ProjectedRunID: asset.SourceRunID}, nil
		}
		if !IsSafeCode(err, "temporary_link_expired") {
			break
		}
	}
	if IsSafeCode(err, "media_version_changed") {
		return "", nil, errors.New("INSPECTION_MEDIA_VERSION_CHANGED")
	}
	return "", nil, errors.New("INSPECTION_REMOTE_ASSET_UNREADABLE")
}

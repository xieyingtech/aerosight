package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
)

type FlightAssetURLClient interface {
	RefreshFlightTaskMediaURL(context.Context, string, string, string, string) (TemporaryDownload, error)
	GetFlightRecordDownloadURL(context.Context, string, string, string) (TemporaryDownload, error)
}

type FlightAssetAccessService struct {
	db         *sql.DB
	client     FlightAssetURLClient
	resolver   TokenResolver
	authSecret string
	now        func() time.Time
}

func NewFlightAssetAccessService(database *sql.DB, client FlightAssetURLClient, resolver TokenResolver, authSecret string, now func() time.Time) (*FlightAssetAccessService, error) {
	if database == nil || client == nil || resolver == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub asset access dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	return &FlightAssetAccessService{db: database, client: client, resolver: resolver, authSecret: authSecret, now: now}, nil
}

func (service *FlightAssetAccessService) RefreshDownload(ctx context.Context, instance connector.Instance, assetID int) (TemporaryDownload, error) {
	if service == nil || service.db == nil || instance.ID <= 0 || instance.ProjectID <= 0 || assetID <= 0 {
		return TemporaryDownload{}, connector.ErrRemoteResourceUnavailable
	}
	var accessKind string
	var referenceRaw, connectorCredentialRaw, discoveryScopeRaw []byte
	err := service.db.QueryRowContext(ctx, `select reference.access_kind,reference.credential_envelope_json,
		adapter.credential_envelope_json,adapter.discovery_scope_json
	 from connector_asset_access_refs reference
	 join assets asset on asset.id=reference.id and asset.project_id=reference.project_id and asset.status='available'
	 join device_adapters adapter on adapter.id=reference.connector_instance_id and adapter.project_id=reference.project_id
	 where reference.project_id=$1 and reference.connector_instance_id=$2 and reference.id=$3
	   and adapter.status in('connecting','connected','degraded')`, instance.ProjectID, instance.ID, assetID).
		Scan(&accessKind, &referenceRaw, &connectorCredentialRaw, &discoveryScopeRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return TemporaryDownload{}, connector.ErrRemoteResourceUnavailable
	}
	if err != nil {
		return TemporaryDownload{}, err
	}
	instance.CredentialEnvelope = json.RawMessage(connectorCredentialRaw)
	instance.DiscoveryScope = json.RawMessage(discoveryScopeRaw)
	referenceEnvelope, err := credentials.ParseEnvelope(referenceRaw)
	if err != nil {
		return TemporaryDownload{}, errors.New("FlightHub asset reference is unavailable")
	}
	locator := map[string]string{}
	if err := credentials.DecryptJSON(referenceEnvelope, service.authSecret,
		credentials.AAD("flighthub-asset-reference", assetID, instance.ProjectID), &locator); err != nil {
		return TemporaryDownload{}, errors.New("FlightHub asset reference is unavailable")
	}
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return TemporaryDownload{}, err
	}
	token, err := service.resolver.ResolveToken(ctx, instance)
	if err != nil {
		return TemporaryDownload{}, err
	}
	defer func() { token = "" }()
	refresh := func() (TemporaryDownload, error) {
		switch accessKind {
		case "flight-media":
			if locator["taskUUID"] == "" || locator["mediaUUID"] == "" {
				return TemporaryDownload{}, errors.New("FlightHub asset reference is unavailable")
			}
			return service.client.RefreshFlightTaskMediaURL(ctx, token, scope.ProjectUUID, locator["taskUUID"], locator["mediaUUID"])
		case "flight-record":
			if locator["objectKey"] == "" {
				return TemporaryDownload{}, errors.New("FlightHub asset reference is unavailable")
			}
			return service.client.GetFlightRecordDownloadURL(ctx, token, scope.ProjectUUID, locator["objectKey"])
		default:
			return TemporaryDownload{}, errors.New("FlightHub asset reference is unavailable")
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		download, refreshErr := refresh()
		if refreshErr == nil {
			if download.URL == "" || !download.ExpiresAt.After(service.now().UTC()) {
				refreshErr = &APIError{SafeCode: "temporary_link_expired"}
			} else {
				return download, nil
			}
		}
		if !IsSafeCode(refreshErr, "temporary_link_expired") || attempt == 1 {
			return TemporaryDownload{}, refreshErr
		}
	}
	return TemporaryDownload{}, &APIError{SafeCode: "temporary_link_expired"}
}

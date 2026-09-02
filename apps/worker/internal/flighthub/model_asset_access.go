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

type ModelAssetDownloadClient interface {
	GetModelDownloadURL(context.Context, string, string, string) (ModelDownload, error)
}

type ModelAssetAccessService struct {
	db         *sql.DB
	client     ModelAssetDownloadClient
	resolver   TokenResolver
	authSecret string
	now        func() time.Time
}

func NewModelAssetAccessService(database *sql.DB, client ModelAssetDownloadClient, resolver TokenResolver, authSecret string, now func() time.Time) (*ModelAssetAccessService, error) {
	if database == nil || client == nil || resolver == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub model asset access dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	return &ModelAssetAccessService{db: database, client: client, resolver: resolver, authSecret: authSecret, now: now}, nil
}

func (service *ModelAssetAccessService) RefreshDownload(ctx context.Context, instance connector.Instance, assetID int) (ModelDownload, error) {
	if service == nil || service.db == nil || instance.ID <= 0 || instance.ProjectID <= 0 || assetID <= 0 {
		return ModelDownload{}, connector.ErrRemoteResourceUnavailable
	}
	var referenceRaw, connectorCredentialRaw, discoveryScopeRaw []byte
	err := service.db.QueryRowContext(ctx, `select reference.credential_envelope_json,
		adapter.credential_envelope_json,adapter.discovery_scope_json
	 from connector_asset_access_refs reference
	 join assets asset on asset.id=reference.id and asset.project_id=reference.project_id and asset.status='available'
	 join device_adapters adapter on adapter.id=reference.connector_instance_id and adapter.project_id=reference.project_id
	 where reference.project_id=$1 and reference.connector_instance_id=$2 and reference.id=$3
	   and reference.access_kind='model' and adapter.status in('connecting','connected','degraded')`,
		instance.ProjectID, instance.ID, assetID).Scan(&referenceRaw, &connectorCredentialRaw, &discoveryScopeRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelDownload{}, connector.ErrRemoteResourceUnavailable
	}
	if err != nil {
		return ModelDownload{}, err
	}
	instance.CredentialEnvelope = json.RawMessage(connectorCredentialRaw)
	instance.DiscoveryScope = json.RawMessage(discoveryScopeRaw)
	referenceEnvelope, err := credentials.ParseEnvelope(referenceRaw)
	if err != nil {
		return ModelDownload{}, errors.New("FlightHub model asset reference is unavailable")
	}
	locator := map[string]string{}
	if err := credentials.DecryptJSON(referenceEnvelope, service.authSecret,
		credentials.AAD("flighthub-asset-reference", assetID, instance.ProjectID), &locator); err != nil {
		return ModelDownload{}, errors.New("FlightHub model asset reference is unavailable")
	}
	if locator["fileID"] == "" {
		return ModelDownload{}, errors.New("FlightHub model asset reference is unavailable")
	}
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return ModelDownload{}, err
	}
	token, err := service.resolver.ResolveToken(ctx, instance)
	if err != nil {
		return ModelDownload{}, err
	}
	defer func() { token = "" }()
	for attempt := 0; attempt < 2; attempt++ {
		download, refreshErr := service.client.GetModelDownloadURL(ctx, token, scope.ProjectUUID, locator["fileID"])
		if refreshErr == nil {
			if !download.Ready || download.URL == "" {
				return ModelDownload{}, connector.ErrRemoteResourceUnavailable
			}
			if download.ExpiresAt.After(service.now().UTC()) {
				return download, nil
			}
			refreshErr = &APIError{SafeCode: "temporary_link_expired"}
		}
		if !IsSafeCode(refreshErr, "temporary_link_expired") || attempt == 1 {
			return ModelDownload{}, refreshErr
		}
	}
	return ModelDownload{}, &APIError{SafeCode: "temporary_link_expired"}
}

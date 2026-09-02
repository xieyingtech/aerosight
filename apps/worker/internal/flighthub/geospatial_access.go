package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
)

type GeospatialDownloadKind string

const (
	GeospatialFlightAreaFile GeospatialDownloadKind = "flight-area-file"
	GeospatialOfflineMap     GeospatialDownloadKind = "offline-map"
)

type GeospatialFileClient interface {
	GetProjectFlightAreaFile(context.Context, string, string) (GeospatialFileDownload, error)
	GetWorkspaceOfflineMapDownload(context.Context, string, string) (OfflineMapDownload, error)
}

type GeospatialAccessObservation struct {
	Kind     GeospatialDownloadKind
	SafeCode string
}

type GeospatialAccessService struct {
	client   GeospatialFileClient
	resolver TokenResolver
	load     func(context.Context, connector.Instance) (connector.Instance, error)
	now      func() time.Time
	observe  func(GeospatialAccessObservation)
}

func NewGeospatialAccessService(database *sql.DB, client GeospatialFileClient, resolver TokenResolver, now func() time.Time) (*GeospatialAccessService, error) {
	if database == nil || client == nil || resolver == nil {
		return nil, errors.New("FlightHub geospatial access dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	service := &GeospatialAccessService{client: client, resolver: resolver, now: now, observe: func(GeospatialAccessObservation) {}}
	service.load = func(ctx context.Context, requested connector.Instance) (connector.Instance, error) {
		if requested.ID <= 0 || requested.ProjectID <= 0 {
			return connector.Instance{}, connector.ErrRemoteResourceUnavailable
		}
		var credentialRaw, scopeRaw []byte
		err := database.QueryRowContext(ctx, `select adapter.credential_envelope_json,adapter.discovery_scope_json
			from device_adapters adapter
			join connector_definitions definition on definition.id=adapter.connector_definition_id
			where adapter.id=$1 and adapter.project_id=$2 and adapter.status in('connecting','connected','degraded')
			  and definition.connector_key='dji.flighthub2'`, requested.ID, requested.ProjectID).Scan(&credentialRaw, &scopeRaw)
		if errors.Is(err, sql.ErrNoRows) {
			return connector.Instance{}, connector.ErrRemoteResourceUnavailable
		}
		if err != nil {
			return connector.Instance{}, err
		}
		requested.CredentialEnvelope = json.RawMessage(credentialRaw)
		requested.DiscoveryScope = json.RawMessage(scopeRaw)
		return requested, nil
	}
	return service, nil
}

func validGeospatialDownloadKind(kind GeospatialDownloadKind) bool {
	return kind == GeospatialFlightAreaFile || kind == GeospatialOfflineMap
}

func (service *GeospatialAccessService) observeResult(kind GeospatialDownloadKind, err error) {
	if service == nil || service.observe == nil {
		return
	}
	code := "ok"
	if err != nil {
		code = resourceStreamErrorCode(err)
	}
	service.observe(GeospatialAccessObservation{Kind: kind, SafeCode: code})
}

func (service *GeospatialAccessService) RefreshDownload(ctx context.Context, requested connector.Instance, kind GeospatialDownloadKind) (file GeospatialFileDownload, returnedErr error) {
	defer func() { service.observeResult(kind, returnedErr) }()
	if service == nil || service.client == nil || service.resolver == nil || service.load == nil || service.now == nil ||
		requested.ID <= 0 || requested.ProjectID <= 0 || !validGeospatialDownloadKind(kind) {
		return GeospatialFileDownload{}, connector.ErrRemoteResourceUnavailable
	}
	instance, err := service.load(ctx, requested)
	if err != nil {
		return GeospatialFileDownload{}, err
	}
	if instance.ID != requested.ID || instance.ProjectID != requested.ProjectID {
		return GeospatialFileDownload{}, connector.ErrRemoteResourceUnavailable
	}
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return GeospatialFileDownload{}, err
	}
	token, err := service.resolver.ResolveToken(ctx, instance)
	if err != nil {
		return GeospatialFileDownload{}, err
	}
	defer func() { token = "" }()
	refresh := func() (GeospatialFileDownload, error) {
		switch kind {
		case GeospatialFlightAreaFile:
			return service.client.GetProjectFlightAreaFile(ctx, token, scope.ProjectUUID)
		case GeospatialOfflineMap:
			download, downloadErr := service.client.GetWorkspaceOfflineMapDownload(ctx, token, scope.ProjectUUID)
			if downloadErr != nil {
				return GeospatialFileDownload{}, downloadErr
			}
			if !download.Enabled || download.File == nil {
				return GeospatialFileDownload{}, &APIError{SafeCode: "resource_empty"}
			}
			return *download.File, nil
		default:
			return GeospatialFileDownload{}, connector.ErrRemoteResourceUnavailable
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		file, err = refresh()
		if err == nil {
			if strings.TrimSpace(file.URL) == "" || !file.ExpiresAt.After(service.now().UTC()) {
				err = &APIError{SafeCode: "temporary_link_expired"}
			} else {
				return file, nil
			}
		}
		if !IsSafeCode(err, "temporary_link_expired") || attempt == 1 {
			return GeospatialFileDownload{}, err
		}
	}
	return GeospatialFileDownload{}, &APIError{SafeCode: "temporary_link_expired"}
}

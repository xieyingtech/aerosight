package flighthub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/dji"
)

const (
	ConnectorKey     = "dji.flighthub2"
	ConnectorVersion = "1.0.0"
	ContractVersion  = "dji-flighthub-openapi-v2-cn-2026-08-30"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type DirectoryClient interface {
	ListDevices(context.Context, string, string) ([]Topology, error)
}

type TokenResolver interface {
	ResolveToken(context.Context, connector.Instance) (string, error)
}

type EncryptedTokenResolver struct{ AuthSecret string }

func (resolver EncryptedTokenResolver) ResolveToken(_ context.Context, instance connector.Instance) (string, error) {
	if instance.ID <= 0 || instance.ProjectID <= 0 || len(instance.CredentialEnvelope) == 0 {
		return "", errors.New("DJI_FLIGHTHUB_CREDENTIAL_UNAVAILABLE")
	}
	envelope, err := credentials.ParseEnvelope(instance.CredentialEnvelope)
	if err != nil {
		return "", errors.New("DJI_FLIGHTHUB_CREDENTIAL_UNAVAILABLE")
	}
	var payload map[string]json.RawMessage
	if err := credentials.DecryptJSON(envelope, resolver.AuthSecret,
		credentials.AAD("device-adapter", instance.ID, instance.ProjectID), &payload); err != nil {
		return "", errors.New("DJI_FLIGHTHUB_CREDENTIAL_UNAVAILABLE")
	}
	if len(payload) != 1 {
		return "", errors.New("DJI_FLIGHTHUB_CREDENTIAL_INVALID")
	}
	var token string
	if err := json.Unmarshal(payload["token"], &token); err != nil {
		return "", errors.New("DJI_FLIGHTHUB_CREDENTIAL_INVALID")
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 16_384 {
		return "", errors.New("DJI_FLIGHTHUB_CREDENTIAL_INVALID")
	}
	return token, nil
}

type discoveryScope struct {
	ProjectUUID        string `json:"projectUuid"`
	ProjectName        string `json:"projectName"`
	OrganizationUUID   string `json:"organizationUuid,omitempty"`
	AccountFingerprint string `json:"accountFingerprint,omitempty"`
}

func parseScope(raw json.RawMessage) (discoveryScope, error) {
	var scope discoveryScope
	if err := json.Unmarshal(raw, &scope); err != nil {
		return discoveryScope{}, errors.New("DJI_FLIGHTHUB_SCOPE_INVALID")
	}
	scope.ProjectUUID = strings.ToLower(strings.TrimSpace(scope.ProjectUUID))
	scope.ProjectName = strings.TrimSpace(scope.ProjectName)
	scope.OrganizationUUID = strings.ToLower(strings.TrimSpace(scope.OrganizationUUID))
	scope.AccountFingerprint = strings.TrimSpace(scope.AccountFingerprint)
	if !uuidPattern.MatchString(scope.ProjectUUID) || scope.ProjectName == "" {
		return discoveryScope{}, errors.New("DJI_FLIGHTHUB_SCOPE_INVALID")
	}
	if scope.OrganizationUUID != "" && !uuidPattern.MatchString(scope.OrganizationUUID) {
		return discoveryScope{}, errors.New("DJI_FLIGHTHUB_SCOPE_INVALID")
	}
	if scope.AccountFingerprint != "" && !validAccountFingerprint(scope.AccountFingerprint) {
		return discoveryScope{}, errors.New("DJI_FLIGHTHUB_SCOPE_INVALID")
	}
	return scope, nil
}

func RegisterRuntime(registry *connector.Registry, client DirectoryClient, resolver TokenResolver) error {
	if registry == nil || client == nil || resolver == nil {
		return errors.New("DJI_FLIGHTHUB_RUNTIME_DEPENDENCY_REQUIRED")
	}
	runtime := connector.Runtime{
		Manifest: connector.Manifest{
			ConnectorKey: ConnectorKey, Version: ConnectorVersion, DisplayName: "DJI FlightHub 2",
			ConfigSchema:     json.RawMessage(`{"type":"object","additionalProperties":false}`),
			CredentialSchema: json.RawMessage(`{"type":"object","required":["token"],"properties":{"token":{"type":"string"}}}`),
			DiscoveryModes:   []connector.DiscoveryMode{connector.DiscoveryPoll}, Protocols: []string{"https"},
			CompatibleDrivers: []string{"dji.cloud"},
			Capabilities:      Capabilities(),
			Lease:             connector.LeasePolicy{Duration: 60 * time.Second, RenewBefore: 20 * time.Second},
		},
		DiscoveryHandlers: map[connector.DiscoveryMode]connector.DiscoveryHandler{},
	}
	runtime.DiscoveryHandlers[connector.DiscoveryPoll] = func(ctx context.Context, request connector.DiscoveryRequest) (connector.DiscoveryBatch, error) {
		scope, err := parseScope(request.Instance.DiscoveryScope)
		if err != nil {
			return connector.DiscoveryBatch{}, err
		}
		token, err := resolver.ResolveToken(ctx, request.Instance)
		if err != nil {
			return connector.DiscoveryBatch{}, err
		}
		defer func() { token = "" }()
		topologies, err := client.ListDevices(ctx, token, scope.ProjectUUID)
		if err != nil {
			return connector.DiscoveryBatch{}, err
		}
		devices, cursor, err := MapDirectory(scope.ProjectUUID, topologies)
		if err != nil {
			return connector.DiscoveryBatch{}, err
		}
		return connector.DiscoveryBatch{
			Devices: devices, Cursor: cursor, CompleteSnapshot: true, SourceVersion: ContractVersion,
		}, nil
	}
	runtime.HealthCheck = func(ctx context.Context, instance connector.Instance) (connector.Health, error) {
		if _, err := parseScope(instance.DiscoveryScope); err != nil {
			return connector.Health{Status: "failed", Details: map[string]any{"code": "scope_invalid"}}, err
		}
		if _, err := resolver.ResolveToken(ctx, instance); err != nil {
			return connector.Health{Status: "failed", Details: map[string]any{"code": "credential_unavailable"}}, err
		}
		return connector.Health{Status: "configured", Details: map[string]any{"region": "cn", "readOnly": true}}, nil
	}
	runtime.ScopeFilter = func(instance connector.Instance, device connector.ExternalDevice) bool {
		scope, err := parseScope(instance.DiscoveryScope)
		if err != nil || !strings.HasPrefix(device.ExternalID, scope.ProjectUUID+"/") {
			return false
		}
		projectUUID, ok := device.Attributes["projectUuid"].(string)
		return ok && strings.EqualFold(projectUUID, scope.ProjectUUID)
	}
	return registry.Register(runtime)
}

func productType(model DeviceModel) (string, bool) {
	domain, domainErr := strconv.Atoi(model.Domain)
	typeCode, typeErr := strconv.Atoi(model.Type)
	subtype, subtypeErr := strconv.Atoi(model.Subtype)
	if domainErr != nil || typeErr != nil || subtypeErr != nil {
		return dji.UnknownDeviceTypeKey, false
	}
	key := dji.ProductKey{Domain: domain, Type: typeCode, Subtype: subtype}
	if product, ok := dji.ResolveDock2Product(key); ok {
		return product.TypeKey, true
	}
	if product, ok := dji.ResolveDock3Product(key); ok {
		return product.TypeKey, true
	}
	return dji.UnknownDeviceTypeKey, false
}

func mapDevice(projectUUID string, device *Device, parent string) connector.ExternalDevice {
	typeKey, known := productType(device.Model)
	externalID := projectUUID + "/" + device.SN
	attributes := map[string]any{
		"source": "dji.flighthub2", "projectUuid": projectUUID, "serialNumber": device.SN,
		"callsign": device.Callsign, "online": device.Online, "modeCode": device.ModeCode,
		"model": map[string]any{
			"key": device.Model.Key, "domain": device.Model.Domain, "type": device.Model.Type,
			"subtype": device.Model.Subtype, "name": device.Model.Name, "class": device.Model.Class,
		},
		"readOnly": true, "capabilities": []string{"state.read"}, "knownProduct": known,
	}
	if !known {
		attributes["reviewReason"] = "DJI_PRODUCT_ENUM_UNKNOWN"
	}
	return connector.ExternalDevice{
		ExternalID: externalID, ExternalType: typeKey, ParentExternalID: parent, Attributes: attributes,
	}
}

func MapDirectory(projectUUID string, topologies []Topology) ([]connector.ExternalDevice, json.RawMessage, error) {
	projectUUID = strings.ToLower(strings.TrimSpace(projectUUID))
	if !uuidPattern.MatchString(projectUUID) {
		return nil, nil, errors.New("DJI_FLIGHTHUB_SCOPE_INVALID")
	}
	seen := make(map[string]struct{}, len(topologies)*2)
	devices := make([]connector.ExternalDevice, 0, len(topologies)*2)
	for _, topology := range topologies {
		parent := ""
		if topology.Gateway != nil {
			if _, exists := seen[topology.Gateway.SN]; exists {
				return nil, nil, errors.New("DJI_FLIGHTHUB_DUPLICATE_SERIAL")
			}
			seen[topology.Gateway.SN] = struct{}{}
			gateway := mapDevice(projectUUID, topology.Gateway, "")
			parent = gateway.ExternalID
			devices = append(devices, gateway)
		}
		if topology.Drone != nil {
			if _, exists := seen[topology.Drone.SN]; exists {
				return nil, nil, errors.New("DJI_FLIGHTHUB_DUPLICATE_SERIAL")
			}
			seen[topology.Drone.SN] = struct{}{}
			devices = append(devices, mapDevice(projectUUID, topology.Drone, parent))
		}
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].ExternalID < devices[j].ExternalID })
	canonical, err := json.Marshal(devices)
	if err != nil {
		return nil, nil, err
	}
	digest := sha256.Sum256(canonical)
	cursor, err := json.Marshal(map[string]string{"snapshotSha256": hex.EncodeToString(digest[:])})
	if err != nil {
		return nil, nil, err
	}
	return devices, cursor, nil
}

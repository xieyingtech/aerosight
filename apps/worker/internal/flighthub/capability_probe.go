package flighthub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type CapabilityProbeStatus string

const (
	ProbeSupported     CapabilityProbeStatus = "supported"
	ProbeEmpty         CapabilityProbeStatus = "empty"
	ProbeForbidden     CapabilityProbeStatus = "forbidden"
	ProbeNotApplicable CapabilityProbeStatus = "not_applicable"
	ProbeUnverified    CapabilityProbeStatus = "unverified"
	ProbeDegraded      CapabilityProbeStatus = "degraded"
	ProbeFailed        CapabilityProbeStatus = "failed"
)

type CapabilityProbeInput struct {
	Token        string
	Region       string
	Deployment   string
	ProjectUUID  string
	DeviceSerial string
}

type CapabilityProbeLayers struct {
	Contract       CapabilityProbeStatus `json:"contract"`
	Deployment     CapabilityProbeStatus `json:"deployment"`
	Account        CapabilityProbeStatus `json:"account"`
	Implementation CapabilityProbeStatus `json:"implementation"`
	Acceptance     CapabilityProbeStatus `json:"acceptance"`
}

type CapabilityProbeResult struct {
	CapabilityCode string                `json:"capabilityCode"`
	Status         CapabilityProbeStatus `json:"status"`
	Reason         string                `json:"reason"`
	EndpointID     string                `json:"endpointId,omitempty"`
	ItemCount      *int                  `json:"itemCount,omitempty"`
	Layers         CapabilityProbeLayers `json:"layers"`
}

type capabilityProbeEndpoint struct {
	ID                string
	Method            string
	Path              string
	Scope             string
	Profile           string
	Released          bool
	Regions           []string
	Deployments       []string
	CapabilityCodes   []string
	TemplateParameter string
}

type capabilityReadiness struct {
	Implemented bool
	Accepted    bool
}

type capabilityProbeObservation struct {
	Status    CapabilityProbeStatus
	Reason    string
	ItemCount *int
}

var defaultCapabilityProbeEndpoints = []capabilityProbeEndpoint{
	{ID: "456242199e0", Method: http.MethodGet, Path: "/openapi/v2.0/health", Scope: "global", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"health.read", "security.temporary-credential"}},
	{ID: "456447011e0", Method: http.MethodGet, Path: "/openapi/v2.0/organizations", Scope: "global", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"organization.read", "organization.write"}},
	{ID: "456680822e0", Method: http.MethodGet, Path: "/openapi/v2.0/project/device", Scope: "project", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"inventory.read"}},
	{ID: "458069501e0", Method: http.MethodGet, Path: "/openapi/v2.0/device/{device_sn}/state", Scope: "device", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"state.read", "live.quality.set", "device.camera.change", "device.lens.change"}, TemplateParameter: "device_sn"},
	{ID: "456680824e0", Method: http.MethodGet, Path: "/openapi/v2.0/wayline", Scope: "project", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"flight.read", "flight.execute"}},
	{ID: "457494965e0", Method: http.MethodGet, Path: "/openapi/v2.0/live-shares", Scope: "project", Profile: "live-share-list", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"live.read", "live.control", "live.share.manage"}},
	{ID: "457494960e0", Method: http.MethodGet, Path: "/openapi/v2.0/auto-record-configs", Scope: "project", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"live.recording.control"}},
	{ID: "456444816e0", Method: http.MethodGet, Path: "/openapi/v2.0/stream-converters", Scope: "project", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"live.converter.create", "live.converter.toggle", "live.converter.delete"}},
	{ID: "457494969e0", Method: http.MethodGet, Path: "/openapi/v2.0/flight-areas", Scope: "project", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"geospatial.read", "geospatial.write", "geospatial.element.delete"}},
	{ID: "458069507e0", Method: http.MethodGet, Path: "/openapi/v2.0/model", Scope: "project", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"model.read", "model.write", "model.delete", "model.resource.delete"}},
	{ID: "457494961e0", Method: http.MethodGet, Path: "/openapi/v2.0/topologies/cmds/control/status", Scope: "project", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"device.control"}},
	{ID: "454273421e0", Method: http.MethodGet, Path: "/openapi/v2.0/workspaces/{workspace_id}/groups/tcas", Scope: "workspace", Released: true, Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"tca.status.read"}, TemplateParameter: "workspace_id"},
}

var defaultCapabilityReadiness = map[string]capabilityReadiness{
	"inventory.read":                {Implemented: true, Accepted: true},
	"state.read":                    {Implemented: true, Accepted: true},
	"health.read":                   {Implemented: true, Accepted: true},
	"organization.read":             {Implemented: true, Accepted: true},
	"flight.read":                   {Accepted: true},
	"live.read":                     {Accepted: true},
	"geospatial.read":               {Accepted: true},
	"model.read":                    {Accepted: true},
	"security.temporary-credential": {},
	"flight.execute":                {},
	"live.control":                  {},
	"live.quality.set":              {Implemented: true},
	"live.recording.control":        {},
	"live.share.manage":             {},
	"live.converter.create":         {Implemented: true},
	"live.converter.toggle":         {Implemented: true},
	"live.converter.delete":         {Implemented: true},
	"geospatial.write":              {Implemented: true},
	"geospatial.element.delete":     {Implemented: true},
	"model.write":                   {},
	"model.delete":                  {Implemented: true},
	"model.resource.delete":         {Implemented: true},
	"device.control":                {},
	"device.camera.change":          {Implemented: true},
	"device.lens.change":            {Implemented: true},
	"tca.status.read":               {Implemented: true, Accepted: true},
	"organization.write":            {},
}

// ProbeCapabilities gathers read-only upstream evidence and then fails closed
// through the official contract, deployment, account, implementation and
// acceptance layers. It never calls a non-GET endpoint.
func (client *Client) ProbeCapabilities(ctx context.Context, input CapabilityProbeInput) ([]CapabilityProbeResult, error) {
	return client.probeCapabilities(ctx, input, defaultCapabilityProbeEndpoints, defaultCapabilityReadiness)
}

func (client *Client) probeCapabilities(ctx context.Context, input CapabilityProbeInput, endpoints []capabilityProbeEndpoint, readiness map[string]capabilityReadiness) ([]CapabilityProbeResult, error) {
	input.Region = strings.TrimSpace(input.Region)
	input.Deployment = strings.TrimSpace(input.Deployment)
	if input.Region == "" || input.Deployment == "" {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	definitions := Capabilities()
	known := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		known[definition.Code] = struct{}{}
	}
	endpointByCapability := make(map[string]capabilityProbeEndpoint, len(definitions))
	for _, endpoint := range endpoints {
		for _, capabilityCode := range endpoint.CapabilityCodes {
			if _, exists := known[capabilityCode]; !exists {
				return nil, errors.New("DJI_FLIGHTHUB_PROBE_PLAN_INVALID")
			}
			if _, duplicate := endpointByCapability[capabilityCode]; duplicate {
				return nil, errors.New("DJI_FLIGHTHUB_PROBE_PLAN_INVALID")
			}
			endpointByCapability[capabilityCode] = endpoint
		}
	}

	observations := make(map[string]capabilityProbeObservation, len(endpoints))
	for _, endpoint := range endpoints {
		observations[endpoint.ID] = client.probeEndpoint(ctx, input, endpoint)
	}
	results := make([]CapabilityProbeResult, 0, len(definitions))
	for _, definition := range definitions {
		endpoint, hasEndpoint := endpointByCapability[definition.Code]
		local, hasReadiness := readiness[definition.Code]
		if !hasReadiness {
			local = capabilityReadiness{}
		}
		result := CapabilityProbeResult{CapabilityCode: definition.Code, Layers: CapabilityProbeLayers{
			Contract: ProbeUnverified, Deployment: ProbeUnverified, Account: ProbeUnverified,
			Implementation: boolProbeStatus(local.Implemented), Acceptance: boolProbeStatus(local.Accepted),
		}}
		if hasEndpoint {
			result.EndpointID = endpoint.ID
			if endpoint.Released {
				result.Layers.Contract = ProbeSupported
			}
			if regionAllowed(input.Region, endpoint.Regions) && deploymentAllowed(input.Deployment, endpoint.Deployments) {
				result.Layers.Deployment = ProbeSupported
			} else {
				result.Layers.Deployment = ProbeNotApplicable
			}
			observation := observations[endpoint.ID]
			result.Layers.Account = observation.Status
			result.Reason = observation.Reason
			result.ItemCount = observation.ItemCount
		}
		result.Status, result.Reason = effectiveProbeStatus(result.Layers, result.Reason)
		results = append(results, result)
	}
	return results, nil
}

func (client *Client) probeEndpoint(ctx context.Context, input CapabilityProbeInput, endpoint capabilityProbeEndpoint) capabilityProbeObservation {
	if !endpoint.Released {
		return capabilityProbeObservation{Status: ProbeUnverified, Reason: "endpoint_not_released"}
	}
	if !regionAllowed(input.Region, endpoint.Regions) || !deploymentAllowed(input.Deployment, endpoint.Deployments) {
		return capabilityProbeObservation{Status: ProbeNotApplicable, Reason: "deployment_not_applicable"}
	}
	if endpoint.Method != http.MethodGet {
		return capabilityProbeObservation{Status: ProbeUnverified, Reason: "probe_requires_non_get"}
	}
	path := endpoint.Path
	if endpoint.TemplateParameter != "" {
		parameterValue := ""
		switch endpoint.TemplateParameter {
		case "device_sn":
			parameterValue = strings.TrimSpace(input.DeviceSerial)
		case "workspace_id":
			parameterValue = strings.TrimSpace(input.ProjectUUID)
		}
		if parameterValue == "" {
			return capabilityProbeObservation{Status: ProbeUnverified, Reason: "probe_context_required"}
		}
		var err error
		path, err = resolvePathTemplate(path, map[string]string{endpoint.TemplateParameter: parameterValue})
		if err != nil {
			return capabilityProbeObservation{Status: ProbeFailed, Reason: "probe_context_invalid"}
		}
	}
	projectUUID := ""
	if endpoint.Scope == "project" || endpoint.Scope == "device" || endpoint.Scope == "workspace" {
		projectUUID = strings.TrimSpace(input.ProjectUUID)
		if projectUUID == "" {
			return capabilityProbeObservation{Status: ProbeUnverified, Reason: "probe_context_required"}
		}
	}
	payload, err := client.request(ctx, input.Token, projectUUID, requestSpec{
		Method: http.MethodGet, Path: path, Profile: endpoint.Profile, DataOptional: true,
	})
	if err != nil {
		return classifyCapabilityProbeError(err)
	}
	if endpoint.ID == "454273421e0" {
		data := payload.Data
		if len(bytes.TrimSpace(data)) == 0 {
			data = json.RawMessage(`null`)
		}
		decoded, decodeErr := DecodeControlActionOutput("tca.status", data)
		if decodeErr != nil {
			return capabilityProbeObservation{Status: ProbeFailed, Reason: "upstream_schema_invalid"}
		}
		summary, ok := decoded.(OpenControlOutputSummary)
		if !ok {
			return capabilityProbeObservation{Status: ProbeFailed, Reason: "upstream_schema_invalid"}
		}
		count := summary.ItemCount
		if count == 0 {
			return capabilityProbeObservation{Status: ProbeEmpty, Reason: "upstream_empty", ItemCount: &count}
		}
		return capabilityProbeObservation{Status: ProbeSupported, Reason: "read_probe_succeeded", ItemCount: &count}
	}
	if payload.Empty || emptyProbeData(payload.Data) {
		return capabilityProbeObservation{Status: ProbeEmpty, Reason: "upstream_empty"}
	}
	return capabilityProbeObservation{Status: ProbeSupported, Reason: "read_probe_succeeded"}
}

func classifyCapabilityProbeError(err error) capabilityProbeObservation {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return capabilityProbeObservation{Status: ProbeFailed, Reason: "probe_failed"}
	}
	switch apiErr.SafeCode {
	case "scope_forbidden":
		return capabilityProbeObservation{Status: ProbeForbidden, Reason: "account_scope_forbidden"}
	case "configuration_required", "scope_not_found", "schema_incompatible", "upstream_error":
		return capabilityProbeObservation{Status: ProbeUnverified, Reason: "upstream_response_unverified"}
	case "rate_limited", "request_timeout", "upstream_unavailable":
		return capabilityProbeObservation{Status: ProbeDegraded, Reason: "upstream_temporarily_degraded"}
	default:
		return capabilityProbeObservation{Status: ProbeFailed, Reason: apiErr.SafeCode}
	}
}

func emptyProbeData(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return true
	}
	var value any
	if json.Unmarshal(trimmed, &value) != nil {
		return false
	}
	if list, ok := value.([]any); ok {
		return len(list) == 0
	}
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"list", "items", "records"} {
		if list, exists := object[key].([]any); exists {
			return len(list) == 0
		}
	}
	return false
}

func deploymentAllowed(deployment string, allowed []string) bool {
	for _, candidate := range allowed {
		if deployment == candidate {
			return true
		}
	}
	return false
}

func regionAllowed(region string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if region == candidate {
			return true
		}
	}
	return false
}

func boolProbeStatus(value bool) CapabilityProbeStatus {
	if value {
		return ProbeSupported
	}
	return ProbeUnverified
}

func effectiveProbeStatus(layers CapabilityProbeLayers, reason string) (CapabilityProbeStatus, string) {
	if layers.Contract != ProbeSupported {
		return ProbeUnverified, firstProbeReason(reason, "contract_unverified")
	}
	if layers.Deployment != ProbeSupported {
		return ProbeNotApplicable, firstProbeReason(reason, "deployment_not_applicable")
	}
	switch layers.Account {
	case ProbeForbidden, ProbeDegraded, ProbeFailed, ProbeUnverified, ProbeNotApplicable:
		return layers.Account, firstProbeReason(reason, "account_unverified")
	}
	if layers.Implementation != ProbeSupported {
		return ProbeUnverified, "implementation_unavailable"
	}
	if layers.Acceptance != ProbeSupported {
		switch layers.Acceptance {
		case ProbeForbidden, ProbeDegraded, ProbeFailed, ProbeNotApplicable:
			return layers.Acceptance, "acceptance_" + string(layers.Acceptance)
		}
		return ProbeUnverified, "acceptance_required"
	}
	if layers.Account == ProbeEmpty {
		return ProbeEmpty, firstProbeReason(reason, "upstream_empty")
	}
	return ProbeSupported, firstProbeReason(reason, "read_probe_succeeded")
}

func firstProbeReason(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

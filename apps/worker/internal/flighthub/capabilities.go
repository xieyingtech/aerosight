package flighthub

import (
	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/driver"
)

const (
	FlightHubActionFeatureFlag              = "flighthub.actions"
	FlightHubLiveQualityFeatureFlag         = "flighthub.live.quality"
	FlightHubLiveRecordingFeatureFlag       = "flighthub.live.recording"
	FlightHubLiveShareFeatureFlag           = "flighthub.live.share"
	FlightHubLiveConverterCreateFeatureFlag = "flighthub.live.converter.create"
	FlightHubLiveConverterToggleFeatureFlag = "flighthub.live.converter.toggle"
	FlightHubLiveConverterDeleteFeatureFlag = "flighthub.live.converter.delete"
	FlightHubGeospatialDeleteFeatureFlag    = "flighthub.geospatial.delete"
)

func Capabilities() []connector.CapabilityDefinition {
	return []connector.CapabilityDefinition{
		{Code: "inventory.read", Kind: connector.CapabilityRead, Risk: driver.RiskLow, EndpointDomains: []string{"device"}},
		{Code: "state.read", Kind: connector.CapabilityRead, Risk: driver.RiskLow, EndpointDomains: []string{"device"}, DriverCapability: "state.read", DefaultEnabled: true},
		{Code: "health.read", Kind: connector.CapabilityRead, Risk: driver.RiskLow, EndpointDomains: []string{"system", "device"}, DefaultEnabled: true},
		{Code: "organization.read", Kind: connector.CapabilityRead, Risk: driver.RiskLow, EndpointDomains: []string{"organization", "project"}, DefaultEnabled: true},
		{Code: "flight.read", Kind: connector.CapabilityRead, Risk: driver.RiskLow, EndpointDomains: []string{"flight"}, DefaultEnabled: true},
		{Code: "live.read", Kind: connector.CapabilityRead, Risk: driver.RiskLow, EndpointDomains: []string{"live"}, DriverCapability: "stream.video.read", DefaultEnabled: true},
		{Code: "geospatial.read", Kind: connector.CapabilityRead, Risk: driver.RiskLow, EndpointDomains: []string{"geospatial"}, DefaultEnabled: true},
		{Code: "model.read", Kind: connector.CapabilityRead, Risk: driver.RiskLow, EndpointDomains: []string{"model"}, DefaultEnabled: true},
		{Code: "security.temporary-credential", Kind: connector.CapabilityAction, Risk: driver.RiskMedium, EndpointDomains: []string{"security"}, FeatureFlag: FlightHubActionFeatureFlag},
		{Code: "flight.execute", Kind: connector.CapabilityAction, Risk: driver.RiskCritical, EndpointDomains: []string{"flight"}, DriverCapability: "mission.execute", FeatureFlag: FlightHubActionFeatureFlag},
		{Code: "live.control", Kind: connector.CapabilityAction, Risk: driver.RiskHigh, EndpointDomains: []string{"live"}, DriverCapability: "stream.video.control", FeatureFlag: FlightHubActionFeatureFlag},
		{Code: "live.quality.set", Kind: connector.CapabilityAction, Risk: driver.RiskHigh, EndpointDomains: []string{"live"}, DriverCapability: "stream.video.control", FeatureFlag: FlightHubLiveQualityFeatureFlag},
		{Code: "live.recording.control", Kind: connector.CapabilityAction, Risk: driver.RiskHigh, EndpointDomains: []string{"live"}, FeatureFlag: FlightHubLiveRecordingFeatureFlag},
		{Code: "live.share.manage", Kind: connector.CapabilityAction, Risk: driver.RiskHigh, EndpointDomains: []string{"live"}, FeatureFlag: FlightHubLiveShareFeatureFlag},
		{Code: "live.converter.create", Kind: connector.CapabilityAction, Risk: driver.RiskHigh, EndpointDomains: []string{"live"}, FeatureFlag: FlightHubLiveConverterCreateFeatureFlag},
		{Code: "live.converter.toggle", Kind: connector.CapabilityAction, Risk: driver.RiskHigh, EndpointDomains: []string{"live"}, FeatureFlag: FlightHubLiveConverterToggleFeatureFlag},
		{Code: "live.converter.delete", Kind: connector.CapabilityAction, Risk: driver.RiskCritical, EndpointDomains: []string{"live"}, FeatureFlag: FlightHubLiveConverterDeleteFeatureFlag},
		{Code: "geospatial.write", Kind: connector.CapabilityAction, Risk: driver.RiskHigh, EndpointDomains: []string{"geospatial"}, FeatureFlag: FlightHubActionFeatureFlag},
		{Code: "geospatial.element.delete", Kind: connector.CapabilityAction, Risk: driver.RiskCritical, EndpointDomains: []string{"geospatial"}, FeatureFlag: FlightHubGeospatialDeleteFeatureFlag},
		{Code: "model.write", Kind: connector.CapabilityAction, Risk: driver.RiskHigh, EndpointDomains: []string{"model"}, FeatureFlag: FlightHubActionFeatureFlag},
		{Code: "device.control", Kind: connector.CapabilityAction, Risk: driver.RiskCritical, EndpointDomains: []string{"control"}, DriverCapability: "flight.return_home", FeatureFlag: FlightHubActionFeatureFlag},
		{Code: "organization.write", Kind: connector.CapabilityAction, Risk: driver.RiskCritical, EndpointDomains: []string{"organization", "project"}, FeatureFlag: FlightHubActionFeatureFlag},
	}
}

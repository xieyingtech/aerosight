package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"github.com/gin-gonic/gin"
	"testing"
)

func TestDock2DeviceCatalogClassification(t *testing.T) {
	for key, role := range map[string]string{"dji.dock2": "机场本体", "dji.matrice3d": "配套飞行器", "dji.matrice3td": "配套飞行器", "dji.dock2.camera": "相机负载", "dji.matrice3td.camera": "相机负载", "dji.dock2.environment-sensor": "环境传感器"} {
		catalog := flightHubDeviceCatalog(gin.H{"connectorKey": "dji.flighthub2", "typeKey": key})
		if catalog["family"] != "Dock 2" || catalog["role"] != role {
			t.Fatalf("%s: %+v", key, catalog)
		}
	}
	if flightHubDeviceCatalog(gin.H{"connectorKey": "dji.cloud", "typeKey": "dji.dock2"}) != nil {
		t.Fatal("Cloud API got FlightHub claims")
	}
	other := flightHubDeviceCatalog(gin.H{"connectorKey": "dji.flighthub2", "typeKey": "dji.dock3"})
	if other["family"] == "Dock 2" || len(other["groups"].([]gin.H)) != 0 {
		t.Fatal("unsupported family inherited Dock 2")
	}
}

func TestFlightHubFlightCommandsRequireDockTarget(t *testing.T) {
	for _, key := range []string{"return_home", "return_home_cancel", "flighttask_pause", "flighttask_recovery"} {
		policy := fhDiscretePolicies[key]
		_, err := fhCommandSafety(context.Background(), nil, 1, 1, deviceCommandInput{Key: key, Capability: policy.capability, Parameters: map[string]any{}}, sqlcgen.LockDeviceCommandTargetRow{TypeKey: "dji.matrice3td"}, gin.H{})
		if err == nil || err.Error() != "FLIGHTHUB_COMMAND_MODEL_UNSUPPORTED" {
			t.Fatalf("%s: %v", key, err)
		}
	}
}

package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
)

func loadFHWorkspace(ctx context.Context, q *sqlcgen.Queries, kind string, uid, pid, tid int32, cid int64) (map[string][]gin.H, error) {
	out := map[string][]gin.H{}
	var raw []json.RawMessage
	var err error
	switch kind {
	case "FlightOps":
		raw, err = q.ReadFHFlightOpsAccess(ctx, sqlcgen.ReadFHFlightOpsAccessParams{UserID: uid, ProjectID: pid})
		if err != nil {
			return nil, err
		}
		out["access"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHFlightOpsConnectors(ctx, sqlcgen.ReadFHFlightOpsConnectorsParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["connectors"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHFlightOpsWaylines(ctx, sqlcgen.ReadFHFlightOpsWaylinesParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["waylines"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHFlightOpsTaskRuns(ctx, sqlcgen.ReadFHFlightOpsTaskRunsParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["task-runs"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHFlightOpsTracks(ctx, sqlcgen.ReadFHFlightOpsTracksParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["tracks"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		for _, resourceKind := range []string{"flight-media", "flight-record"} {
			raw, err = q.ReadFHFlightOpsAssets(ctx, sqlcgen.ReadFHFlightOpsAssetsParams{ProjectID: pid, TeamID: tid, ResourceKind: resourceKind})
			if err != nil {
				return nil, err
			}
			out[resourceKind], err = decodeSnapshotRows(raw)
			if err != nil {
				return nil, err
			}
		}
		raw, err = q.ReadFHFlightOpsAlerts(ctx, sqlcgen.ReadFHFlightOpsAlertsParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["alerts"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHFlightOpsActions(ctx, sqlcgen.ReadFHFlightOpsActionsParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["actions"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}

	case "Geo":
		raw, err = q.ReadFHGeoAccess(ctx, sqlcgen.ReadFHGeoAccessParams{UserID: uid, ProjectID: pid})
		if err != nil {
			return nil, err
		}
		out["access"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHGeoConnectors(ctx, sqlcgen.ReadFHGeoConnectorsParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["connectors"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHGeoSyncStates(ctx, sqlcgen.ReadFHGeoSyncStatesParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["sync-states"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHGeoResources(ctx, sqlcgen.ReadFHGeoResourcesParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["resources"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}

	case "Models":
		raw, err = q.ReadFHModelsAccess(ctx, sqlcgen.ReadFHModelsAccessParams{UserID: uid, ProjectID: pid})
		if err != nil {
			return nil, err
		}
		out["access"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHModelsConnectors(ctx, sqlcgen.ReadFHModelsConnectorsParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["connectors"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHModelsActions(ctx, sqlcgen.ReadFHModelsActionsParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["actions"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHModelsSync(ctx, sqlcgen.ReadFHModelsSyncParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["sync"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHModelsResources(ctx, sqlcgen.ReadFHModelsResourcesParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["resources"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHModelsJobs(ctx, sqlcgen.ReadFHModelsJobsParams{ProjectID: pid, TeamID: tid})
		if err != nil {
			return nil, err
		}
		out["jobs"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}

	case "Diagnostics":
		raw, err = q.ReadFHDiagnosticsAccess(ctx, sqlcgen.ReadFHDiagnosticsAccessParams{UserID: uid, ProjectID: pid, ConnectorID: cid})
		if err != nil {
			return nil, err
		}
		out["access"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHDiagnosticsWatermarks(ctx, sqlcgen.ReadFHDiagnosticsWatermarksParams{ProjectID: pid, ConnectorID: cid})
		if err != nil {
			return nil, err
		}
		out["watermarks"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHDiagnosticsCapabilities(ctx, sqlcgen.ReadFHDiagnosticsCapabilitiesParams{ProjectID: pid, ConnectorID: cid})
		if err != nil {
			return nil, err
		}
		out["capabilities"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}

	case "Management":
		raw, err = q.ReadFHManagementAccess(ctx, sqlcgen.ReadFHManagementAccessParams{UserID: uid, ProjectID: pid, ConnectorID: cid})
		if err != nil {
			return nil, err
		}
		out["access"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}

		if len(out["access"]) == 0 {
			return nil, errors.New("FLIGHTHUB_MANAGEMENT_ACCESS_DENIED")
		}
		access := out["access"][0]
		if access["role"] != "owner" && access["role"] != "admin" {
			return nil, errors.New("FLIGHTHUB_MANAGEMENT_PERMISSION_DENIED")
		}
		if access["connectorStatus"] != "connected" {
			return nil, errors.New("FLIGHTHUB_MANAGEMENT_CONNECTOR_OFFLINE")
		}
		if access["managementCapabilityVerified"] != true {
			return nil, errors.New("FLIGHTHUB_MANAGEMENT_CAPABILITY_REQUIRED")
		}
		raw, err = q.ReadFHManagementState(ctx, sqlcgen.ReadFHManagementStateParams{ProjectID: pid, TeamID: tid, ConnectorID: cid})
		if err != nil {
			return nil, err
		}
		out["state"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFHManagementResources(ctx, sqlcgen.ReadFHManagementResourcesParams{ProjectID: pid, TeamID: tid, ConnectorID: cid})
		if err != nil {
			return nil, err
		}
		out["resources"], err = decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unknown workspace")
	}
	if len(out["access"]) == 0 {
		return nil, sql.ErrNoRows
	}
	return out, nil
}

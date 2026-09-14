package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"aerosight/server/internal/connector"
)

func retainInspectionAlert(ctx context.Context, tx *sql.Tx, instance connector.Instance, resourceID int64, alert AIAlertRecord, ownership string) error {
	// Preserve the provider's target_value semantics, not the legacy confidence
	// interpretation. Private remote identifiers and signed media URLs are omitted.
	targets := make([]map[string]any, 0, len(alert.Targets))
	for _, target := range alert.Targets {
		targets = append(targets, map[string]any{"targetType": target.TargetType, "targetValue": target.Confidence, "useMinThreshold": target.UseMinThreshold, "useMaxThreshold": target.UseMaxThreshold, "minimumThreshold": target.MinimumThreshold, "maximumThreshold": target.MaximumThreshold, "label": alertText(target.Label, 96)})
	}
	evidence := map[string]any{"source": "flighthub-ai", "algorithmSource": alert.AlgorithmSource, "modelVersion": "unknown", "algorithmEnabledConfirmed": false,
		"timestamp": alert.Timestamp, "reason": alertText(alert.Reason, 512), "status": alert.Status, "targets": targets, "positionSource": "unknown", "coordinateReference": "unverified"}
	if validAIAlertLocation(alert.Location) {
		evidence["location"] = alert.Location
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `insert into inspection_alert_sources(project_id,connector_instance_id,remote_resource_id,remote_flight_id,evidence_json)
 values($1,$2,$3,$4,$5) on conflict(project_id,remote_resource_id) do update set evidence_json=excluded.evidence_json,updated_at=now()
 where inspection_alert_sources.connector_instance_id=excluded.connector_instance_id and inspection_alert_sources.remote_flight_id=excluded.remote_flight_id`, instance.ProjectID, instance.ID, resourceID, alert.FlightID, raw)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("INSPECTION_ALERT_FLIGHT_ID_CHANGED")
	}
	if ownership != "legacy" {
		_, err = tx.ExecContext(ctx, `update connector_remote_resources set summary_json=(summary_json-'confidence')||jsonb_build_object('inspectionOwnership',$3::text,'evidenceOnly',true,'modelVersion','unknown','positionSource','unknown') where project_id=$1 and id=$2`, instance.ProjectID, resourceID, ownership)
	}
	return err
}

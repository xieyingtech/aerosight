package report

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"aerosight/server/internal/inspection"
)

type inspectionReport struct {
	ScopeNotice  string                   `json:"scopeNotice"`
	Observations []inspection.Observation `json:"observations"`
	EvidenceSets []inspection.EvidenceSet `json:"evidenceSets"`
	Assessments  []json.RawMessage        `json:"assessments"`
	Links        []inspectionReportLink   `json:"links"`
}
type inspectionReportLink struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Href string `json:"href"`
}

func appendInspectionContent(ctx context.Context, tx *sql.Tx, projectID, runID int, content *reportContent) error {
	detail := inspectionReport{ScopeNotice: "结论仅适用于列出的图片和观察时段；既有图片分析不代表本次现场航拍，图片位置不等同于目标坐标。", Observations: []inspection.Observation{}, EvidenceSets: []inspection.EvidenceSet{}, Assessments: []json.RawMessage{}, Links: []inspectionReportLink{}}
	rows, err := tx.QueryContext(ctx, "select manifest_json from inspection_observations where project_id=$1 and task_run_id=$2 and sealed_at is not null order by created_at,id", projectID, runID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var observation inspection.Observation
		if err = json.Unmarshal(raw, &observation); err != nil {
			rows.Close()
			return err
		}
		detail.Observations = append(detail.Observations, observation)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(detail.Observations) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, asset := range content.Assets {
		seen[fmt.Sprintf("%d:%d", asset.ID, asset.Version)] = true
	}
	for _, observation := range detail.Observations {
		detail.Links = append(detail.Links, inspectionReportLink{"observation", observation.ID, fmt.Sprintf("/api/projects/%d/inspection/observations/%s", projectID, observation.ID)})
		if observation.Completeness != inspection.Complete {
			content.DataGaps = append(content.DataGaps, "observation:"+observation.ID+":"+string(observation.Completeness))
		}
		for _, ref := range observation.Assets {
			key := fmt.Sprintf("%d:%d", ref.AssetID, ref.Version)
			existing := seen[key]
			seen[key] = true
			var current bool
			if err = tx.QueryRowContext(ctx, `select exists(select 1 from assets where project_id=$1 and id=$2 and version=$3 and status='available' and deleted_at is null and coalesce(object_version,'')=$4)`, projectID, ref.AssetID, ref.Version, ref.ObjectVersion).Scan(&current); err != nil {
				return err
			}
			if !existing {
				content.Assets = append(content.Assets, assetFact{ID: int(ref.AssetID), Version: ref.Version, Kind: "image", Checksum: ref.ChecksumSHA256})
			} else {
				for n := range content.Assets {
					if content.Assets[n].ID == int(ref.AssetID) && content.Assets[n].Version == ref.Version {
						content.Assets[n].Checksum = ref.ChecksumSHA256
					}
				}
			}
			if !current {
				content.DataGaps = append(content.DataGaps, "asset:"+key+":unavailable-or-version-changed")
			}
		}
	}
	rows, err = tx.QueryContext(ctx, "select evidence_json from inspection_evidence_sets where project_id=$1 and task_run_id=$2 order by created_at,id", projectID, runID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var e inspection.EvidenceSet
		if err = json.Unmarshal(raw, &e); err != nil {
			rows.Close()
			return err
		}
		detail.EvidenceSets = append(detail.EvidenceSets, e)
		detail.Links = append(detail.Links, inspectionReportLink{"evidenceSet", e.ID, fmt.Sprintf("/api/projects/%d/inspection/evidence-sets/%s", projectID, e.ID)})
		if e.Completeness != inspection.Complete {
			content.DataGaps = append(content.DataGaps, "evidenceSet:"+e.ID+":"+string(e.Completeness))
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `select a.id::text,a.status,jsonb_build_object('id',a.id,'status',a.status,'revision',a.revision,'evidenceSetId',a.evidence_set_id,'providerId',a.provider_id,'modelVersion',a.model_version,'promptVersion',a.prompt_version,'evidenceHash',a.evidence_hash,'decisions',coalesce(r.decisions_json,'[]'::jsonb),'decisionSource',r.source) from inspection_assessments a left join inspection_assessment_revisions r on r.assessment_id=a.id and r.project_id=a.project_id and r.revision=a.revision where a.project_id=$1 and a.task_run_id=$2 order by a.created_at,a.id`, projectID, runID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, status string
		var raw []byte
		if err = rows.Scan(&id, &status, &raw); err != nil {
			rows.Close()
			return err
		}
		detail.Assessments = append(detail.Assessments, json.RawMessage(raw))
		detail.Links = append(detail.Links, inspectionReportLink{"assessment", id, fmt.Sprintf("/api/projects/%d/inspection/assessments/%s", projectID, id)})
		if status != "succeeded" {
			content.DataGaps = append(content.DataGaps, "assessment:"+id+":"+status)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	content.Inspection = &detail
	return nil
}

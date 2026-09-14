package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) readGeneratedReport(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, "REPORT_NOT_FOUND")
		return
	}
	id, err := uuid.Parse(c.Param("reportId"))
	if err != nil {
		s.failure(c, 404, "REPORT_NOT_FOUND")
		return
	}
	if _, err = s.projectAccess(c.Request.Context(), s.queries, currentUser(c).ID, pid, "project:view"); err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	var raw []byte
	err = s.db.QueryRowContext(c.Request.Context(), `select jsonb_build_object('id',report.id,'title',report.title,'versionId',version.id,'version',version.version,'status',version.status,'completeness',version.completeness,'content',version.content_json,'dataGaps',version.data_gaps_json,'evidence',coalesce((select jsonb_agg(jsonb_build_object('kind',ref.evidence_type,'id',ref.evidence_id,'version',ref.evidence_version,'href',ref.href,'availability',case when ref.asset_id is not null and not exists(select 1 from assets asset where asset.id=ref.asset_id and asset.project_id=ref.project_id and (asset.version::text=ref.evidence_version or 'version:'||asset.version::text=ref.evidence_version or (ref.checksum_sha256 is not null and ref.evidence_version=ref.checksum_sha256 and asset.checksum_sha256=ref.checksum_sha256)) and (ref.checksum_sha256 is null or asset.checksum_sha256 is null or ref.checksum_sha256=asset.checksum_sha256) and asset.status='available' and asset.deleted_at is null) then 'unavailable-or-version-changed' else 'not-revalidated' end)) from generated_report_evidence ref where ref.project_id=report.project_id and ref.report_version_id=version.id),'[]'::jsonb)) from generated_reports report join lateral (select v.* from generated_report_versions v where v.generated_report_id=report.id and v.project_id=report.project_id order by v.version desc limit 1) version on true where report.id=$1 and report.project_id=$2`, id, pid).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		s.failure(c, 404, "REPORT_NOT_FOUND")
		return
	}
	if err != nil {
		s.failure(c, 500, "REPORT_READ_FAILED")
		return
	}
	c.JSON(200, json.RawMessage(raw))
}

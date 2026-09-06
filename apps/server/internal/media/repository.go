package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type SQLRepository struct{}

func NewSQLRepository() SQLRepository { return SQLRepository{} }

func (SQLRepository) LoadSource(ctx context.Context, tx *sql.Tx, projectID, assetID int) (SourceAsset, error) {
	if tx == nil {
		return SourceAsset{}, errors.New("media repository requires an active transaction")
	}
	var asset SourceAsset
	err := tx.QueryRowContext(ctx, `
		select id, project_id, team_id, logical_key, version, kind, mime_type,
		       storage_key, coalesce(checksum_sha256, '')
		from assets
		where project_id = $1 and id = $2 and status = 'available'`, projectID, assetID,
	).Scan(
		&asset.ID, &asset.ProjectID, &asset.TeamID, &asset.LogicalKey, &asset.Version,
		&asset.Kind, &asset.MimeType, &asset.StorageKey, &asset.ChecksumSHA256,
	)
	return asset, err
}

func (SQLRepository) SaveDerivative(ctx context.Context, tx *sql.Tx, derivative Derivative) error {
	if tx == nil {
		return errors.New("media repository requires an active transaction")
	}
	if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1, 0))",
		"asset-derivative:"+derivative.StorageKey); err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]any{
		"width": derivative.Width, "height": derivative.Height,
		"generator": derivative.Generator, "sourceAssetId": derivative.SourceAssetID,
	})
	if err != nil {
		return err
	}
	var derivativeAssetID int
	err = tx.QueryRowContext(ctx, `
		insert into assets (
		  project_id, team_id, kind, mime_type, storage_key, logical_key, version,
		  status, object_version, size_bytes, checksum, checksum_sha256, metadata_json, available_at
		) values ($1, $2, $3, $4, $5, $6, $7, 'available', nullif($8, ''), $9, $10, $10, $11, now())
		on conflict (project_id, logical_key, version) do update
		  set logical_key = excluded.logical_key
		returning id`,
		derivative.ProjectID, derivative.TeamID, derivative.Kind, derivative.MimeType,
		derivative.StorageKey, derivative.LogicalKey, derivative.Version,
		derivative.ObjectVersion, derivative.SizeBytes, derivative.ChecksumSHA256, metadata,
	).Scan(&derivativeAssetID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		insert into asset_derivatives (
		  project_id, team_id, source_asset_id, derived_asset_id,
		  derivative_type, generator, generator_version, parameters_json
		) values ($1, $2, $3, $4, $5, $6, $6, $7)
		on conflict (source_asset_id, derived_asset_id, derivative_type) do nothing`,
		derivative.ProjectID, derivative.TeamID, derivative.SourceAssetID, derivativeAssetID,
		derivative.DerivativeType, derivative.Generator, metadata,
	)
	return err
}

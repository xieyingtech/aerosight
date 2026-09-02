package flighthub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"aerosight/worker/internal/connector"
)

func modelTime(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	var parsed time.Time
	if value < 10_000_000_000 {
		parsed = time.Unix(value, 0).UTC()
	} else {
		parsed = time.UnixMilli(value).UTC()
	}
	return &parsed
}

func modelVersion(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:16]), nil
}

func modelRemoteResources(items []ModelSummary) ([]connector.RemoteResource, error) {
	resources := make([]connector.RemoteResource, 0, len(items))
	seen := make(map[int64]struct{}, len(items))
	for index := range items {
		item := items[index]
		if !validModelSummary(&item) {
			return nil, schemaError()
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, schemaError()
		}
		seen[item.ID] = struct{}{}
		version, err := modelVersion(map[string]any{
			"fileType": item.FileType, "size": item.Size, "showOnMap": item.ShowOnMap, "updatedAt": item.UpdatedAt,
		})
		if err != nil {
			return nil, err
		}
		resources = append(resources, connector.RemoteResource{
			RemoteID: strconv.FormatInt(item.ID, 10), RemoteVersion: version, RemoteUpdatedAt: modelTime(item.UpdatedAt),
			Summary: map[string]any{
				"name": item.Name, "fileType": item.FileType, "showOnMap": item.ShowOnMap,
				"sizeBytes": item.Size, "createdAt": item.CreatedAt, "updatedAt": item.UpdatedAt,
				"source": "dji-flighthub-openapi",
			},
		})
	}
	return resources, nil
}

func openModelRemoteResources(models []OpenModel, resources []OpenModelResource) ([]connector.RemoteResource, error) {
	result := make([]connector.RemoteResource, 0, len(models)+len(resources))
	seen := make(map[string]struct{}, len(models)+len(resources))
	for index := range models {
		item := models[index]
		if !validOpenModel(&item, true) {
			return nil, schemaError()
		}
		remoteID := "model:" + item.ModelUUID
		if _, duplicate := seen[remoteID]; duplicate {
			return nil, schemaError()
		}
		seen[remoteID] = struct{}{}
		version, err := modelVersion(map[string]any{
			"type": item.ModelType, "status": item.ModelStatus, "size": item.ModelSize,
			"progress": item.ReconstructionProgress, "errorCode": item.ErrorCode,
			"zipStatus": item.ZipStatus, "zipProgress": item.ZipProgress,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, connector.RemoteResource{
			RemoteID: remoteID, RemoteVersion: version,
			Summary: map[string]any{
				"resourceReference": secureRemoteKey(item.ResourceUUID), "modelType": item.ModelType,
				"modelStatus": item.ModelStatus, "sizeBytes": item.ModelSize,
				"reconstructionProgress": item.ReconstructionProgress, "errorCode": item.ErrorCode,
				"zipStatus": item.ZipStatus, "zipProgress": item.ZipProgress,
				"source": "dji-flighthub-openapi",
			},
		})
	}
	for index := range resources {
		item := resources[index]
		if !validModelString(item.ResourceUUID, 256, false) || item.Status < 1 || item.Status > 3 || item.Size < 0 || len(item.FileNames) > 20000 {
			return nil, schemaError()
		}
		for _, name := range item.FileNames {
			if !validModelFileName(name) {
				return nil, schemaError()
			}
		}
		remoteID := "resource:" + item.ResourceUUID
		if _, duplicate := seen[remoteID]; duplicate {
			return nil, schemaError()
		}
		seen[remoteID] = struct{}{}
		version, err := modelVersion(map[string]any{"status": item.Status, "size": item.Size, "fileNames": item.FileNames})
		if err != nil {
			return nil, err
		}
		result = append(result, connector.RemoteResource{
			RemoteID: remoteID, RemoteVersion: version,
			Summary: map[string]any{
				"resourceStatus": item.Status, "sizeBytes": item.Size, "fileCount": len(item.FileNames),
				"source": "dji-flighthub-openapi",
			},
		})
	}
	return result, nil
}

func openModelAssetState(item OpenModel) (string, string) {
	switch item.ModelStatus {
	case OpenModelReconstructionFailed, OpenModelRequestingResourceFailed:
		return "failed", "DJI_FLIGHTHUB_MODEL_FAILED"
	case OpenModelReconstructionCanceled:
		return "failed", "DJI_FLIGHTHUB_MODEL_CANCELED"
	case OpenModelMapReconstructionSucceeded, OpenModelReconstructionSucceeded:
		if item.ZipStatus == OpenModelZipFailed {
			return "failed", "DJI_FLIGHTHUB_MODEL_PACKAGE_FAILED"
		}
		if item.ZipStatus == OpenModelZipFinished || item.ZipStatus == OpenModelZipInitial {
			return "available", ""
		}
	}
	return "pending", ""
}

func openResourceAssetState(status int) (string, string) {
	switch status {
	case 1:
		return "available", ""
	case 2:
		return "pending", ""
	case 3:
		return "failed", "DJI_FLIGHTHUB_MODEL_RESOURCE_DELETED"
	default:
		return "failed", "DJI_FLIGHTHUB_MODEL_RESOURCE_INVALID"
	}
}

func (projector *SQLFlightCatalogProjector) ApplyModels(ctx context.Context, instance connector.Instance, poll ModelCatalogPoll) (returnedErr error) {
	if projector == nil || projector.db == nil || poll.ReceivedAt.IsZero() {
		return errors.New("FlightHub model projector is unavailable")
	}
	if len(poll.Models)+len(poll.OpenModels)+len(poll.Resources) > 20_000 {
		return errors.New("FlightHub model projection exceeds bounded batch")
	}
	tx, teamID, err := projector.beginWritable(ctx, instance)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()

	for index := range poll.Models {
		item := poll.Models[index]
		if !validModelSummary(&item) {
			return schemaError()
		}
		version, err := modelVersion(map[string]any{
			"fileType": item.FileType, "size": item.Size, "showOnMap": item.ShowOnMap, "updatedAt": item.UpdatedAt,
		})
		if err != nil {
			return err
		}
		size := item.Size
		if _, err := projector.upsertExternalAsset(ctx, tx, instance, teamID, externalAssetInput{
			ResourceKind: "model", RemoteID: strconv.FormatInt(item.ID, 10), RemoteVersion: version,
			RemoteUpdatedAt: modelTime(item.UpdatedAt), AssetKind: "model", MIMEType: "application/octet-stream",
			Status: "available", SizeBytes: &size, CapturedAt: modelTime(item.CreatedAt),
			Summary: map[string]any{
				"name": item.Name, "fileType": item.FileType, "showOnMap": item.ShowOnMap,
				"sizeBytes": item.Size, "createdAt": item.CreatedAt, "updatedAt": item.UpdatedAt,
			},
			Metadata: map[string]any{
				"source": "dji-flighthub-openapi", "sourceKind": "model", "remoteReference": true,
				"temporaryAccess": true, "fileType": item.FileType, "name": item.Name,
			},
			Locator: map[string]string{"fileID": strconv.FormatInt(item.ID, 10)},
		}); err != nil {
			return err
		}
	}

	for index := range poll.OpenModels {
		item := poll.OpenModels[index]
		if !validOpenModel(&item, true) {
			return schemaError()
		}
		version, err := modelVersion(map[string]any{
			"type": item.ModelType, "status": item.ModelStatus, "size": item.ModelSize,
			"progress": item.ReconstructionProgress, "errorCode": item.ErrorCode,
			"zipStatus": item.ZipStatus, "zipProgress": item.ZipProgress,
		})
		if err != nil {
			return err
		}
		status, failureCode := openModelAssetState(item)
		size := item.ModelSize
		if _, err := projector.upsertExternalAsset(ctx, tx, instance, teamID, externalAssetInput{
			ResourceKind: "model-resource", RemoteID: "model:" + item.ModelUUID, RemoteVersion: version,
			RemoteUpdatedAt: &poll.ReceivedAt, AssetKind: "model", MIMEType: "application/octet-stream",
			Status: status, SizeBytes: &size,
			Summary: map[string]any{
				"resourceReference": secureRemoteKey(item.ResourceUUID), "modelType": item.ModelType,
				"modelStatus": item.ModelStatus, "sizeBytes": item.ModelSize,
				"reconstructionProgress": item.ReconstructionProgress, "errorCode": item.ErrorCode,
				"zipStatus": item.ZipStatus, "zipProgress": item.ZipProgress,
			},
			Metadata: map[string]any{
				"source": "dji-flighthub-openapi", "sourceKind": "model-resource", "remoteReference": true,
				"temporaryAccess": false, "modelType": item.ModelType,
			},
			FailureCode: failureCode,
		}); err != nil {
			return err
		}
	}

	for index := range poll.Resources {
		item := poll.Resources[index]
		version, err := modelVersion(map[string]any{"status": item.Status, "size": item.Size, "fileNames": item.FileNames})
		if err != nil {
			return err
		}
		status, failureCode := openResourceAssetState(item.Status)
		size := item.Size
		if _, err := projector.upsertExternalAsset(ctx, tx, instance, teamID, externalAssetInput{
			ResourceKind: "model-resource", RemoteID: "resource:" + item.ResourceUUID, RemoteVersion: version,
			RemoteUpdatedAt: &poll.ReceivedAt, AssetKind: "file", MIMEType: "application/octet-stream",
			Status: status, SizeBytes: &size,
			Summary: map[string]any{"resourceStatus": item.Status, "sizeBytes": item.Size, "fileCount": len(item.FileNames)},
			Metadata: map[string]any{
				"source": "dji-flighthub-openapi", "sourceKind": "model-resource", "remoteReference": true,
				"temporaryAccess": false, "fileCount": len(item.FileNames),
			},
			FailureCode: failureCode,
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

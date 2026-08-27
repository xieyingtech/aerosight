package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"

	"aerosight/worker/internal/outbox"
)

const ThumbnailGeneratorVersion = "aerosight-thumbnail/v1"

type Object struct {
	Key            string
	Body           []byte
	ContentType    string
	ChecksumSHA256 string
	VersionID      string
}

type ObjectStorage interface {
	GetObject(context.Context, string) (Object, error)
	PutObject(context.Context, string, io.Reader, string) (Object, error)
}

type SourceAsset struct {
	ID             int
	ProjectID      int
	TeamID         int
	LogicalKey     string
	Version        int
	Kind           string
	MimeType       string
	StorageKey     string
	ChecksumSHA256 string
}

type Derivative struct {
	SourceAssetID  int
	ProjectID      int
	TeamID         int
	LogicalKey     string
	Version        int
	Kind           string
	MimeType       string
	StorageKey     string
	ObjectVersion  string
	SizeBytes      int64
	ChecksumSHA256 string
	DerivativeType string
	Generator      string
	Width          int
	Height         int
}

type Repository interface {
	LoadSource(context.Context, *sql.Tx, int, int) (SourceAsset, error)
	SaveDerivative(context.Context, *sql.Tx, Derivative) error
}

type Processor struct {
	storage    ObjectStorage
	repository Repository
	maxWidth   int
	maxHeight  int
}

func NewProcessor(storage ObjectStorage, repository Repository) *Processor {
	return &Processor{storage: storage, repository: repository, maxWidth: 320, maxHeight: 240}
}

func (processor *Processor) Handler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var payload struct {
		AssetID int `json:"assetId"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.AssetID <= 0 {
		return errors.New("asset.available payload requires a valid assetId")
	}
	source, err := processor.repository.LoadSource(ctx, tx, event.ProjectID, payload.AssetID)
	if err != nil {
		return err
	}
	if source.ProjectID != event.ProjectID || source.TeamID != event.TeamID {
		return errors.New("asset source scope does not match outbox event")
	}
	if source.Kind != "image" || (source.MimeType != "image/jpeg" && source.MimeType != "image/png") {
		return nil
	}

	object, err := processor.storage.GetObject(ctx, source.StorageKey)
	if err != nil {
		return fmt.Errorf("read source media: %w", err)
	}
	actualChecksum := checksum(object.Body)
	if source.ChecksumSHA256 == "" || actualChecksum != source.ChecksumSHA256 {
		return errors.New("source media checksum mismatch")
	}
	decoded, _, err := image.Decode(bytes.NewReader(object.Body))
	if err != nil {
		return fmt.Errorf("decode source media: %w", err)
	}
	thumbnail := resizeContain(decoded, processor.maxWidth, processor.maxHeight)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, thumbnail, &jpeg.Options{Quality: 82}); err != nil {
		return fmt.Errorf("encode thumbnail: %w", err)
	}
	key := fmt.Sprintf("projects/%d/derivatives/%d/thumbnail-v1.jpg", source.ProjectID, source.ID)
	stored, err := processor.storage.PutObject(ctx, key, bytes.NewReader(encoded.Bytes()), "image/jpeg")
	if err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}
	return processor.repository.SaveDerivative(ctx, tx, Derivative{
		SourceAssetID: source.ID, ProjectID: source.ProjectID, TeamID: source.TeamID,
		LogicalKey: source.LogicalKey + "/thumbnail", Version: source.Version,
		Kind: "image", MimeType: "image/jpeg", StorageKey: key, ObjectVersion: stored.VersionID,
		SizeBytes: int64(len(stored.Body)), ChecksumSHA256: stored.ChecksumSHA256,
		DerivativeType: "thumbnail", Generator: ThumbnailGeneratorVersion,
		Width: thumbnail.Bounds().Dx(), Height: thumbnail.Bounds().Dy(),
	})
}

func resizeContain(source image.Image, maxWidth, maxHeight int) *image.RGBA {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	scale := min(float64(maxWidth)/float64(width), float64(maxHeight)/float64(height))
	if scale > 1 {
		scale = 1
	}
	targetWidth := max(1, int(float64(width)*scale))
	targetHeight := max(1, int(float64(height)*scale))
	target := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := range targetHeight {
		for x := range targetWidth {
			sourceX := bounds.Min.X + x*width/targetWidth
			sourceY := bounds.Min.Y + y*height/targetHeight
			target.Set(x, y, color.RGBAModel.Convert(source.At(sourceX, sourceY)))
		}
	}
	return target
}

func checksum(body []byte) string {
	value := sha256.Sum256(body)
	return hex.EncodeToString(value[:])
}

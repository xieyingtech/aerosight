package media

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"

	"aerosight/worker/internal/outbox"
)

type memoryStorage struct {
	objects  map[string]Object
	putError error
}

func (storage *memoryStorage) GetObject(_ context.Context, key string) (Object, error) {
	object, ok := storage.objects[key]
	if !ok {
		return Object{}, errors.New("object not found")
	}
	return object, nil
}

func (storage *memoryStorage) PutObject(_ context.Context, key string, reader io.Reader, contentType string) (Object, error) {
	if storage.putError != nil {
		err := storage.putError
		storage.putError = nil
		return Object{}, err
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return Object{}, err
	}
	object := Object{Key: key, Body: body, ContentType: contentType, ChecksumSHA256: checksum(body), VersionID: "1"}
	storage.objects[key] = object
	return object, nil
}

type memoryRepository struct {
	source      SourceAsset
	derivatives map[string]Derivative
}

func (repository *memoryRepository) LoadSource(_ context.Context, _ *sql.Tx, projectID, assetID int) (SourceAsset, error) {
	if repository.source.ProjectID != projectID || repository.source.ID != assetID {
		return SourceAsset{}, sql.ErrNoRows
	}
	return repository.source, nil
}

func (repository *memoryRepository) SaveDerivative(_ context.Context, _ *sql.Tx, derivative Derivative) error {
	repository.derivatives[derivative.StorageKey] = derivative
	return nil
}

func imageFixture(t *testing.T, width, height int) []byte {
	t.Helper()
	fixture := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			fixture.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, fixture); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func derivativeFixture(t *testing.T, body []byte) (*Processor, *memoryStorage, *memoryRepository, outbox.Event) {
	t.Helper()
	storage := &memoryStorage{objects: map[string]Object{}}
	storage.objects["source/frame.png"] = Object{
		Key: "source/frame.png", Body: body, ContentType: "image/png", ChecksumSHA256: checksum(body),
	}
	repository := &memoryRepository{
		source: SourceAsset{ID: 42, ProjectID: 17, TeamID: 5, LogicalKey: "mission/frame", Version: 1,
			Kind: "image", MimeType: "image/png", StorageKey: "source/frame.png", ChecksumSHA256: checksum(body)},
		derivatives: map[string]Derivative{},
	}
	event := outbox.Event{ProjectID: 17, TeamID: 5, Payload: []byte(`{"assetId":42}`)}
	return NewProcessor(storage, repository), storage, repository, event
}

func TestValidImageCreatesBoundedThumbnailWithSourceLineage(t *testing.T) {
	processor, _, repository, event := derivativeFixture(t, imageFixture(t, 640, 480))
	if err := processor.Handler(context.Background(), nil, event); err != nil {
		t.Fatal(err)
	}
	derivative := repository.derivatives["projects/17/derivatives/42/thumbnail-v1.jpg"]
	if derivative.SourceAssetID != 42 || derivative.Width != 320 || derivative.Height != 240 {
		t.Fatalf("unexpected thumbnail lineage or dimensions: %#v", derivative)
	}
	if derivative.ChecksumSHA256 == "" || derivative.Generator != ThumbnailGeneratorVersion {
		t.Fatalf("thumbnail metadata incomplete: %#v", derivative)
	}
}

func TestDamagedImageFailsWithoutPublishingDerivative(t *testing.T) {
	processor, _, repository, event := derivativeFixture(t, []byte("not-an-image"))
	err := processor.Handler(context.Background(), nil, event)
	if err == nil || len(repository.derivatives) != 0 {
		t.Fatalf("damaged media should fail without a derivative: err=%v derivatives=%v", err, repository.derivatives)
	}
}

func TestTransientStorageFailureRetriesIdempotently(t *testing.T) {
	processor, storage, repository, event := derivativeFixture(t, imageFixture(t, 100, 50))
	storage.putError = errors.New("temporary storage outage")
	if err := processor.Handler(context.Background(), nil, event); err == nil {
		t.Fatal("first derivative attempt should fail")
	}
	if err := processor.Handler(context.Background(), nil, event); err != nil {
		t.Fatal(err)
	}
	if len(repository.derivatives) != 1 {
		t.Fatalf("retry should publish one derivative, got %d", len(repository.derivatives))
	}
}

func TestUnsupportedMediaCompletesWithoutDerivative(t *testing.T) {
	processor, _, repository, event := derivativeFixture(t, []byte("video"))
	repository.source.Kind = "video"
	repository.source.MimeType = "video/mp4"
	if err := processor.Handler(context.Background(), nil, event); err != nil {
		t.Fatal(err)
	}
	if len(repository.derivatives) != 0 {
		t.Fatal("unsupported media should not create a derivative")
	}
}

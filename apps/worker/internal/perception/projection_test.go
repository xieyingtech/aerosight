package perception

import (
	"context"
	"math"
	"testing"
)

type memoryProjectionRepository struct{ records []ProjectionRecord }

func (repository *memoryProjectionRepository) SaveProjection(_ context.Context, record ProjectionRecord) error {
	repository.records = append(repository.records, record)
	return nil
}

func TestKnownControlPointsProjectWithPoseAndCalibration(t *testing.T) {
	repository := &memoryProjectionRepository{}
	service := NewProjectionService(repository)
	record, err := service.ProjectAndSave(context.Background(), ProjectionInput{
		ProjectID: 2, AlgorithmRunID: "run-1", DetectionKey: "d-1",
		PixelGeometry: PixelGeometry{Type: "bbox", X: 500, Y: 500, Width: 100, Height: 100},
		Pose:          &DevicePose{Longitude: 120, Latitude: 30, AltitudeMeters: 100, HorizontalAccuracyMeters: 1, VerticalAccuracyMeters: 2},
		Calibration:   &CameraCalibration{Version: "camera-v3", FocalX: 1000, FocalY: 1000, CenterX: 500, CenterY: 500, AngularErrorDegrees: 0.5},
	})
	if err != nil {
		t.Fatal(err)
	}
	first := record.GeographicGeometry.Coordinates[0][0]
	second := record.GeographicGeometry.Coordinates[0][1]
	if math.Abs(first[0]-120) > 1e-9 || math.Abs(first[1]-30) > 1e-9 {
		t.Fatalf("center control point drifted: %v", first)
	}
	expectedLongitude := 120 + 10/(111_320*math.Cos(30*math.Pi/180))
	if math.Abs(second[0]-expectedLongitude) > 1e-9 {
		t.Fatalf("east control point drifted: got %v expected %v", second[0], expectedLongitude)
	}
	if record.Method != "nadir-ray-ground-plane" || record.Quality != "estimated" || record.HorizontalErrorMeters <= 0 || len(repository.records) != 1 {
		t.Fatalf("projection provenance not saved: %+v", record)
	}
}

func TestImageGeoreferenceTakesPrecedenceAndClosesPolygon(t *testing.T) {
	record, err := ProjectDetection(ProjectionInput{ProjectID: 2, AlgorithmRunID: "run-1", DetectionKey: "d-2",
		PixelGeometry: PixelGeometry{Type: "polygon", Coordinates: []Point{{0, 0}, {10, 0}, {10, 10}}},
		Georeference:  &ImageGeoreference{Version: "ortho-v2", Homography: [9]float64{0.00001, 0, 120, 0, 0.00001, 30, 0, 0, 1}, HorizontalErrorMeters: 0.2},
		Pose:          &DevicePose{Longitude: 0, Latitude: 0, AltitudeMeters: 1},
		Calibration:   &CameraCalibration{Version: "ignored", FocalX: 1, FocalY: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	ring := record.GeographicGeometry.Coordinates[0]
	if record.Method != "image-homography" || record.Quality != "surveyed" || len(ring) != 4 || ring[0] != ring[3] {
		t.Fatalf("georeference projection invalid: %+v", record)
	}
}

func TestMissingCalibrationPreservesImageGeometryWithoutInventingLocation(t *testing.T) {
	record, err := ProjectDetection(ProjectionInput{ProjectID: 2, AlgorithmRunID: "run-1", DetectionKey: "d-3",
		PixelGeometry: PixelGeometry{Type: "bbox", X: 1, Y: 2, Width: 3, Height: 4}, Pose: &DevicePose{Longitude: 120, Latitude: 30, AltitudeMeters: 80}})
	if err != nil {
		t.Fatal(err)
	}
	if record.GeographicGeometry != nil || record.Quality != "unavailable" || record.DegradationReason == "" {
		t.Fatalf("missing calibration invented a map location: %+v", record)
	}
	if record.PixelGeometry.Width != 3 {
		t.Fatal("image-level evidence was not retained")
	}
}

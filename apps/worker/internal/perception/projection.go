package perception

import (
	"context"
	"errors"
	"math"
)

const ProjectionVersionV1 = "aerosight-geo-projection/v1"

type Point struct{ X, Y float64 }

type PixelGeometry struct {
	Type                string
	X, Y, Width, Height float64
	Coordinates         []Point
}

type DevicePose struct {
	Longitude, Latitude, AltitudeMeters float64
	YawDegrees                          float64
	HorizontalAccuracyMeters            float64
	VerticalAccuracyMeters              float64
}

type CameraCalibration struct {
	Version                          string
	FocalX, FocalY, CenterX, CenterY float64
	AngularErrorDegrees              float64
}

type ImageGeoreference struct {
	Version string
	// Homography maps [pixelX,pixelY,1] to [longitude*w,latitude*w,w].
	Homography            [9]float64
	HorizontalErrorMeters float64
}

type ProjectionInput struct {
	ProjectID                    int
	AlgorithmRunID, DetectionKey string
	PixelGeometry                PixelGeometry
	Pose                         *DevicePose
	Calibration                  *CameraCalibration
	Georeference                 *ImageGeoreference
}

type GeographicGeometry struct {
	Type                  string         `json:"type"`
	Coordinates           [][][2]float64 `json:"coordinates"`
	Quality               string         `json:"quality"`
	Method                string         `json:"method"`
	HorizontalErrorMeters float64        `json:"horizontalErrorMeters"`
	TransformVersion      string         `json:"transformVersion"`
}

type ProjectionRecord struct {
	ProjectID                         int
	AlgorithmRunID, DetectionKey      string
	PixelGeometry                     PixelGeometry
	GeographicGeometry                *GeographicGeometry
	Method, Quality, TransformVersion string
	HorizontalErrorMeters             float64
	DegradationReason                 string
}

type ProjectionRepository interface {
	SaveProjection(context.Context, ProjectionRecord) error
}

type ProjectionService struct{ repository ProjectionRepository }

func NewProjectionService(repository ProjectionRepository) *ProjectionService {
	return &ProjectionService{repository: repository}
}

func (service *ProjectionService) ProjectAndSave(ctx context.Context, input ProjectionInput) (ProjectionRecord, error) {
	record, err := ProjectDetection(input)
	if err != nil {
		return ProjectionRecord{}, err
	}
	if service.repository != nil {
		if err := service.repository.SaveProjection(ctx, record); err != nil {
			return ProjectionRecord{}, err
		}
	}
	return record, nil
}

func ProjectDetection(input ProjectionInput) (ProjectionRecord, error) {
	points, err := geometryPoints(input.PixelGeometry)
	if err != nil {
		return ProjectionRecord{}, err
	}
	record := ProjectionRecord{ProjectID: input.ProjectID, AlgorithmRunID: input.AlgorithmRunID,
		DetectionKey: input.DetectionKey, PixelGeometry: input.PixelGeometry, TransformVersion: ProjectionVersionV1}
	if input.Georeference != nil {
		coordinates := make([][2]float64, 0, len(points)+1)
		for _, point := range points {
			longitude, latitude, ok := applyHomography(input.Georeference.Homography, point)
			if !ok {
				return ProjectionRecord{}, errors.New("invalid image georeference homography")
			}
			coordinates = append(coordinates, [2]float64{longitude, latitude})
		}
		coordinates = closeRing(coordinates)
		record.Method, record.Quality = "image-homography", "surveyed"
		record.TransformVersion = ProjectionVersionV1 + ":" + input.Georeference.Version
		record.HorizontalErrorMeters = max(0, input.Georeference.HorizontalErrorMeters)
		record.GeographicGeometry = geographicGeometry(coordinates, record)
		return record, nil
	}
	if input.Pose == nil || input.Calibration == nil {
		record.Method, record.Quality, record.DegradationReason = "image-only", "unavailable", "missing pose, camera calibration, or image georeference"
		return record, nil
	}
	pose, calibration := input.Pose, input.Calibration
	if pose.AltitudeMeters <= 0 || calibration.FocalX <= 0 || calibration.FocalY <= 0 || calibration.Version == "" {
		record.Method, record.Quality, record.DegradationReason = "image-only", "unavailable", "invalid altitude or camera calibration"
		return record, nil
	}
	coordinates := make([][2]float64, 0, len(points)+1)
	yaw := pose.YawDegrees * math.Pi / 180
	metersPerLatitudeDegree := 111_320.0
	metersPerLongitudeDegree := metersPerLatitudeDegree * math.Cos(pose.Latitude*math.Pi/180)
	if math.Abs(metersPerLongitudeDegree) < 1 {
		return ProjectionRecord{}, errors.New("pose latitude cannot be projected")
	}
	for _, point := range points {
		right := (point.X - calibration.CenterX) / calibration.FocalX * pose.AltitudeMeters
		forward := -(point.Y - calibration.CenterY) / calibration.FocalY * pose.AltitudeMeters
		east := right*math.Cos(yaw) + forward*math.Sin(yaw)
		north := -right*math.Sin(yaw) + forward*math.Cos(yaw)
		coordinates = append(coordinates, [2]float64{pose.Longitude + east/metersPerLongitudeDegree, pose.Latitude + north/metersPerLatitudeDegree})
	}
	coordinates = closeRing(coordinates)
	angularError := pose.AltitudeMeters * math.Tan(max(0, calibration.AngularErrorDegrees)*math.Pi/180)
	record.HorizontalErrorMeters = math.Sqrt(pose.HorizontalAccuracyMeters*pose.HorizontalAccuracyMeters + pose.VerticalAccuracyMeters*pose.VerticalAccuracyMeters + angularError*angularError)
	record.Method, record.Quality = "nadir-ray-ground-plane", "estimated"
	if record.HorizontalErrorMeters > 25 {
		record.Quality = "low"
	}
	record.TransformVersion = ProjectionVersionV1 + ":" + calibration.Version
	record.GeographicGeometry = geographicGeometry(coordinates, record)
	return record, nil
}

func geometryPoints(geometry PixelGeometry) ([]Point, error) {
	switch geometry.Type {
	case "bbox":
		if geometry.X < 0 || geometry.Y < 0 || geometry.Width <= 0 || geometry.Height <= 0 {
			return nil, errors.New("invalid pixel bbox")
		}
		return []Point{{geometry.X, geometry.Y}, {geometry.X + geometry.Width, geometry.Y}, {geometry.X + geometry.Width, geometry.Y + geometry.Height}, {geometry.X, geometry.Y + geometry.Height}}, nil
	case "polygon":
		if len(geometry.Coordinates) < 3 {
			return nil, errors.New("pixel polygon requires at least three points")
		}
		for _, point := range geometry.Coordinates {
			if point.X < 0 || point.Y < 0 {
				return nil, errors.New("invalid pixel polygon")
			}
		}
		return geometry.Coordinates, nil
	default:
		return nil, errors.New("unsupported pixel geometry")
	}
}

func applyHomography(matrix [9]float64, point Point) (float64, float64, bool) {
	w := matrix[6]*point.X + matrix[7]*point.Y + matrix[8]
	if math.Abs(w) < 1e-12 {
		return 0, 0, false
	}
	longitude := (matrix[0]*point.X + matrix[1]*point.Y + matrix[2]) / w
	latitude := (matrix[3]*point.X + matrix[4]*point.Y + matrix[5]) / w
	return longitude, latitude, longitude >= -180 && longitude <= 180 && latitude >= -90 && latitude <= 90
}

func closeRing(coordinates [][2]float64) [][2]float64 {
	if len(coordinates) > 0 && coordinates[0] != coordinates[len(coordinates)-1] {
		coordinates = append(coordinates, coordinates[0])
	}
	return coordinates
}

func geographicGeometry(coordinates [][2]float64, record ProjectionRecord) *GeographicGeometry {
	return &GeographicGeometry{Type: "Polygon", Coordinates: [][][2]float64{coordinates}, Quality: record.Quality,
		Method: record.Method, HorizontalErrorMeters: record.HorizontalErrorMeters, TransformVersion: record.TransformVersion}
}

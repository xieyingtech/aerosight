package algorithm

import (
	"encoding/json"
	"errors"
	"fmt"
)

type CanonicalKind string

const (
	ResultClassification CanonicalKind = "classification"
	ResultDetection      CanonicalKind = "detection"
	ResultSegmentation   CanonicalKind = "segmentation"
	ResultKeypoints      CanonicalKind = "keypoints"
	ResultTracking       CanonicalKind = "tracking"
	ResultOCR            CanonicalKind = "ocr"
	ResultScalar         CanonicalKind = "scalar"
	ResultTable          CanonicalKind = "table"
	ResultAsset          CanonicalKind = "asset"
	ResultCustom         CanonicalKind = "custom"
)

type Classification struct {
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
}

type Segmentation struct {
	MaskAssetRef string   `json:"maskAssetRef"`
	Labels       []string `json:"labels,omitempty"`
}

type Point struct {
	Name       string  `json:"name,omitempty"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Confidence float64 `json:"confidence,omitempty"`
}

type KeypointSet struct {
	SubjectKey string  `json:"subjectKey,omitempty"`
	Points     []Point `json:"points"`
}

type Track struct {
	TrackKey string  `json:"trackKey"`
	Label    string  `json:"label,omitempty"`
	Path     []Point `json:"path"`
}

type OCRBlock struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence,omitempty"`
	Geometry   any     `json:"geometry,omitempty"`
}

type OCRResult struct {
	Text   string     `json:"text"`
	Blocks []OCRBlock `json:"blocks,omitempty"`
}

type ScalarResult struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit,omitempty"`
}

type TableResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

type AssetResult struct {
	AssetRef string `json:"assetRef"`
	MIMEType string `json:"mimeType,omitempty"`
}

type CanonicalResult struct {
	Kind           CanonicalKind   `json:"kind"`
	Classification *Classification `json:"classification,omitempty"`
	Detections     []Detection     `json:"detections,omitempty"`
	Segmentation   *Segmentation   `json:"segmentation,omitempty"`
	Keypoints      []KeypointSet   `json:"keypoints,omitempty"`
	Tracks         []Track         `json:"tracks,omitempty"`
	OCR            *OCRResult      `json:"ocr,omitempty"`
	Scalar         *ScalarResult   `json:"scalar,omitempty"`
	Table          *TableResult    `json:"table,omitempty"`
	Asset          *AssetResult    `json:"asset,omitempty"`
	Custom         json.RawMessage `json:"custom,omitempty"`
}

type CanonicalMapping struct {
	Kind       CanonicalKind `json:"kind"`
	ResultPath string        `json:"resultPath"`
}

func MapCanonicalResult(raw []byte, mapping CanonicalMapping) (CanonicalResult, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return CanonicalResult{}, fmt.Errorf("%w: invalid JSON", ErrFormatDrift)
	}
	value := payload
	if mapping.ResultPath != "" {
		var exists bool
		value, exists = pathValue(payload, mapping.ResultPath)
		if !exists {
			return CanonicalResult{}, fmt.Errorf("%w: result path %q is missing", ErrFormatDrift, mapping.ResultPath)
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return CanonicalResult{}, err
	}
	result := CanonicalResult{Kind: mapping.Kind}
	switch mapping.Kind {
	case ResultClassification:
		result.Classification = &Classification{}
		err = json.Unmarshal(encoded, result.Classification)
	case ResultDetection:
		err = json.Unmarshal(encoded, &result.Detections)
	case ResultSegmentation:
		result.Segmentation = &Segmentation{}
		err = json.Unmarshal(encoded, result.Segmentation)
	case ResultKeypoints:
		err = json.Unmarshal(encoded, &result.Keypoints)
	case ResultTracking:
		err = json.Unmarshal(encoded, &result.Tracks)
	case ResultOCR:
		result.OCR = &OCRResult{}
		err = json.Unmarshal(encoded, result.OCR)
	case ResultScalar:
		result.Scalar = &ScalarResult{}
		err = json.Unmarshal(encoded, result.Scalar)
	case ResultTable:
		result.Table = &TableResult{}
		err = json.Unmarshal(encoded, result.Table)
	case ResultAsset:
		result.Asset = &AssetResult{}
		err = json.Unmarshal(encoded, result.Asset)
	case ResultCustom:
		result.Custom = append(json.RawMessage(nil), encoded...)
	default:
		return CanonicalResult{}, fmt.Errorf("%w: unsupported canonical result kind %q", ErrFormatDrift, mapping.Kind)
	}
	if err != nil {
		return CanonicalResult{}, fmt.Errorf("%w: canonical %s payload is invalid: %v", ErrFormatDrift, mapping.Kind, err)
	}
	if err := result.Validate(); err != nil {
		return CanonicalResult{}, fmt.Errorf("%w: %v", ErrFormatDrift, err)
	}
	return result, nil
}

func validConfidence(value float64) bool { return value >= 0 && value <= 1 }

func (result CanonicalResult) Validate() error {
	switch result.Kind {
	case ResultClassification:
		if result.Classification == nil || result.Classification.Label == "" || !validConfidence(result.Classification.Confidence) {
			return errors.New("classification requires label and confidence between zero and one")
		}
	case ResultDetection:
		for _, detection := range result.Detections {
			if detection.DetectionKey == "" || detection.Label == "" || !validConfidence(detection.Confidence) {
				return errors.New("detection requires key, label, and confidence between zero and one")
			}
		}
	case ResultSegmentation:
		if result.Segmentation == nil || result.Segmentation.MaskAssetRef == "" {
			return errors.New("segmentation requires a mask asset reference")
		}
	case ResultKeypoints:
		if len(result.Keypoints) == 0 {
			return errors.New("keypoints result requires at least one point set")
		}
		for _, set := range result.Keypoints {
			if len(set.Points) == 0 {
				return errors.New("keypoint set cannot be empty")
			}
		}
	case ResultTracking:
		for _, track := range result.Tracks {
			if track.TrackKey == "" || len(track.Path) == 0 {
				return errors.New("track requires stable key and path")
			}
		}
	case ResultOCR:
		if result.OCR == nil || (result.OCR.Text == "" && len(result.OCR.Blocks) == 0) {
			return errors.New("OCR result requires text or blocks")
		}
	case ResultScalar:
		if result.Scalar == nil {
			return errors.New("scalar result is missing")
		}
	case ResultTable:
		if result.Table == nil || len(result.Table.Columns) == 0 {
			return errors.New("table result requires columns")
		}
		for _, row := range result.Table.Rows {
			if len(row) != len(result.Table.Columns) {
				return errors.New("table row width does not match columns")
			}
		}
	case ResultAsset:
		if result.Asset == nil || result.Asset.AssetRef == "" {
			return errors.New("asset result requires an asset reference")
		}
	case ResultCustom:
		if len(result.Custom) == 0 || !json.Valid(result.Custom) {
			return errors.New("custom result requires valid JSON")
		}
	default:
		return errors.New("unsupported canonical result kind")
	}
	return nil
}

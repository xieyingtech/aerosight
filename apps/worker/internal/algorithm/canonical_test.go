package algorithm

import (
	"errors"
	"testing"
)

func TestCanonicalResultUnionCoversGenericAlgorithmOutputs(t *testing.T) {
	tests := []struct {
		kind CanonicalKind
		raw  string
	}{
		{ResultClassification, `{"result":{"label":"clear","confidence":0.92}}`},
		{ResultDetection, `{"result":[{"detectionKey":"d1","label":"vehicle","confidence":0.8}]}`},
		{ResultSegmentation, `{"result":{"maskAssetRef":"asset://mask/1","labels":["road"]}}`},
		{ResultKeypoints, `{"result":[{"subjectKey":"person-1","points":[{"name":"head","x":1,"y":2,"confidence":0.9}]}]}`},
		{ResultTracking, `{"result":[{"trackKey":"track-1","label":"car","path":[{"x":1,"y":2}]}]}`},
		{ResultOCR, `{"result":{"text":"AEROSIGHT","blocks":[]}}`},
		{ResultScalar, `{"result":{"value":23.5,"unit":"celsius"}}`},
		{ResultTable, `{"result":{"columns":["name","count"],"rows":[["car",2]]}}`},
		{ResultAsset, `{"result":{"assetRef":"asset://derived/9","mimeType":"image/png"}}`},
		{ResultCustom, `{"result":{"vendorExtension":{"score":7}}}`},
	}
	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			result, err := MapCanonicalResult([]byte(test.raw), CanonicalMapping{Kind: test.kind, ResultPath: "result"})
			if err != nil || result.Kind != test.kind {
				t.Fatalf("map %s: %+v err=%v", test.kind, result, err)
			}
		})
	}
}

func TestCanonicalMappingFailsClosedOnInvalidShape(t *testing.T) {
	if _, err := MapCanonicalResult([]byte(`{"result":{"label":"car","confidence":4}}`), CanonicalMapping{Kind: ResultClassification, ResultPath: "result"}); !errors.Is(err, ErrFormatDrift) {
		t.Fatalf("invalid classification was accepted: %v", err)
	}
	if _, err := MapCanonicalResult([]byte(`{"result":{"columns":["a","b"],"rows":[[1]]}}`), CanonicalMapping{Kind: ResultTable, ResultPath: "result"}); !errors.Is(err, ErrFormatDrift) {
		t.Fatalf("invalid table was accepted: %v", err)
	}
}

package report

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReportContentRetainsStepsConditionsIssuesAndAssets(t *testing.T) {
	content := reportContent{
		SchemaVersion: "task-report-v1",
		TaskRun:       map[string]any{"id": 9, "taskVersionId": 4},
		Steps: []stepFact{{ID: 2, Key: "detect", Uses: "algorithm.run", Status: "succeeded",
			Condition: json.RawMessage(`{"result":true}`), Output: json.RawMessage(`{"detections":2}`)}},
		Issues:   []issueFact{{ID: 3, Number: 7, Title: "疑似违建", Status: "open", Conclusion: "需人工复核"}},
		Assets:   []assetFact{{ID: 5, Version: 2, Kind: "image", Checksum: "sha256"}},
		DataGaps: []string{},
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"detect", "algorithm.run", "result", "疑似违建", "需人工复核", "sha256"} {
		if !json.Valid(encoded) || !strings.Contains(string(encoded), marker) {
			t.Fatalf("report omitted %q: %s", marker, encoded)
		}
	}
}

package flighthub

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEndpointManifestCoverage(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve endpoint manifest path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/endpoints.tsv")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = 11
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 90 {
		t.Fatalf("manifest rows = %d, want header + 89 endpoints", len(rows))
	}
	wantHeader := []string{"id", "method", "path", "status", "title", "domain", "scope", "risk", "pagination", "deployment", "verification"}
	if strings.Join(rows[0], "|") != strings.Join(wantHeader, "|") {
		t.Fatalf("unexpected manifest header: %#v", rows[0])
	}
	methodCounts := map[string]int{}
	identities := map[string]bool{}
	validRisks := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	for index, row := range rows[1:] {
		methodCounts[row[1]]++
		identity := row[1] + " " + row[2]
		if identities[identity] {
			t.Fatalf("duplicate endpoint at row %d: %s", index+2, identity)
		}
		identities[identity] = true
		if row[3] != "released" || row[4] == "" || row[5] == "" || row[6] == "" || !validRisks[row[7]] || row[8] == "" || row[9] == "" || row[10] == "" {
			t.Fatalf("incomplete endpoint metadata at row %d: %#v", index+2, row)
		}
		if row[9] != "cn-public-cloud" {
			t.Fatalf("unexpected deployment at row %d: %s", index+2, row[9])
		}
	}
	wantMethods := map[string]int{"GET": 59, "POST": 19, "PUT": 6, "DELETE": 5}
	for method, want := range wantMethods {
		if got := methodCounts[method]; got != want {
			t.Fatalf("%s endpoints = %d, want %d", method, got, want)
		}
	}
}

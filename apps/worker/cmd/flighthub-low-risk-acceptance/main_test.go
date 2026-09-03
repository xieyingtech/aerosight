package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"aerosight/worker/internal/flighthub"
)

func TestLowRiskAcceptanceRequiresExplicitOptInAndScopedConnector(t *testing.T) {
	t.Parallel()
	if !validInvocation(true, 1123, 7, nil) {
		t.Fatal("valid explicitly confirmed invocation was rejected")
	}
	for _, input := range []struct {
		confirmed   bool
		projectID   int
		connectorID int64
		arguments   []string
	}{
		{false, 1123, 7, nil}, {true, 0, 7, nil}, {true, 1123, 0, nil}, {true, 1123, 7, []string{"flight-task"}},
	} {
		if validInvocation(input.confirmed, input.projectID, input.connectorID, input.arguments) {
			t.Fatalf("unsafe invocation was accepted: %#v", input)
		}
	}
}

func TestNormalizeCLIArgsAcceptsPNPMScriptSeparator(t *testing.T) {
	got := normalizeCLIArgs([]string{"acceptance", "--", "--project-id", "1"})
	want := []string{"acceptance", "--project-id", "1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized args=%#v want=%#v", got, want)
	}
}

func TestLowRiskAcceptanceGeneratesOpaqueRequestAndFileIDs(t *testing.T) {
	t.Parallel()
	first, second := acceptanceRequestID(), acceptanceRequestID()
	fileID := acceptanceFileUUID()
	if len(first) != 32 || len(second) != 32 || first == second || !strings.HasPrefix(fileID, "aerosight-acceptance-") || len(fileID) != 53 {
		t.Fatalf("invalid acceptance ids: %q %q %q", first, second, fileID)
	}
}

func TestLowRiskAcceptanceEmitsOnlySafePersistenceErrors(t *testing.T) {
	t.Parallel()
	if got := safeAcceptanceError(&flighthub.APIError{SafeCode: "acceptance_incomplete"}); got != "acceptance_incomplete" {
		t.Fatalf("category=%q", got)
	}
	if got := safeAcceptanceError(errors.New("SECRET_DATABASE_DETAIL")); got != "evidence_unavailable" || strings.Contains(got, "SECRET") {
		t.Fatalf("unsafe category=%q", got)
	}
}

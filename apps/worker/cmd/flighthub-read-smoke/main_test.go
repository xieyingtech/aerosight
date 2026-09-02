package main

import "testing"

func TestReadSmokeRequiresExplicitOptInAndScopedConnector(t *testing.T) {
	t.Parallel()
	if !validInvocation(true, 11, 7, nil) {
		t.Fatal("valid explicitly confirmed invocation was rejected")
	}
	for _, input := range []struct {
		confirmed   bool
		projectID   int
		connectorID int64
		arguments   []string
	}{
		{false, 11, 7, nil}, {true, 0, 7, nil}, {true, 11, 0, nil}, {true, 11, 7, []string{"secret"}},
	} {
		if validInvocation(input.confirmed, input.projectID, input.connectorID, input.arguments) {
			t.Fatalf("unsafe invocation was accepted: %#v", input)
		}
	}
}

func TestReadSmokeGeneratesOpaqueRequestIDs(t *testing.T) {
	t.Parallel()
	first, second := smokeRequestID(), smokeRequestID()
	if len(first) != 32 || len(second) != 32 || first == second {
		t.Fatalf("invalid smoke request ids: %q %q", first, second)
	}
}

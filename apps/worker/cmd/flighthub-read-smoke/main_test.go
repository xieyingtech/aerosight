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

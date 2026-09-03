package main

import (
	"testing"
)

func TestAcceptanceRunID(t *testing.T) {
	first, second := acceptanceRunID(), acceptanceRunID()
	if len(first) != 32 || len(second) != 32 || first == second {
		t.Fatalf("invalid acceptance run ids: %q %q", first, second)
	}
}

package main

import (
	"reflect"
	"testing"
)

func TestAcceptanceRunID(t *testing.T) {
	first, second := acceptanceRunID(), acceptanceRunID()
	if len(first) != 32 || len(second) != 32 || first == second {
		t.Fatalf("invalid acceptance run ids: %q %q", first, second)
	}
}

func TestNormalizeCLIArgsAcceptsPNPMScriptSeparator(t *testing.T) {
	got := normalizeCLIArgs([]string{"acceptance", "--", "--project-id", "1"})
	want := []string{"acceptance", "--project-id", "1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized args=%#v want=%#v", got, want)
	}
}

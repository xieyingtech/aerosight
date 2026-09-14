package taskdefinition

import (
	"reflect"
	"strings"
	"testing"
)

func TestFormatsHaveTheSameSemanticHash(t *testing.T) {
	yaml, err := Parse("yaml", "name: 巡检\ntrigger: {type: manual}\nsteps:\n  - key: report\n    uses: report.generate\n")
	if err != nil {
		t.Fatal(err)
	}
	json, err := Parse("json", `{"steps":[{"uses":"report.generate","key":"report"}],"trigger":{"type":"manual"},"name":"巡检"}`)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := Hash(yaml)
	b, _ := Hash(json)
	if !reflect.DeepEqual(yaml, json) || a != b {
		t.Fatal("format changed definition semantics")
	}
}
func TestUnsafeOrAmbiguousSourcesAreRejected(t *testing.T) {
	for _, source := range []string{
		"name: a\nname: b", "base: &base {a: 1}\ncopy: *base", "name: !!str x", "name: !custom x", "name: a\n---\nname: b",
		"name: .inf", "name: .nan", "123: value", "x: 9007199254740992", "x: {<<: {name: value}}",
		"- item", "", strings.Repeat("a", MaxSourceBytes+1), "value: " + strings.Repeat("[", 70) + "0" + strings.Repeat("]", 70),
	} {
		t.Run(source[:min(len(source), 30)], func(t *testing.T) {
			if _, err := Parse("yaml", source); err == nil {
				t.Fatal("accepted unsafe source")
			}
		})
	}
	if _, err := Parse("json", `{"name":"a","name":"b"}`); err == nil {
		t.Fatal("accepted duplicate JSON keys")
	}
	if _, err := Parse("json", "name: yaml"); err == nil {
		t.Fatal("accepted YAML as JSON")
	}
}

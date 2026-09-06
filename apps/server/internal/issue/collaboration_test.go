package issue

import (
	"reflect"
	"strings"
	"testing"
)

func TestCollaborationPlanPermissionsAndVersions(t *testing.T) {
	permissions := map[string]bool{"issue:handle": true}
	if _, err := PlanMutation(Mutation{Action: "assign", AssigneeType: "user", AssigneeID: 4}, permissions, 1, 1); err == nil || err.Error() != "PROJECT_ACCESS_DENIED" {
		t.Fatalf("assignment permission %v", err)
	}
	if _, err := PlanMutation(Mutation{Action: "comment", Body: "comment"}, permissions, 2, 1); err == nil || err.Error() != "ISSUE_VERSION_CONFLICT" {
		t.Fatalf("version %v", err)
	}
	p, err := PlanMutation(Mutation{Action: "comment", Body: "  Comment  "}, permissions, 1, 1)
	if err != nil || p.NextVersion != 2 || p.EventType != "comment.created" || *p.Body != "Comment" {
		t.Fatalf("comment %+v %v", p, err)
	}
	p, err = PlanMutation(Mutation{Action: "labels", Labels: []string{" A ", "", "A", "b"}}, permissions, 1, 1)
	if err != nil || !reflect.DeepEqual(p.Labels, []string{"A", "b"}) {
		t.Fatalf("labels %+v %v", p, err)
	}
	for _, m := range []Mutation{{Action: "comment", Body: " "}, {Action: "comment", Body: strings.Repeat("😀", 2501)}, {Action: "labels", Labels: []string{strings.Repeat("😀", 26)}}} {
		if _, err = PlanMutation(m, permissions, 1, 1); err == nil {
			t.Fatalf("invalid accepted %+v", m)
		}
	}
	permissions["issue:assign"] = true
	p, err = PlanMutation(Mutation{Action: "assign", AssigneeType: "agent", AssigneeID: 4}, permissions, 1, 1)
	if err != nil || p.Assignment == nil || p.EventType != "assignee.added" {
		t.Fatalf("assign %+v %v", p, err)
	}
	if AssignmentChangeRequired("assign", true) || AssignmentChangeRequired("unassign", false) || !AssignmentChangeRequired("unassign", true) {
		t.Fatal("assignment no-op semantics changed")
	}
	if !IsCopilotAgent(" Copilot ", "") || !IsCopilotAgent("Assistant", "copilot") || IsCopilotAgent("Copilot helper", "") {
		t.Fatal("Copilot identity")
	}
}
func TestCopilotMentionVisibilityAndUnicode(t *testing.T) {
	for _, text := range []string{"@copilot 请分析", "你好，@COPILOT!", "x\n@copilot", "`ignored` @copilot", "\\`@copilot"} {
		if !HasActionableCopilotMention(text) {
			t.Errorf("missing mention %q", text)
		}
	}
	for _, text := range []string{"name@copilot", "中@copilot", "1@copilot", "_@copilot", "@copilot-name", "@copilot2", "@copilot中", "`@copilot`", "> @copilot", "```go\n@copilot\n```", "~~~\n@copilot\n~~~"} {
		if HasActionableCopilotMention(text) {
			t.Errorf("unexpected mention %q", text)
		}
	}
	if ShouldQueueCopilotMention("@copilot", map[string]bool{}) || !ShouldQueueCopilotMention("@copilot", map[string]bool{"agent:use": true}) {
		t.Fatal("agent use permission")
	}
}

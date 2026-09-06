package issue

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf16"
)

type Mutation struct {
	Action       string   `json:"action"`
	Body         string   `json:"body,omitempty"`
	Status       string   `json:"status,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	AssigneeType string   `json:"assigneeType,omitempty"`
	AssigneeID   int32    `json:"assigneeId,omitempty"`
}
type MutationPlan struct {
	EventType   string
	Body        *string
	Metadata    map[string]any
	NextVersion int32
	Status      *string
	Labels      []string
	Assignment  *Mutation
}

func MutationPermission(action string) string {
	if action == "assign" || action == "unassign" {
		return "issue:assign"
	}
	return "issue:handle"
}
func PlanMutation(m Mutation, permissions map[string]bool, actual, expected int32) (MutationPlan, error) {
	p := MutationPlan{NextVersion: actual + 1, Metadata: map[string]any{}}
	fail := func(code string) (MutationPlan, error) { return MutationPlan{}, errors.New(code) }
	if !permissions[MutationPermission(m.Action)] {
		return fail("PROJECT_ACCESS_DENIED")
	}
	if actual != expected {
		return fail("ISSUE_VERSION_CONFLICT")
	}
	switch m.Action {
	case "comment":
		body := strings.TrimSpace(m.Body)
		if body == "" || len(utf16.Encode([]rune(body))) > 5000 {
			return fail("ISSUE_COMMENT_INVALID")
		}
		p.EventType = "comment.created"
		p.Body = &body
	case "status":
		if m.Status != "open" && m.Status != "closed" {
			return fail("ISSUE_STATUS_INVALID")
		}
		p.EventType = "status.changed"
		p.Status = &m.Status
		p.Metadata["status"] = m.Status
	case "labels":
		labels := []string{}
		seen := map[string]bool{}
		for _, label := range m.Labels {
			label = strings.TrimSpace(label)
			if label == "" || seen[label] {
				continue
			}
			if len(utf16.Encode([]rune(label))) > 50 {
				return fail("ISSUE_LABELS_INVALID")
			}
			seen[label] = true
			labels = append(labels, label)
		}
		if len(labels) > 20 {
			return fail("ISSUE_LABELS_INVALID")
		}
		p.EventType = "labels.changed"
		p.Labels = labels
		p.Metadata["labels"] = labels
	case "assign", "unassign":
		if m.AssigneeID <= 0 || (m.AssigneeType != "user" && m.AssigneeType != "agent") {
			return fail("ISSUE_ASSIGNEE_INVALID")
		}
		p.EventType = "assignee.added"
		if m.Action == "unassign" {
			p.EventType = "assignee.removed"
		}
		p.Assignment = &m
		p.Metadata["assigneeType"] = m.AssigneeType
		p.Metadata["assigneeId"] = m.AssigneeID
	default:
		return fail("ISSUE_MUTATION_INVALID")
	}
	return p, nil
}
func AssignmentChangeRequired(action string, active bool) bool {
	if action == "assign" {
		return !active
	}
	return active
}
func IsCopilotAgent(name, kind string) bool {
	return kind == "copilot" || strings.EqualFold(strings.TrimSpace(name), "copilot")
}

// Match only visible mentions: quotations, fenced code, inline code, email
// addresses and longer handles are not requests to run an agent.
func HasActionableCopilotMention(markdown string) bool {
	var visible strings.Builder
	var fence byte
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		var opening byte
		if strings.HasPrefix(trimmed, "```") {
			opening = '`'
		} else if strings.HasPrefix(trimmed, "~~~") {
			opening = '~'
		}
		if opening != 0 {
			if fence == opening {
				fence = 0
			} else if fence == 0 {
				fence = opening
			}
			continue
		}
		if fence != 0 || strings.HasPrefix(trimmed, ">") {
			continue
		}
		inCode := false
		for i, ch := range line {
			if ch == '`' && (i == 0 || line[i-1] != '\\') {
				inCode = !inCode
				continue
			}
			if !inCode {
				visible.WriteRune(ch)
			}
		}
		visible.WriteByte('\n')
	}
	chars := []rune(visible.String())
	word := func(ch rune) bool { return unicode.IsLetter(ch) || unicode.IsNumber(ch) || ch == '_' }
	for i, ch := range chars {
		if ch != '@' || i+8 > len(chars) || (i > 0 && word(chars[i-1])) {
			continue
		}
		if !strings.EqualFold(string(chars[i:i+8]), "@copilot") {
			continue
		}
		if i+8 < len(chars) && (word(chars[i+8]) || chars[i+8] == '-') {
			continue
		}
		return true
	}
	return false
}
func ShouldQueueCopilotMention(markdown string, permissions map[string]bool) bool {
	return permissions["agent:use"] && HasActionableCopilotMention(markdown)
}

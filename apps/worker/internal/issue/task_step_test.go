package issue

import "testing"

func TestTaskIssueKeyIsStableAndProjectScoped(t *testing.T) {
	first := taskIssueKey(17, 31, "confidence-v1", "detection-group:9")
	if first != taskIssueKey(17, 31, "confidence-v1", "detection-group:9") {
		t.Fatal("repeated task issue changed its idempotency key")
	}
	if first == taskIssueKey(18, 31, "confidence-v1", "detection-group:9") {
		t.Fatal("different project reused task issue key")
	}
}

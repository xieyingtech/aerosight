package automation

func ShouldRunAutomaticDrafts(mode string) bool {
	return mode == "agent-auto-draft" || mode == "follow-up-draft"
}

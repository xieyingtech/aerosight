package httpapi

import (
	_ "embed"
	"encoding/json"
	"github.com/gin-gonic/gin"
)

// Mirrors the existing UI action catalog; decoded afresh so projections cannot mutate other responses.
//
//go:embed capability_actions.json
var actionCatalog []byte

func capabilityActions(code string) []gin.H {
	var catalog map[string][]gin.H
	if err := json.Unmarshal(actionCatalog, &catalog); err != nil {
		panic("invalid embedded action catalog")
	}
	return catalog[code]
}

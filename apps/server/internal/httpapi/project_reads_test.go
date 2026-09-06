package httpapi

import (
	"github.com/gin-gonic/gin"
	"net/url"
	"testing"
	"time"
)

func TestReplayWindowValidation(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ query, code string }{{"from=2026-09-01&to=2026-09-09", "REPLAY_WINDOW_TOO_LARGE"}, {"from=invalid", "INVALID_REPLAY_WINDOW"}, {"bbox=NaN,0,1,1", "INVALID_REPLAY_BBOX"}, {"from=2026-09-06&to=2026-09-06", "INVALID_REPLAY_WINDOW"}} {
		values, _ := url.ParseQuery(tc.query)
		_, err := parseReplay(values, now)
		if err == nil || err.Error() != tc.code {
			t.Fatalf("%s: %v", tc.query, err)
		}
	}
	input, err := parseReplay(url.Values{}, now)
	if err != nil || input.To.Sub(input.From) != time.Hour || input.DeviceTypes == nil || input.BBox != nil {
		t.Fatalf("default %+v %v", input, err)
	}
}

func TestDeviceTreeCycleAndDetachedNodes(t *testing.T) {
	devices := []gin.H{{"id": 1}, {"id": 2}, {"id": 3}, {"id": 4}}
	relations := []gin.H{{"fromDeviceId": 1, "toDeviceId": 2, "relationType": "mounted"}, {"fromDeviceId": 2, "toDeviceId": 1, "relationType": "linked"}, {"fromDeviceId": 4, "toDeviceId": 99, "relationType": "missing"}}
	roots := buildDeviceTree(devices, relations)
	if len(roots) != 4 {
		t.Fatalf("cycle recovery roots %+v", roots)
	}
	for _, root := range roots {
		if root["id"] == 1 {
			children := root["children"].([]gin.H)
			if len(children) != 1 || len(children[0]["children"].([]gin.H)) != 0 {
				t.Fatal("cycle not terminated")
			}
		}
	}
}

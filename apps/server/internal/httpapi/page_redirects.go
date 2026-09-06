package httpapi

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

var pageUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func projectPageURL(pid int32, path string, parameters url.Values) string {
	query := url.Values{}
	for key, values := range parameters {
		query[key] = append([]string(nil), values...)
	}
	query.Set("projectId", strconv.Itoa(int(pid)))
	return (&url.URL{Path: path, RawQuery: query.Encode()}).String()
}

func validPageID(value string) bool {
	if value == "" || value[0] == '0' {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	id, err := strconv.ParseInt(value, 10, 32)
	return err == nil && id > 0
}

func legacyPageURL(u *url.URL) (string, bool) {
	parts := strings.Split(strings.TrimSuffix(u.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "" || !validPageID(parts[2]) {
		return "", false
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", false
	}
	target := ""
	if parts[1] == "teams" && len(parts) == 3 {
		target = "/teams/detail/"
		query.Set("teamId", parts[2])
	} else if parts[1] == "projects" {
		query.Set("projectId", parts[2])
		switch len(parts) {
		case 3:
			target = "/projects/detail/"
		case 4:
			switch parts[3] {
			case "tasks", "settings", "realtime", "assets", "issues", "events", "devices", "algorithms", "connectors", "agents":
				target = "/projects/" + parts[3] + "/"
			}
		case 5:
			if parts[3] == "issues" && validPageID(parts[4]) {
				target = "/projects/issues/detail/"
				query.Set("issueId", parts[4])
			}
			if parts[3] == "events" && pageUUID.MatchString(parts[4]) {
				target = "/projects/events/detail/"
				query.Set("eventId", parts[4])
			}
		case 6:
			if parts[4] == "runs" {
				if parts[3] == "tasks" && validPageID(parts[5]) {
					target = "/projects/tasks/runs/detail/"
					query.Set("runId", parts[5])
				}
				if parts[3] == "algorithms" && pageUUID.MatchString(parts[5]) {
					target = "/projects/algorithms/runs/detail/"
					query.Set("runId", parts[5])
				}
			}
		}
	}
	if target == "" {
		return "", false
	}
	return (&url.URL{Path: target, RawQuery: query.Encode()}).String(), true
}

func (s *Server) pageNotFound(c *gin.Context) {
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		if target, ok := legacyPageURL(c.Request.URL); ok {
			c.Header("Cache-Control", "no-cache")
			c.Redirect(http.StatusTemporaryRedirect, target)
			return
		}
	}
	s.failure(c, 404, "NOT_FOUND")
}

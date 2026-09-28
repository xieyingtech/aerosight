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
	query.Del("projectId")
	segment := strings.Trim(strings.TrimPrefix(path, "/projects/"), "/")
	if segment == "detail" {
		segment = ""
	}
	for page, key := range map[string]string{"tasks/detail": "taskId", "tasks/runs/detail": "runId", "algorithms/runs/detail": "runId", "issues/detail": "issueId", "events/detail": "eventId", "reports/detail": "reportId", "inspection/summary": "runId", "inspection/observation": "observationId", "inspection/evidence": "evidenceSetId", "inspection/assessment": "assessmentId"} {
		if segment == page && query.Get(key) != "" {
			base := strings.TrimSuffix(page, "/detail")
			switch page {
			case "inspection/summary":
				base = "inspection/runs"
			case "inspection/observation":
				base = "inspection/observations"
			case "inspection/evidence":
				base = "inspection/evidence-sets"
			case "inspection/assessment":
				base = "inspection/assessments"
			}
			segment = base + "/" + query.Get(key)
			query.Del(key)
			break
		}
	}
	path = "/projects/" + strconv.Itoa(int(pid)) + "/"
	if segment != "" {
		path += segment + "/"
	}
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
			case "tasks", "settings", "realtime", "assets", "issues", "events", "devices", "algorithms", "connectors", "agents", "flight-operations", "geospatial", "models":
				target = "/projects/" + parts[3] + "/"
			}
		case 5:
			if parts[3] == "reports" && pageUUID.MatchString(parts[4]) {
				target = "/projects/reports/detail/"
				query.Set("reportId", parts[4])
			}
			if parts[3] == "tasks" && validPageID(parts[4]) {
				target = "/projects/tasks/detail/"
				query.Set("taskId", parts[4])
			}
			if parts[3] == "issues" && validPageID(parts[4]) {
				target = "/projects/issues/detail/"
				query.Set("issueId", parts[4])
			}
			if parts[3] == "events" && pageUUID.MatchString(parts[4]) {
				target = "/projects/events/detail/"
				query.Set("eventId", parts[4])
			}
		case 6:
			if parts[3] == "inspection" && parts[4] == "runs" && validPageID(parts[5]) {
				target = "/projects/inspection/summary/"
				query.Set("runId", parts[5])
			}
			if parts[3] == "realtime" && parts[4] == "devices" && validPageID(parts[5]) {
				target = "/projects/realtime/"
				query.Set("deviceId", parts[5])
			}
			if parts[3] == "inspection" && pageUUID.MatchString(parts[5]) {
				for plural, mapping := range map[string][2]string{"observations": {"observation", "observationId"}, "evidence-sets": {"evidence", "evidenceSetId"}, "assessments": {"assessment", "assessmentId"}} {
					if parts[4] == plural {
						target = "/projects/inspection/" + mapping[0] + "/"
						query.Set(mapping[1], parts[5])
					}
				}
			}
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
		if target, ok := legacyPageURL(c.Request.URL); ok && s.staticPages != nil {
			// Serve the exported page shell without exposing its internal query route.
			request := c.Request.Clone(c.Request.Context())
			request.URL, _ = url.Parse(target)
			c.Status(http.StatusOK)
			s.staticPages.ServeHTTP(c.Writer, request)
			c.Writer.WriteHeaderNow()
			return
		}
	}
	if s.staticPages != nil && c.Request.URL.Path != "/api" && !strings.HasPrefix(c.Request.URL.Path, "/api/") && c.Request.URL.Path != "/algorithm-assets" && !strings.HasPrefix(c.Request.URL.Path, "/algorithm-assets/") {
		s.staticPages.ServeHTTP(c.Writer, c.Request)
		c.Writer.WriteHeaderNow()
		return
	}
	s.failure(c, 404, "NOT_FOUND")
}

func (s *Server) AttachStaticPages(handler http.Handler) {
	if pages, ok := handler.(interface{ ConfigureCSP([]string, []string) }); ok {
		pages.ConfigureCSP(s.cfg.CSPMapOrigins, s.cfg.CSPMediaOrigins)
	}
	s.staticPages = handler
}

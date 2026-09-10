package observability

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"
)

type DependencyCheck struct {
	Name     string
	Critical bool
	Check    func(context.Context) error
}

type DependencyStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type HealthReport struct {
	Status       string             `json:"status"`
	Live         bool               `json:"live"`
	Ready        bool               `json:"ready"`
	Dependencies []DependencyStatus `json:"dependencies"`
}

type HealthHandler struct {
	checks  []DependencyCheck
	timeout time.Duration
}

func NewHealthHandler(checks []DependencyCheck, timeout time.Duration) *HealthHandler {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &HealthHandler{checks: checks, timeout: timeout}
}

func (handler *HealthHandler) Evaluate(ctx context.Context) HealthReport {
	report := HealthReport{Status: "healthy", Live: true, Ready: true}
	checks := append([]DependencyCheck(nil), handler.checks...)
	sort.Slice(checks, func(left, right int) bool { return checks[left].Name < checks[right].Name })
	for _, check := range checks {
		status := DependencyStatus{Name: check.Name, Status: "available"}
		probeContext, cancel := context.WithTimeout(ctx, handler.timeout)
		err := check.Check(probeContext)
		cancel()
		if err != nil {
			status.Status = "unavailable"
			status.Reason = dependencyReason(check.Name)
			report.Status = "degraded"
			if check.Critical {
				report.Ready = false
				report.Status = "unavailable"
			}
		}
		report.Dependencies = append(report.Dependencies, status)
	}
	return report
}

func (handler *HealthHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	report := handler.Evaluate(request.Context())
	status := http.StatusOK
	if request.URL.Path == "/readyz" && !report.Ready {
		status = http.StatusServiceUnavailable
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(report)
}

func dependencyReason(name string) string {
	switch name {
	case "database":
		return "DATABASE_UNAVAILABLE"
	case "object_storage":
		return "OBJECT_STORAGE_UNAVAILABLE"
	default:
		return "DEPENDENCY_UNAVAILABLE"
	}
}

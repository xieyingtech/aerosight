package flighthub

import (
	"context"
	"net/http"
	"testing"
)

func TestFlightAlertsExplicitNullEmptyPage(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		bad        bool
	}{
		{"confirmed empty", `{"data":null,"page":1,"page_size":50,"total":0,"page_count":0}`, false},
		{"missing collection", `{"page":1,"page_size":50,"total":0,"page_count":0}`, true},
		{"missing count", `{"data":null,"page":1,"page_size":50,"page_count":0}`, true},
		{"nonempty count", `{"data":null,"page":1,"page_size":50,"total":1,"page_count":1}`, true},
		{"wrong page", `{"data":null,"page":2,"page_size":50,"total":0,"page_count":0}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testClient(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" {
					t.Fatal("unexpected write")
				}
				return response(200, []byte(`{"code":0,"data":`+tc.body+`}`), nil), nil
			}), nil)
			page, err := c.ListFlightAlerts(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", FlightAlertOptions{DroneSN: "DRONE_REDACTED"})
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v wantBad=%t", err, tc.bad)
			}
			if !tc.bad && (page.Data == nil || page.Total != 0 || len(page.Data) != 0) {
				t.Fatal("empty result not normalized")
			}
		})
	}
}

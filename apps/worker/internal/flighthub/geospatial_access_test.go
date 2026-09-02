package flighthub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
)

func geospatialAccessInstance() connector.Instance {
	return connector.Instance{
		ID: 71, ProjectID: 31,
		DiscoveryScope: json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"synthetic project"}`),
	}
}

func geospatialAccessServiceForTest(client GeospatialFileClient, now time.Time, observations *[]GeospatialAccessObservation) *GeospatialAccessService {
	instance := geospatialAccessInstance()
	return &GeospatialAccessService{
		client: client, resolver: tokenResolverFixture{token: "TOKEN_REDACTED"}, now: func() time.Time { return now },
		load: func(_ context.Context, requested connector.Instance) (connector.Instance, error) {
			if requested.ID != instance.ID || requested.ProjectID != instance.ProjectID {
				return connector.Instance{}, connector.ErrRemoteResourceUnavailable
			}
			return instance, nil
		},
		observe: func(observation GeospatialAccessObservation) { *observations = append(*observations, observation) },
	}
}

func TestGeospatialAccessRefreshesExpiredLinksAndScopesEveryRequest(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	areaCalls, offlineCalls := 0, 0
	secret := "SIGNED_QUERY_MUST_NOT_REACH_OBSERVABILITY"
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/openapi/v2.0/flight-areas/url":
			areaCalls++
			expires := now.Add(-time.Minute).Unix()
			if areaCalls > 1 {
				expires = now.Add(10 * time.Minute).Unix()
			}
			body := fmt.Sprintf(`{"code":0,"message":"","data":{"name":"areas.json","url":"https://objects.vendor.example/areas.json?Expires=%d&Signature=%s","checksum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":12}}`, expires, secret)
			return response(http.StatusOK, []byte(body), nil), nil
		case "/openapi/v2.0/workspaces/" + runtimeProjectUUID + "/offline-maps/url":
			offlineCalls++
			body := fmt.Sprintf(`{"code":0,"message":"","data":{"offline_map_enable":true,"file":{"name":"offline.zip","url":"https://objects.vendor.example/offline.zip?Expires=%d&Signature=%s","checksum":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","size":24}}}`, now.Add(10*time.Minute).Unix(), secret)
			return response(http.StatusOK, []byte(body), nil), nil
		default:
			t.Fatalf("unexpected geospatial access request %s", request.URL.Path)
			return nil, nil
		}
	}), func(config *Config) {
		config.Now = func() time.Time { return now }
		config.AllowedLinkHosts = []string{"objects.vendor.example"}
	})
	observations := []GeospatialAccessObservation{}
	service := geospatialAccessServiceForTest(client, now, &observations)

	if _, err := service.RefreshDownload(context.Background(), connector.Instance{ID: 71, ProjectID: 99}, GeospatialFlightAreaFile); !errors.Is(err, connector.ErrRemoteResourceUnavailable) || areaCalls != 0 || offlineCalls != 0 {
		t.Fatalf("cross-project request reached upstream calls=%d/%d err=%v", areaCalls, offlineCalls, err)
	}
	area, err := service.RefreshDownload(context.Background(), geospatialAccessInstance(), GeospatialFlightAreaFile)
	if err != nil || areaCalls != 2 || !area.ExpiresAt.Equal(now.Add(10*time.Minute)) || !strings.Contains(area.URL, secret) {
		t.Fatalf("refreshed flight-area file=%#v calls=%d err=%v", area, areaCalls, err)
	}
	offline, err := service.RefreshDownload(context.Background(), geospatialAccessInstance(), GeospatialOfflineMap)
	if err != nil || offlineCalls != 1 || !offline.ExpiresAt.Equal(now.Add(10*time.Minute)) || !strings.Contains(offline.URL, secret) {
		t.Fatalf("refreshed offline map=%#v calls=%d err=%v", offline, offlineCalls, err)
	}
	serialized, _ := json.Marshal(observations)
	if strings.Contains(string(serialized), secret) || strings.Contains(string(serialized), "Signature") || len(observations) != 3 || observations[0].SafeCode != "stream_failed" || observations[1].SafeCode != "ok" || observations[2].SafeCode != "ok" {
		t.Fatalf("geospatial observability leaked signed parameters or unstable codes: %s", serialized)
	}
}

func TestGeospatialAccessFailsClosedForForbiddenHostsAndEmptyOfflineMaps(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC)
	mode := "forbidden"
	secretURL := fmt.Sprintf("https://attacker.example/map.zip?Expires=%d&Signature=DO_NOT_LOG", now.Add(time.Minute).Unix())
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if mode == "empty" {
			return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{"offline_map_enable":false,"file":{}}}`), nil), nil
		}
		body := fmt.Sprintf(`{"code":0,"message":"","data":{"name":"map.zip","url":%q,"checksum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":1}}`, secretURL)
		return response(http.StatusOK, []byte(body), nil), nil
	}), func(config *Config) {
		config.Now = func() time.Time { return now }
		config.AllowedLinkHosts = []string{"objects.vendor.example"}
	})
	observations := []GeospatialAccessObservation{}
	service := geospatialAccessServiceForTest(client, now, &observations)
	_, err := service.RefreshDownload(context.Background(), geospatialAccessInstance(), GeospatialFlightAreaFile)
	if !IsSafeCode(err, "temporary_link_host_forbidden") || strings.Contains(err.Error(), secretURL) {
		t.Fatalf("forbidden host was not safely rejected: %v", err)
	}
	mode = "empty"
	_, err = service.RefreshDownload(context.Background(), geospatialAccessInstance(), GeospatialOfflineMap)
	if !IsSafeCode(err, "resource_empty") {
		t.Fatalf("disabled offline map was not an empty resource: %v", err)
	}
	serialized, _ := json.Marshal(observations)
	if strings.Contains(string(serialized), "attacker.example") || strings.Contains(string(serialized), "DO_NOT_LOG") {
		t.Fatalf("geospatial error observation leaked URL: %s", serialized)
	}
}

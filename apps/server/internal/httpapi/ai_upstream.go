package httpapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Dial only the addresses checked by the outbound policy. The original URL
// hostname is retained for TLS verification and the HTTP Host header.
func pinnedAIHTTPClient(target *url.URL, addresses []netip.Addr) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		expectedPort := target.Port()
		if expectedPort == "" {
			expectedPort = "443"
		}
		if err != nil || !strings.EqualFold(host, target.Hostname()) || port != expectedPort {
			return nil, errors.New("AI_UPSTREAM_DESTINATION_CHANGED")
		}
		var last error
		for _, ip := range addresses {
			conn, e := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		if last == nil {
			last = errors.New("OUTBOUND_DNS_EMPTY")
		}
		return nil, last
	}
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("AI_UPSTREAM_REDIRECT_FORBIDDEN") }}
}

// The existing models probe uses status only. Discard the upstream body without
// decoding or buffering it; give the SDK an empty page for its typed decoder.
type aiStatusClient struct{ client *http.Client }

func (c aiStatusClient) Do(req *http.Request) (*http.Response, error) {
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	res.Body = io.NopCloser(strings.NewReader(`{"data":[]}`))
	res.ContentLength = -1
	return res, nil
}

func probeAIModels(ctx context.Context, client *http.Client, baseURL, apiKey string) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	sdk := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(strings.TrimRight(baseURL, "/")+"/"), option.WithHTTPClient(aiStatusClient{client}), option.WithMaxRetries(0))
	var response *http.Response
	_, _ = sdk.Models.List(ctx, option.WithResponseInto(&response))
	if response == nil {
		return false, "AI_PROVIDER_CONNECTION_FAILED"
	}
	// Non-2xx responses are SDK errors, but retain the original status contract.
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return true, "OK"
	}
	return false, "HTTP_" + strconv.Itoa(response.StatusCode)
}

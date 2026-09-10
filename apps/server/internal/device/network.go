package device

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var NetworkEndpointFields = []string{"mqttEndpoint", "apiPublicBaseUrl", "websocketPublicUrl", "mediaIngestBaseUrl", "mediaPlaybackBaseUrl"}
var endpointSchemes = map[string][]string{
	"mqttEndpoint": {"mqtt", "mqtts"}, "apiPublicBaseUrl": {"http", "https"}, "websocketPublicUrl": {"ws", "wss"}, "mediaIngestBaseUrl": {"rtmp", "rtmps"}, "mediaPlaybackBaseUrl": {"http", "https"},
}

type NetworkProfile struct {
	Mode                                           string
	Endpoints                                      map[string]string
	TLSRequired, MQTTAnonymous, CredentialProvided bool
}
type NetworkIssue struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}
type NetworkEndpoint struct {
	Field     string
	URL       *url.URL
	Addresses []netip.Addr
}
type NetworkValidation struct {
	Valid     bool
	Issues    []NetworkIssue
	Endpoints map[string]NetworkEndpoint
}
type HostResolver func(context.Context, string) ([]netip.Addr, error)
type EndpointProbe func(context.Context, NetworkEndpoint) error

func unroutableDeviceAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.Is4() {
		b := ip.As4()
		return b[0] == 0 || b[0] == 127 || (b[0] == 169 && b[1] == 254) || b[0] >= 224
	}
	return !ip.IsValid() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast()
}
func restrictedDeviceAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if unroutableDeviceAddress(ip) || ip.IsPrivate() {
		return true
	}
	if ip.Is4() {
		b := ip.As4()
		return (b[0] == 100 && b[1] >= 64 && b[1] <= 127) || (b[0] == 192 && b[1] == 0) || (b[0] == 198 && (b[1] == 18 || b[1] == 19))
	}
	return false
}
func parseNetworkURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return nil, errors.New("ENDPOINT_URL_INVALID")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if port := u.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 0 || n > 65535 {
			return nil, errors.New("ENDPOINT_URL_INVALID")
		}
	}
	// WHATWG URL omits default ports for its special HTTP/WebSocket schemes.
	if (u.Port() == "80" && (u.Scheme == "http" || u.Scheme == "ws")) || (u.Port() == "443" && (u.Scheme == "https" || u.Scheme == "wss")) {
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	return u, nil
}
func ValidateNetworkProfile(ctx context.Context, p NetworkProfile, resolve HostResolver) NetworkValidation {
	result := NetworkValidation{Issues: []NetworkIssue{}, Endpoints: map[string]NetworkEndpoint{}}
	add := func(field, code string) {
		for _, v := range result.Issues {
			if v.Field == field && v.Code == code {
				return
			}
		}
		result.Issues = append(result.Issues, NetworkIssue{field, code})
	}
	if p.Mode == "public" {
		if !p.TLSRequired {
			add("tlsRequired", "PUBLIC_TLS_REQUIRED")
		}
		if p.MQTTAnonymous {
			add("mqttAnonymous", "PUBLIC_MQTT_ANONYMOUS_FORBIDDEN")
		}
		if !p.CredentialProvided {
			add("credential", "PUBLIC_CREDENTIAL_REQUIRED")
		}
	}
	if resolve == nil {
		resolve = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	type resolution struct {
		ips []netip.Addr
		err error
	}
	cache := map[string]resolution{}
	for _, field := range NetworkEndpointFields {
		u, err := parseNetworkURL(p.Endpoints[field])
		if err != nil {
			add(field, "ENDPOINT_URL_INVALID")
			continue
		}
		schemes := endpointSchemes[field]
		allowed := u.Scheme == schemes[1] || (p.Mode == "lan" && u.Scheme == schemes[0])
		if !allowed {
			code := "ENDPOINT_SCHEME_UNSUPPORTED"
			if p.Mode == "public" {
				code = "PUBLIC_TLS_SCHEME_REQUIRED"
			}
			add(field, code)
		}
		if u.User != nil && u.User.String() != "" {
			add(field, "ENDPOINT_INLINE_CREDENTIALS_FORBIDDEN")
		}
		host := strings.ToLower(u.Hostname())
		local := strings.TrimSuffix(host, ".")
		if local == "localhost" || strings.HasSuffix(local, ".localhost") {
			add(field, "ENDPOINT_LOOPBACK_FORBIDDEN")
			continue
		}
		r, found := cache[host]
		if !found {
			if ip, e := netip.ParseAddr(host); e == nil {
				r.ips = []netip.Addr{ip}
			} else {
				r.ips, r.err = resolve(ctx, host)
			}
			cache[host] = r
		}
		if r.err != nil {
			add(field, "ENDPOINT_DNS_FAILED")
			continue
		}
		if len(r.ips) == 0 {
			add(field, "ENDPOINT_DNS_EMPTY")
			continue
		}
		for _, ip := range r.ips {
			if unroutableDeviceAddress(ip) {
				add(field, "ENDPOINT_UNROUTABLE_ADDRESS")
			}
			if p.Mode == "public" && restrictedDeviceAddress(ip) {
				add(field, "PUBLIC_ADDRESS_REQUIRED")
			}
		}
		result.Endpoints[field] = NetworkEndpoint{field, u, r.ips}
	}
	result.Valid = len(result.Issues) == 0
	return result
}

type NetworkDiagnostic struct {
	Field              string `json:"field"`
	Endpoint           string `json:"endpoint"`
	Status             string `json:"status"`
	Code               string `json:"code"`
	DeviceVerification string `json:"deviceVerification"`
}
type NetworkCheck struct {
	OK                 bool                `json:"ok"`
	Status             string              `json:"status"`
	ServerVerification string              `json:"serverVerification"`
	DeviceVerification string              `json:"deviceVerification"`
	CheckedAt          string              `json:"checkedAt"`
	Diagnostics        []NetworkDiagnostic `json:"diagnostics"`
	PolicyIssues       []NetworkIssue      `json:"policyIssues"`
}

func CheckNetworkConnection(ctx context.Context, p NetworkProfile, resolve HostResolver, probe EndpointProbe) NetworkCheck {
	validation := ValidateNetworkProfile(ctx, p, resolve)
	if probe == nil {
		probe = ProbeNetworkEndpoint
	}
	result := NetworkCheck{Status: "invalid", ServerVerification: "failed", DeviceVerification: "pending", PolicyIssues: validation.Issues, Diagnostics: make([]NetworkDiagnostic, len(NetworkEndpointFields))}
	var wg sync.WaitGroup
	for i, field := range NetworkEndpointFields {
		wg.Add(1)
		go func(i int, field string) {
			defer wg.Done()
			d := NetworkDiagnostic{Field: field, Endpoint: "[invalid endpoint]", Status: "not_checked", Code: "ENDPOINT_POLICY_REJECTED", DeviceVerification: "pending"}
			if field == "mediaPlaybackBaseUrl" {
				d.DeviceVerification = "not_applicable"
			}
			if u, e := parseNetworkURL(p.Endpoints[field]); e == nil {
				d.Endpoint = u.Scheme + "://" + u.Host
			}
			endpoint, ok := validation.Endpoints[field]
			for _, issue := range validation.Issues {
				if issue.Field == field {
					d.Code = issue.Code
					ok = false
					break
				}
			}
			if ok {
				if e := probe(ctx, endpoint); e == nil {
					d.Status = "server_verified"
					d.Code = "ENDPOINT_REACHABLE"
				} else {
					d.Status = "failed"
					d.Code = "ENDPOINT_PROBE_FAILED"
				}
			}
			result.Diagnostics[i] = d
		}(i, field)
	}
	wg.Wait()
	result.OK = validation.Valid
	for _, d := range result.Diagnostics {
		if d.Status != "server_verified" {
			result.OK = false
		}
	}
	if result.OK {
		result.Status = "valid"
		result.ServerVerification = "verified"
	}
	result.CheckedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	return result
}

// Dial the address validated above, without another DNS lookup or redirect.
// HTTP reachability accepts responses below 500; socket protocols verify TCP/TLS only.
func ProbeNetworkEndpoint(ctx context.Context, e NetworkEndpoint) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if e.URL == nil || len(e.Addresses) == 0 {
		return errors.New("ENDPOINT_PROBE_FAILED")
	}
	defaults := map[string]string{"http": "80", "https": "443", "ws": "80", "wss": "443", "mqtt": "1883", "mqtts": "8883", "rtmp": "1935", "rtmps": "1936"}
	port := e.URL.Port()
	if port == "" {
		port = defaults[e.URL.Scheme]
	}
	if port == "" {
		return errors.New("ENDPOINT_PROBE_FAILED")
	}
	target := net.JoinHostPort(e.Addresses[0].String(), port)
	dialer := net.Dialer{}
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", target)
	}
	tlsConfig := &tls.Config{ServerName: e.URL.Hostname(), MinVersion: tls.VersionTLS12}
	if e.URL.Scheme == "http" || e.URL.Scheme == "https" {
		transport := &http.Transport{DialContext: dial, TLSClientConfig: tlsConfig, DisableKeepAlives: true}
		defer transport.CloseIdleConnections()
		client := http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, err := http.NewRequestWithContext(ctx, "HEAD", e.URL.String(), nil)
		if err != nil {
			return err
		}
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode >= 500 {
			return errors.New("ENDPOINT_UNHEALTHY")
		}
		return nil
	}
	conn, err := dial(ctx, "", "")
	if err != nil {
		return err
	}
	defer conn.Close()
	if e.URL.Scheme == "mqtts" || e.URL.Scheme == "wss" || e.URL.Scheme == "rtmps" {
		return tls.Client(conn, tlsConfig).HandshakeContext(ctx)
	}
	return nil
}

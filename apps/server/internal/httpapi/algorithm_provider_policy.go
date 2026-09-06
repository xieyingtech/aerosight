package httpapi

import (
	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

type algorithmProviderInput struct {
	Params     sqlcgen.CreateAlgorithmProviderParams
	Credential map[string]string
	Audit      map[string]any
}

func parseAlgorithmProvider(raw map[string]any) (algorithmProviderInput, error) {
	out := algorithmProviderInput{Audit: map[string]any{}}
	bad := errors.New("ALGORITHM_PROVIDER_INPUT_INVALID")
	allowed := map[string]bool{"name": true, "providerType": true, "baseUrl": true, "authType": true, "credential": true, "username": true, "allowedHeaders": true, "timeoutSeconds": true, "concurrencyLimit": true, "rateLimitPerMinute": true}
	for k, v := range raw {
		if !allowed[k] {
			return out, bad
		}
		if k != "credential" {
			out.Audit[k] = v
		}
	}
	name, ok := raw["name"].(string)
	name = strings.TrimSpace(name)
	if !ok || utf16Length(name) < 1 || utf16Length(name) > 120 {
		return out, bad
	}
	out.Params.Name = name
	out.Audit["name"] = name
	kind, ok := raw["providerType"].(string)
	if !ok {
		return out, bad
	}
	if _, err := algorithm.CapabilityFor(kind); err != nil {
		return out, bad
	}
	out.Params.ProviderType = kind
	endpoint, ok := raw["baseUrl"].(string)
	if !ok {
		return out, bad
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" {
		return out, bad
	}
	out.Params.BaseUrl = endpoint
	auth, ok := raw["authType"].(string)
	if !ok {
		return out, bad
	}
	switch auth {
	case "none", "bearer", "api-key-header", "basic", "signed":
	default:
		return out, bad
	}
	out.Params.AuthType = auth
	username := ""
	if v, present := raw["username"]; present {
		username, ok = v.(string)
		username = strings.TrimSpace(username)
		if !ok || utf16Length(username) > 255 {
			return out, bad
		}
		out.Audit["username"] = username
	}
	credential := ""
	if v, present := raw["credential"]; present {
		credential, ok = v.(string)
		if !ok || utf16Length(credential) > 16384 {
			return out, bad
		}
		credential = strings.TrimSpace(credential)
	}
	if credential != "" {
		switch auth {
		case "bearer":
			out.Credential = map[string]string{"token": credential}
		case "api-key-header":
			out.Credential = map[string]string{"apiKey": credential}
		case "signed":
			out.Credential = map[string]string{"secret": credential}
		case "basic":
			if username == "" {
				return out, bad
			}
			out.Credential = map[string]string{"username": username, "password": credential}
		}
	}
	headers := []string{}
	if value, present := raw["allowedHeaders"]; present {
		values, yes := value.([]any)
		if !yes || len(values) > 20 {
			return out, bad
		}
		for _, v := range values {
			h, yes := v.(string)
			if !yes || !regexp.MustCompile(`^[A-Za-z0-9-]+$`).MatchString(h) {
				return out, bad
			}
			headers = append(headers, h)
		}
	}
	out.Params.AllowedHeadersJson, _ = json.Marshal(headers)
	out.Audit["allowedHeaders"] = headers
	for _, field := range []struct {
		key  string
		max  float64
		dest *int32
	}{{"timeoutSeconds", 3600, &out.Params.TimeoutSeconds}, {"concurrencyLimit", 1000, &out.Params.ConcurrencyLimit}, {"rateLimitPerMinute", 1000000, &out.Params.RateLimitPerMinute}} {
		n, yes := raw[field.key].(float64)
		if !yes || math.IsNaN(n) || n < 1 || n > field.max || math.Trunc(n) != n {
			return out, bad
		}
		*field.dest = int32(n)
	}
	return out, nil
}

func (s *Server) validateAlgorithmURL(ctx context.Context, raw string) (*url.URL, int, error) {
	return s.validateOutboundURL(ctx, raw, s.cfg.AlgorithmAllowedHosts)
}

func (s *Server) validateOutboundURL(ctx context.Context, raw string, allowedHosts []string) (*url.URL, int, error) {
	fail := func(code string) (*url.URL, int, error) { return nil, 0, errors.New(code) }
	target, err := url.Parse(raw)
	if err != nil || target.Hostname() == "" {
		return fail("OUTBOUND_URL_INVALID")
	}
	if target.Scheme != "https" {
		return fail("OUTBOUND_HTTPS_REQUIRED")
	}
	if target.User != nil {
		return fail("OUTBOUND_URL_CREDENTIALS_FORBIDDEN")
	}
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	allowed := false
	for _, pattern := range allowedHosts {
		pattern = strings.TrimSuffix(strings.ToLower(pattern), ".")
		if strings.HasPrefix(pattern, "*.") {
			allowed = allowed || (strings.HasSuffix(host, pattern[1:]) && host != pattern[2:])
		} else {
			allowed = allowed || host == pattern
		}
	}
	if !allowed {
		return fail("OUTBOUND_HOST_NOT_ALLOWED")
	}
	var addresses []netip.Addr
	if ip, e := netip.ParseAddr(host); e == nil {
		addresses = []netip.Addr{ip}
	} else if s.networkResolver != nil {
		addresses, err = s.networkResolver(ctx, host)
	} else {
		addresses, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}
	if err != nil {
		return fail("OUTBOUND_DNS_FAILED")
	}
	if len(addresses) == 0 {
		return fail("OUTBOUND_DNS_EMPTY")
	}
	for _, ip := range addresses {
		ip = ip.Unmap()
		restricted := !ip.IsValid() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast()
		if ip.Is4() {
			b := ip.As4()
			restricted = restricted || b[0] == 0 || b[0] >= 224 || (b[0] == 100 && b[1] >= 64 && b[1] <= 127) || (b[0] == 192 && b[1] == 0) || (b[0] == 198 && (b[1] == 18 || b[1] == 19))
		}
		if restricted {
			return fail("OUTBOUND_ADDRESS_RESTRICTED")
		}
	}
	return target, len(addresses), nil
}

package httpapi

import (
	"context"
	"net/netip"
	"testing"
)

func TestAdminAlgorithmEndpointsAllowPrivateNetwork(t *testing.T) {
	s := &Server{}
	s.networkResolver = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("10.1.2.3")}, nil
	}
	ctx := context.Background()
	for _, endpoint := range []string{"http://aerosight-algo-demo.zeabur.internal:8080/infer", "https://10.0.0.2/infer", "http://127.0.0.1:8091/infer"} {
		if _, n, err := s.validateAlgorithmURL(ctx, endpoint); err != nil || n != 1 {
			t.Fatalf("administrator endpoint %s: %d %v", endpoint, n, err)
		}
	}
	for _, endpoint := range []string{"file:///etc/passwd", "ftp://10.0.0.2", "http://user:pass@10.0.0.2", "http://10.0.0.2?token=secret"} {
		if _, _, err := s.validateAlgorithmURL(ctx, endpoint); err == nil {
			t.Fatalf("accepted malformed endpoint %s", endpoint)
		}
	}
	if _, _, err := s.validateOutboundURL(ctx, "https://10.0.0.2", []string{"10.0.0.2"}); err == nil {
		t.Fatal("generic outbound policy was relaxed")
	}
}

package httpapi

import (
	"context"
	"testing"
)

func TestDevelopmentAlgorithmEndpointIsExactAndScoped(t *testing.T) {
	s := &Server{}
	s.cfg.AlgorithmDevelopmentEndpoint = "https://127.0.0.1:8444/infer"
	ctx := context.Background()
	if _, _, err := s.validateAlgorithmURL(ctx, s.cfg.AlgorithmDevelopmentEndpoint); err == nil {
		t.Fatal("production accepted loopback")
	}
	s.cfg.Development = true
	if _, _, err := s.validateAlgorithmURL(ctx, s.cfg.AlgorithmDevelopmentEndpoint); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"http://127.0.0.1:8444/infer", "https://127.0.0.1:8445/infer", "https://127.0.0.1:8444/other", "https://127.0.0.1:8444/infer?x=1", "https://10.0.0.1:8444/infer"} {
		if _, _, err := s.validateAlgorithmURL(ctx, u); err == nil {
			t.Fatalf("accepted %s", u)
		}
	}
	if _, _, err := s.validateOutboundURL(ctx, s.cfg.AlgorithmDevelopmentEndpoint, []string{"127.0.0.1"}); err == nil {
		t.Fatal("generic outbound policy was relaxed")
	}
}

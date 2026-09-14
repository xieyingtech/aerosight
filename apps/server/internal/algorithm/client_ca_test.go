package algorithm

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPClientDeploymentCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "ca.pem")
	cert, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := HTTPClientWithCA(file)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 204 {
		t.Fatal(res.StatusCode)
	}
	if _, err = HTTPClientWithCA(file + "missing"); err == nil {
		t.Fatal("accepted missing CA")
	}
	if err = os.WriteFile(file, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = HTTPClientWithCA(file); err == nil {
		t.Fatal("accepted invalid CA")
	}
}

package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/jobdock/jobdock/internal/config"
)

func TestClientAddressTrustsForwardedMetadataOnlyWhenConfigured(t *testing.T) {
	request := httptest.NewRequest("GET", "http://jobdock.test/", nil)
	request.RemoteAddr = "172.18.0.4:43120"
	request.Header.Set("X-Forwarded-For", "203.0.113.17, 172.18.0.3")

	untrusted := &API{config: config.Server{TrustProxyHeaders: false}}
	if got := untrusted.clientAddress(request); got != "172.18.0.4" {
		t.Fatalf("untrusted proxy metadata changed client address to %q", got)
	}

	trusted := &API{config: config.Server{TrustProxyHeaders: true}}
	if got := trusted.clientAddress(request); got != "203.0.113.17" {
		t.Fatalf("trusted proxy metadata produced %q", got)
	}

	request.Header.Set("X-Forwarded-For", "not-an-address")
	request.Header.Set("X-Real-IP", "198.51.100.8")
	if got := trusted.clientAddress(request); got != "198.51.100.8" {
		t.Fatalf("trusted real IP produced %q", got)
	}
}

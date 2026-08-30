package deploy

import (
	"strings"
	"testing"
)

func TestReleaseExposureModesAreIsolated(t *testing.T) {
	base := readTestFile(t, "docker-compose.release.yml.tmpl")
	serverSection := strings.Split(strings.Split(base, "  jobdock-server:")[1], "  jobdock-builder:")[0]
	if strings.Contains(serverSection, "ports:") {
		t.Fatal("the base server must not publish a host port")
	}

	domain := readTestFile(t, "docker-compose.domain.yml")
	for _, required := range []string{"caddy:2.10.2-alpine", `"80:80"`, `"443:443"`, `"443:443/udp"`, `JOBDOCK_TRUST_PROXY_HEADERS: "true"`} {
		if !strings.Contains(domain, required) {
			t.Fatalf("domain mode is missing %q", required)
		}
	}

	proxy := readTestFile(t, "docker-compose.proxy.yml")
	if strings.Contains(proxy, "caddy:") || !strings.Contains(proxy, "127.0.0.1") || !strings.Contains(proxy, `JOBDOCK_TRUST_PROXY_HEADERS: "true"`) {
		t.Fatal("proxy mode must bind the trusted proxy endpoint to loopback without Caddy")
	}

	local := readTestFile(t, "docker-compose.local.yml")
	if strings.Contains(local, "caddy:") || !strings.Contains(local, "0.0.0.0") || !strings.Contains(local, `JOBDOCK_TRUST_PROXY_HEADERS: "false"`) {
		t.Fatal("local mode must expose the server directly without trusting proxy metadata")
	}
}

func TestReleaseCaddyPolicyIsStreamingAndDomainOnly(t *testing.T) {
	config := readTestFile(t, "Caddyfile.release")
	for _, required := range []string{"{$JOBDOCK_DOMAIN}", "Strict-Transport-Security", "flush_interval -1", "reverse_proxy jobdock-server:8080"} {
		if !strings.Contains(config, required) {
			t.Fatalf("Caddy policy is missing %q", required)
		}
	}
	if strings.Contains(readTestFile(t, "docker-compose.proxy.yml"), "Strict-Transport-Security") || strings.Contains(readTestFile(t, "docker-compose.local.yml"), "Strict-Transport-Security") {
		t.Fatal("HSTS must only be applied by automatic domain TLS")
	}
}

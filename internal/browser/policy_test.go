package browser

import (
	"context"
	"net"
	"testing"
)

type resolver map[string][]net.IPAddr

func (r resolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	return append([]net.IPAddr(nil), r[host]...), nil
}

func TestCanonicalOriginRestrictsPublicAndLocalTargets(t *testing.T) {
	for _, test := range []struct {
		value string
		want  string
		err   bool
	}{
		{value: "https://example.com", want: "https://example.com:443"},
		{value: "http://127.0.0.1:4173", want: "http://127.0.0.1:4173"},
		{value: "https://[::1]", want: "https://[::1]:443"},
		{value: "http://example.com", err: true},
		{value: "https://example.com:8443", err: true},
		{value: "http://10.0.0.2:8080", err: true},
		{value: "file:///tmp/page.html", err: true},
		{value: "https://example.com/path", err: true},
	} {
		origin, err := CanonicalOrigin(test.value)
		if test.err {
			if err == nil {
				t.Fatalf("CanonicalOrigin(%q) succeeded with %#v", test.value, origin)
			}
			continue
		}
		if err != nil || origin.URL != test.want {
			t.Fatalf("CanonicalOrigin(%q) = %#v, %v; want %q", test.value, origin, err, test.want)
		}
	}
}

func TestResolveOriginRejectsNonLoopbackLocalhostAndPrivatePublicResolution(t *testing.T) {
	lookup := resolver{
		"localhost":   {{IP: net.ParseIP("10.0.0.4")}},
		"example.com": {{IP: net.ParseIP("127.0.0.1")}},
	}
	if _, err := ResolveOrigin(context.Background(), "http://localhost:3000", lookup); err == nil {
		t.Fatal("localhost resolving to private address was accepted")
	}
	if _, err := ResolveOrigin(context.Background(), "https://example.com", lookup); err == nil {
		t.Fatal("public hostname resolving to loopback was accepted")
	}
}

func TestResolveOriginRejectsDocumentationAddress(t *testing.T) {
	lookup := resolver{"example.com": {{IP: net.ParseIP("203.0.113.10")}}}
	if _, err := ResolveOrigin(context.Background(), "https://example.com", lookup); err == nil {
		t.Fatal("documentation address was accepted as a public browser origin")
	}
}

func TestAllowsURLEnforcesExactOrigin(t *testing.T) {
	origins := []Origin{{URL: "https://example.com:443"}, {URL: "http://127.0.0.1:3000"}}
	for _, test := range []struct {
		value string
		want  bool
	}{
		{"https://example.com/path", true},
		{"http://example.com", false},
		{"https://example.com:444/path", false},
		{"http://127.0.0.1:3000/api", true},
		{"http://127.0.0.1:3001/api", false},
		{"file:///tmp/x", false},
	} {
		if got := AllowsURL(origins, test.value); got != test.want {
			t.Fatalf("AllowsURL(%q) = %t, want %t", test.value, got, test.want)
		}
	}
}

func TestValidateCDPEndpointRejectsRemoteOrAmbiguousTarget(t *testing.T) {
	for _, test := range []struct {
		value string
		ok    bool
	}{
		{"http://127.0.0.1:9222", true},
		{"http://[::1]:9222", true},
		{"http://localhost:9222", false},
		{"https://127.0.0.1:9222", false},
		{"http://127.0.0.1", false},
		{"http://192.168.1.2:9222", false},
		{"http://127.0.0.1:9222/devtools", false},
	} {
		err := ValidateCDPEndpoint(test.value)
		if (err == nil) != test.ok {
			t.Fatalf("ValidateCDPEndpoint(%q) error = %v, want ok=%t", test.value, err, test.ok)
		}
	}
}

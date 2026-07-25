package egress

import (
	"context"
	"net"
	"testing"
)

func TestProxyRejectsPrivateAndIPTargets(t *testing.T) {
	proxy := New("secret")
	proxy.lookup = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("10.0.0.1")}, nil }
	if _, err := proxy.dialPublic(context.Background(), "example.test", "443"); err == nil {
		t.Fatal("private address accepted")
	}
	if _, err := proxy.dialPublic(context.Background(), "127.0.0.1", "443"); err == nil {
		t.Fatal("IP target accepted")
	}
}

func TestProxyDialsResolvedPublicAddress(t *testing.T) {
	proxy := New("secret")
	proxy.lookup = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("1.1.1.1")}, nil }
	called := ""
	proxy.dial = func(_, address string) (net.Conn, error) {
		called = address
		one, two := net.Pipe()
		_ = two.Close()
		return one, nil
	}
	connection, err := proxy.dialPublic(context.Background(), "example.test", "443")
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	if called != "1.1.1.1:443" {
		t.Fatalf("dial=%q", called)
	}
}

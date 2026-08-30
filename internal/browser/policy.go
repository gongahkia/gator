package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// CanonicalOrigin validates and normalizes a developer-approved browser
// origin. Public sites are HTTPS on 443. Loopback development sites may use
// HTTP or HTTPS on any explicit port. No other local address is accepted.
func CanonicalOrigin(value string) (Origin, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil {
		return Origin{}, errors.New("browser origin must be a valid absolute URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" && parsed.Path != "/" {
		return Origin{}, errors.New("browser origin must not include credentials, a path, query, or fragment")
	}
	if parsed.Hostname() == "" {
		return Origin{}, errors.New("browser origin host is required")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	port := parsed.Port()
	loopback := isLiteralLoopback(host) || host == "localhost"
	if loopback {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return Origin{}, errors.New("loopback browser origin must use http or https")
		}
		if port == "" {
			if parsed.Scheme == "http" {
				port = "80"
			} else {
				port = "443"
			}
		}
	} else {
		if parsed.Scheme != "https" || (port != "" && port != "443") {
			return Origin{}, errors.New("public browser origin must use https on port 443")
		}
		port = "443"
	}
	if host == "localhost" {
		// localhost is allowed only after its runtime resolution is checked by
		// ResolveOrigin. Keeping this spelling makes the user-facing policy
		// understandable without treating a mutable hosts entry as safe.
	} else if address, err := netip.ParseAddr(host); err == nil && !address.IsLoopback() {
		return Origin{}, errors.New("browser origin IP address must be loopback")
	}
	return Origin{URL: parsed.Scheme + "://" + net.JoinHostPort(host, port)}, nil
}

// ResolveOrigin verifies that a localhost approval cannot resolve beyond
// loopback before the browser controller accepts it. Public hosts must resolve
// only public addresses when they are first approved; the browser sidecar then
// enforces the canonical origin for every request.
func ResolveOrigin(ctx context.Context, value string, resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}) (Origin, error) {
	origin, err := CanonicalOrigin(value)
	if err != nil {
		return Origin{}, err
	}
	parsed, _ := url.Parse(origin.URL)
	host := parsed.Hostname()
	if isLiteralLoopback(host) {
		return origin, nil
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return Origin{}, fmt.Errorf("resolve browser origin %q: %w", host, err)
	}
	for _, address := range addresses {
		parsedAddress, ok := netip.AddrFromSlice(address.IP)
		if !ok {
			return Origin{}, errors.New("browser origin resolved to an invalid address")
		}
		if host == "localhost" {
			if !parsedAddress.IsLoopback() {
				return Origin{}, errors.New("localhost browser origin resolved outside loopback")
			}
			continue
		}
		if !isPublicAddress(parsedAddress.Unmap()) {
			return Origin{}, errors.New("public browser origin resolved to a non-public address")
		}
	}
	return origin, nil
}

// AllowsURL reports whether a browser request is permitted by the exact
// developer-approved origin set. It is used for every browser network class,
// not just top-level navigation.
func AllowsURL(origins []Origin, value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.User != nil || parsed.Hostname() == "" {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	candidate := parsed.Scheme + "://" + net.JoinHostPort(strings.ToLower(strings.TrimSuffix(parsed.Hostname(), ".")), port)
	for _, origin := range origins {
		if origin.URL == candidate {
			return true
		}
	}
	return false
}

func isLiteralLoopback(host string) bool {
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
}

func isPublicAddress(address netip.Addr) bool {
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() {
		return false
	}
	for _, prefix := range nonPublicBrowserPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

var nonPublicBrowserPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

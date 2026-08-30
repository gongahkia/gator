package browser

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
)

// ValidateCDPEndpoint keeps attached Chromium control strictly local. A CDP
// endpoint is powerful enough to inspect an entire existing browser profile,
// so Gator refuses hostnames, credentials, paths, and non-loopback addresses.
func ValidateCDPEndpoint(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil {
		return errors.New("browser CDP endpoint must be a valid URL")
	}
	if parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errors.New("browser CDP endpoint must be an unauthenticated loopback http URL")
	}
	if parsed.Port() == "" {
		return errors.New("browser CDP endpoint must include an explicit port")
	}
	address, err := netip.ParseAddr(parsed.Hostname())
	if err != nil || !address.IsLoopback() {
		return errors.New("browser CDP endpoint must use a literal loopback IP address")
	}
	return nil
}

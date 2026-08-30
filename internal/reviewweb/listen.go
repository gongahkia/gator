package reviewweb

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// ValidateListenAddress accepts only a literal loopback IP and TCP port.
func ValidateListenAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(port) == "" {
		return errors.New("listen address must be an IP address and TCP port, such as 127.0.0.1:0")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("listen address must be loopback; use SSH port forwarding for remote access")
	}
	return nil
}

// OpenInBrowser opens a loopback http review URL with the platform opener.
// It refuses non-http schemes and non-loopback hosts so a crafted URL cannot
// launch an arbitrary browser target.
func OpenInBrowser(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil {
		return errors.New("browser open is limited to a local http review URL")
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("browser open is limited to loopback http URLs")
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", rawURL).Start()
	case "linux":
		return exec.Command("xdg-open", rawURL).Start()
	default:
		return fmt.Errorf("browser review is supported only on Linux and macOS, not %s", runtime.GOOS)
	}
}

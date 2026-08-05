package egress

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Proxy struct {
	secret []byte
	dial   func(network, address string) (net.Conn, error)
	lookup func(context.Context, string) ([]net.IP, error)
}

func New(secret string) *Proxy {
	return &Proxy{secret: []byte(secret), dial: func(network, address string) (net.Conn, error) {
		return net.DialTimeout(network, address, 10*time.Second)
	}, lookup: func(ctx context.Context, host string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	}}
}
func (p *Proxy) Handler() http.Handler { return http.HandlerFunc(p.serveHTTP) }
func (p *Proxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT is required", http.StatusMethodNotAllowed)
		return
	}
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || port != "443" || !p.authorized(r, host) {
		http.Error(w, "egress denied", http.StatusForbidden)
		return
	}
	target, err := p.dialPublic(r.Context(), host, port)
	if err != nil {
		http.Error(w, "egress target unavailable", http.StatusBadGateway)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		target.Close()
		http.Error(w, "hijacking unavailable", 500)
		return
	}
	client, buffer, err := hijacker.Hijack()
	if err != nil {
		target.Close()
		return
	}
	_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	_ = buffer.Flush()
	go func() { _, _ = io.Copy(target, client); _ = target.Close(); _ = client.Close() }()
	go func() { _, _ = io.Copy(client, target); _ = target.Close(); _ = client.Close() }()
}

func (p *Proxy) dialPublic(ctx context.Context, host, port string) (net.Conn, error) {
	if net.ParseIP(host) != nil {
		return nil, fmt.Errorf("IP targets are not permitted")
	}
	addresses, err := p.lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, address := range addresses {
		if !publicIP(address) {
			continue
		}
		connection, err := p.dial("tcp", net.JoinHostPort(address.String(), port))
		if err == nil {
			return connection, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("target resolved only to non-public addresses")
}

func publicIP(address net.IP) bool {
	return address != nil && !address.IsLoopback() && !address.IsPrivate() && !address.IsLinkLocalUnicast() && !address.IsLinkLocalMulticast() && !address.IsMulticast() && !address.IsUnspecified()
}
func (p *Proxy) authorized(r *http.Request, target string) bool {
	hosts := strings.TrimSpace(r.Header.Get("X-Norbot-Egress-Hosts"))
	signature := strings.TrimSpace(r.Header.Get("X-Norbot-Egress-Signature"))
	if hosts == "" || signature == "" {
		return false
	}
	decoded, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, p.secret)
	_, _ = mac.Write([]byte(hosts))
	if !hmac.Equal(decoded, mac.Sum(nil)) {
		return false
	}
	for _, host := range strings.Split(hosts, ",") {
		if strings.EqualFold(strings.TrimSpace(host), target) {
			return true
		}
	}
	return false
}
func Serve(addr, secret string) (*http.Server, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("egress proxy secret is required")
	}
	server := &http.Server{Addr: addr, Handler: New(secret).Handler(), ReadHeaderTimeout: 5 * time.Second}
	return server, nil
}

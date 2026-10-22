package browser

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/privatefs"
)

// Windows uses an ephemeral IPv4 loopback endpoint. Its address lives in a
// private file; the existing random 256-bit token authenticates every RPC.
// No listener binds to LAN interfaces and no socket path limit is involved.
func listenController(path string) (net.Listener, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(listener.Addr().String()), 0600); err != nil {
		listener.Close()
		return nil, err
	}
	if err := privatefs.Check(path); err != nil {
		listener.Close()
		os.Remove(path)
		return nil, err
	}
	return listener, nil
}

func dialController(ctx context.Context, path string) (net.Conn, error) {
	if err := privatefs.Check(path); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64 {
		return nil, errors.New("invalid browser endpoint file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	address := strings.TrimSpace(string(data))
	host, port, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		return nil, errors.New("browser endpoint must be IPv4 loopback")
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return nil, errors.New("invalid browser endpoint port")
	}
	return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp4", address)
}

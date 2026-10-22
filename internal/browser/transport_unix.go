//go:build darwin || linux

package browser

import (
	"context"
	"net"
	"os"
	"time"
)

func listenController(path string) (net.Listener, error) {
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func dialController(ctx context.Context, path string) (net.Conn, error) {
	return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", path)
}

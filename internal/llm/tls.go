package llm

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
)

type TLSConfig struct {
	CAFile             string
	InsecureSkipVerify bool
}

var tlsWarningWriter io.Writer = os.Stderr

func newTLSConfig(cfg TLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify}
	if cfg.CAFile == "" {
		return tlsConfig, nil
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system cert pool: %w", err)
	}
	if roots == nil {
		roots = x509.NewCertPool()
	}
	pemBundle, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read TLS CA file: %w", err)
	}
	if !roots.AppendCertsFromPEM(pemBundle) {
		return nil, fmt.Errorf("TLS CA file %q contains no PEM certificates", cfg.CAFile)
	}
	tlsConfig.RootCAs = roots
	return tlsConfig, nil
}

type warningRoundTripper struct {
	inner http.RoundTripper
}

func (rt warningRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if tlsWarningWriter != nil {
		_, _ = fmt.Fprintln(tlsWarningWriter, "WARN: TLS verification disabled")
	}
	return rt.inner.RoundTrip(req)
}

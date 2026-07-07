package llm

import (
	"net/http"
	"time"
)

type HTTPClientConfig struct {
	Timeout time.Duration
	TLS     TLSConfig
}

func newHTTPClient(cfg HTTPClientConfig) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.TLS.CAFile != "" || cfg.TLS.InsecureSkipVerify {
		tlsConfig, err := newTLSConfig(cfg.TLS)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tlsConfig
	}
	var roundTripper http.RoundTripper = transport
	if cfg.TLS.InsecureSkipVerify {
		roundTripper = warningRoundTripper{inner: roundTripper}
	}
	return &http.Client{Timeout: cfg.Timeout, Transport: roundTripper}, nil
}

func defaultHTTPClient(timeout time.Duration) *http.Client {
	client, err := newHTTPClient(HTTPClientConfig{Timeout: timeout})
	if err != nil {
		return &http.Client{Timeout: timeout}
	}
	return client
}

func timeoutOrDefault(timeouts []time.Duration, fallback time.Duration) time.Duration {
	if len(timeouts) > 0 && timeouts[0] > 0 {
		return timeouts[0]
	}
	return fallback
}

package retrieval

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestCloudEmbeddingAdaptersUseSelectedProvider(t *testing.T) {
	for _, test := range []struct{ provider, host, header, response string }{
		{"openai", "api.openai.com", "Authorization", `{"data":[{"index":1,"embedding":[0.2]},{"index":0,"embedding":[0.1]}]}`},
		{"gemini", "generativelanguage.googleapis.com", "x-goog-api-key", `{"embedding":{"values":[0.1]}}`},
	} {
		t.Run(test.provider, func(t *testing.T) {
			embedder, err := NewEmbedder(test.provider, "secret-key")
			if err != nil {
				t.Fatal(err)
			}
			cloud := embedder.(cloudEmbedder)
			calls := 0
			cloud.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.URL.Host != test.host || request.Header.Get(test.header) == "" {
					t.Fatalf("unexpected request: %s %v", request.URL, request.Header)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(test.response)), Header: make(http.Header)}, nil
			})}
			input := []string{"first", "second"}
			if test.provider == "gemini" {
				input = input[:1]
			}
			vectors, err := cloud.Embed(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if len(vectors) != len(input) || vectors[0][0] != 0.1 || calls != len(input) && test.provider == "gemini" {
				t.Fatalf("vectors=%v calls=%d", vectors, calls)
			}
		})
	}
}

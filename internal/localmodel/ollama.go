package localmodel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	maxResponseBytes = 2 * 1024 * 1024
	maxProgressBytes = 64 * 1024
)

// Client invokes the Ollama management API over a literal loopback address.
// It intentionally cannot be configured to contact a remote inference server;
// callers needing that use the existing explicit custom-provider command.
type Client struct {
	baseURL *url.URL
	client  *http.Client
}

// InstalledModel is the bounded inventory returned by an Ollama runtime.
type InstalledModel struct {
	Name string
	Size int64
}

// Progress describes one pull status update. Its content is supplied by the
// local Ollama process and is display-only.
type Progress struct {
	Status    string
	Digest    string
	Completed int64
	Total     int64
}

// NewClient constructs a loopback-only client. An empty URL selects Ollama's
// standard local address.
func NewClient(rawURL string) (Client, error) {
	if strings.TrimSpace(rawURL) == "" {
		rawURL = DefaultBaseURL
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return Client{}, fmt.Errorf("parse local runtime URL: %w", err)
	}
	if parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Host == "" {
		return Client{}, errors.New("local runtime URL must be an absolute loopback http URL without credentials, query, or fragment")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host != "localhost" {
		address := net.ParseIP(host)
		if address == nil || !address.IsLoopback() {
			return Client{}, errors.New("local runtime URL must use localhost or a loopback IP address")
		}
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return Client{}, errors.New("local runtime URL has an invalid port")
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return Client{baseURL: parsed, client: http.DefaultClient}, nil
}

// NewClientFromChatCompletionsURL validates the endpoint persisted for the
// managed provider and recovers its loopback Ollama runtime root.
func NewClientFromChatCompletionsURL(rawURL string) (Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return Client{}, fmt.Errorf("parse local Chat Completions URL: %w", err)
	}
	const suffix = "/v1/chat/completions"
	if !strings.HasSuffix(parsed.Path, suffix) {
		return Client{}, errors.New("local Chat Completions URL must end in /v1/chat/completions")
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, suffix)
	return NewClient(parsed.String())
}

// BaseURL returns the runtime API root used by this client.
func (c Client) BaseURL() string {
	if c.baseURL == nil {
		return ""
	}
	return c.baseURL.String()
}

// ChatCompletionsURL returns the corresponding OpenAI-compatible endpoint.
func (c Client) ChatCompletionsURL() string {
	return c.endpoint("/v1/chat/completions")
}

// Version verifies that the selected local runtime is reachable.
func (c Client) Version(ctx context.Context) (string, error) {
	request, err := c.request(ctx, http.MethodGet, "/api/version", nil)
	if err != nil {
		return "", err
	}
	response, err := c.httpClient().Do(request)
	if err != nil {
		return "", fmt.Errorf("connect to local Ollama runtime: %w", err)
	}
	defer response.Body.Close()
	if err := requireSuccess(response, "read local Ollama version"); err != nil {
		return "", err
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := decodeBounded(response.Body, &payload); err != nil {
		return "", fmt.Errorf("decode local Ollama version: %w", err)
	}
	if strings.TrimSpace(payload.Version) == "" {
		return "", errors.New("local Ollama runtime returned an empty version")
	}
	return payload.Version, nil
}

// Models lists models installed in the selected local runtime.
func (c Client) Models(ctx context.Context) ([]InstalledModel, error) {
	request, err := c.request(ctx, http.MethodGet, "/api/tags", nil)
	if err != nil {
		return nil, err
	}
	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("connect to local Ollama runtime: %w", err)
	}
	defer response.Body.Close()
	if err := requireSuccess(response, "list local Ollama models"); err != nil {
		return nil, err
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"models"`
	}
	if err := decodeBounded(response.Body, &payload); err != nil {
		return nil, fmt.Errorf("decode local Ollama models: %w", err)
	}
	models := make([]InstalledModel, 0, len(payload.Models))
	seen := make(map[string]struct{}, len(payload.Models))
	for _, item := range payload.Models {
		name := strings.TrimSpace(item.Name)
		if name == "" || len(name) > 512 || strings.ContainsAny(name, "\r\n") {
			continue
		}
		if _, found := seen[name]; found {
			continue
		}
		seen[name] = struct{}{}
		models = append(models, InstalledModel{Name: name, Size: item.Size})
	}
	return models, nil
}

// Pull downloads one curated model and exposes bounded local progress. Ollama
// resumes interrupted pulls itself; Gator never sends an insecure-pull flag.
func (c Client) Pull(ctx context.Context, model Model, progress func(Progress)) error {
	payload, err := json.Marshal(struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}{Model: model.OllamaModel, Stream: true})
	if err != nil {
		return err
	}
	request, err := c.request(ctx, http.MethodPost, "/api/pull", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("connect to local Ollama runtime: %w", err)
	}
	defer response.Body.Close()
	if err := requireSuccess(response, "download local model"); err != nil {
		return err
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 1024), maxProgressBytes)
	sawSuccess := false
	for scanner.Scan() {
		var update struct {
			Status    string `json:"status"`
			Digest    string `json:"digest"`
			Completed int64  `json:"completed"`
			Total     int64  `json:"total"`
			Error     string `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &update); err != nil {
			return fmt.Errorf("decode local model download progress: %w", err)
		}
		if strings.TrimSpace(update.Error) != "" {
			return fmt.Errorf("download local model: %s", strings.TrimSpace(update.Error))
		}
		if strings.EqualFold(strings.TrimSpace(update.Status), "success") {
			sawSuccess = true
		}
		if progress != nil {
			progress(Progress{Status: strings.TrimSpace(update.Status), Digest: strings.TrimSpace(update.Digest), Completed: update.Completed, Total: update.Total})
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read local model download progress: %w", err)
	}
	if !sawSuccess {
		return errors.New("local model download ended without a success status")
	}
	return nil
}

// Remove deletes one curated model from the selected local runtime.
func (c Client) Remove(ctx context.Context, model Model) error {
	payload, err := json.Marshal(struct {
		Model string `json:"model"`
	}{Model: model.OllamaModel})
	if err != nil {
		return err
	}
	request, err := c.request(ctx, http.MethodDelete, "/api/delete", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("connect to local Ollama runtime: %w", err)
	}
	defer response.Body.Close()
	return requireSuccess(response, "remove local model")
}

func (c Client) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	if c.baseURL == nil {
		return nil, errors.New("local runtime URL is not configured")
	}
	return http.NewRequestWithContext(ctx, method, c.endpoint(path), body)
}

func (c Client) endpoint(path string) string {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	return endpoint.String()
}

func (c Client) httpClient() *http.Client {
	if c.client != nil {
		return c.client
	}
	return http.DefaultClient
}

func decodeBounded(source io.Reader, target any) error {
	return json.NewDecoder(io.LimitReader(source, maxResponseBytes)).Decode(target)
}

func requireSuccess(response *http.Response, action string) error {
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	contents, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	message := strings.TrimSpace(string(contents))
	if message == "" {
		return fmt.Errorf("%s: local Ollama runtime returned HTTP %d", action, response.StatusCode)
	}
	return fmt.Errorf("%s: local Ollama runtime returned HTTP %d: %s", action, response.StatusCode, message)
}

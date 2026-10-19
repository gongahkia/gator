// Package retrieval owns private, replaceable semantic indexes for saved
// knowledge. It never decides tool authority or turns retrieved text into
// instructions.
package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/localmodel"
)

const LocalModel = "embeddinggemma:latest"
const OpenAIModel = "text-embedding-3-small"
const GeminiModel = "gemini-embedding-001"

type Embedder interface {
	ID() string
	Embed(context.Context, []string) ([][]float32, error)
}

type ollamaEmbedder struct{ client localmodel.Client }

func (e ollamaEmbedder) ID() string { return "ollama/" + LocalModel }
func (e ollamaEmbedder) Embed(ctx context.Context, input []string) ([][]float32, error) {
	return e.client.Embed(ctx, LocalModel, input)
}

type cloudEmbedder struct {
	provider, model, key string
	client               *http.Client
}

func (e cloudEmbedder) ID() string { return e.provider + "/" + e.model }

// NewEmbedder never derives cloud permission from the selected chat model.
// The caller must check the independent, user-owned upload setting first.
func NewEmbedder(provider, key string) (Embedder, error) {
	switch provider {
	case "", "ollama":
		client, err := localmodel.NewClient("")
		if err != nil {
			return nil, err
		}
		return ollamaEmbedder{client}, nil
	case "openai":
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("OpenAI embeddings require an API key")
		}
		return cloudEmbedder{provider, OpenAIModel, key, &http.Client{Timeout: 20 * time.Second}}, nil
	case "gemini":
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("Gemini embeddings require an API key")
		}
		return cloudEmbedder{provider, GeminiModel, key, &http.Client{Timeout: 20 * time.Second}}, nil
	default:
		return nil, errors.New("unknown embedding provider")
	}
}

func (e cloudEmbedder) Embed(ctx context.Context, input []string) ([][]float32, error) {
	if len(input) == 0 || len(input) > 16 {
		return nil, errors.New("invalid cloud embedding batch")
	}
	if e.provider == "openai" {
		return e.openAI(ctx, input)
	}
	result := make([][]float32, 0, len(input))
	for _, value := range input {
		vector, err := e.gemini(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, vector)
	}
	return result, nil
}

func (e cloudEmbedder) openAI(ctx context.Context, input []string) ([][]float32, error) {
	payload, _ := json.Marshal(map[string]any{"model": e.model, "input": input, "encoding_format": "float"})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+e.key)
	request.Header.Set("Content-Type", "application/json")
	var response struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := e.call(request, &response); err != nil {
		return nil, err
	}
	if len(response.Data) != len(input) {
		return nil, errors.New("OpenAI embedding count mismatch")
	}
	vectors := make([][]float32, len(input))
	for _, item := range response.Data {
		if item.Index < 0 || item.Index >= len(vectors) || vectors[item.Index] != nil {
			return nil, errors.New("OpenAI embedding index mismatch")
		}
		vectors[item.Index] = item.Embedding
	}
	return vectors, nil
}

func (e cloudEmbedder) gemini(ctx context.Context, input string) ([]float32, error) {
	payload, _ := json.Marshal(map[string]any{"content": map[string]any{"parts": []map[string]string{{"text": input}}}, "taskType": "SEMANTIC_SIMILARITY"})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://generativelanguage.googleapis.com/v1beta/models/"+e.model+":embedContent", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("x-goog-api-key", e.key)
	request.Header.Set("Content-Type", "application/json")
	var response struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}
	if err := e.call(request, &response); err != nil {
		return nil, err
	}
	return response.Embedding.Values, nil
}

func (e cloudEmbedder) call(request *http.Request, target any) error {
	response, err := e.client.Do(request)
	if err != nil {
		return fmt.Errorf("%s embedding request failed: %w", e.provider, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s embedding request returned HTTP %d", e.provider, response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s embeddings: %w", e.provider, err)
	}
	return nil
}

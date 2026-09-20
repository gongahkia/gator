// Package localmodel defines Gator's reviewed local Work and coding-model
// catalog and its loopback-only Ollama runtime client.
package localmodel

import "strings"

const (
	// ProviderID is the Gator-owned custom provider selected through the TUI.
	// Keeping it separate from a user-defined provider avoids overwriting a
	// manually configured local endpoint.
	ProviderID = "gator-local"

	// DefaultBaseURL is Ollama's documented local API address.
	DefaultBaseURL = "http://127.0.0.1:11434"
)

// Model is one reviewed, tool-capable local model available from Ollama's
// model library. Download is the published package size, not a promise about
// runtime memory usage, which varies with hardware and context length.
type Model struct {
	Category      string
	ID            string
	OllamaModel   string
	Name          string
	Download      string
	DownloadBytes uint64
	Context       string
	Summary       string
	SourceURL     string
}

var catalog = []Model{
	{
		Category:      "General Work",
		ID:            "qwen3-4b",
		OllamaModel:   "qwen3:4b",
		Name:          "Qwen3 4B",
		Download:      "2.5 GB",
		DownloadBytes: 2_500_000_000,
		Context:       "256K",
		Summary:       "compact general Work model for source synthesis, analysis, and writing",
		SourceURL:     "https://ollama.com/library/qwen3",
	},
	{
		Category:      "General Work",
		ID:            "qwen3-8b",
		OllamaModel:   "qwen3:8b",
		Name:          "Qwen3 8B",
		Download:      "5.2 GB",
		DownloadBytes: 5_200_000_000,
		Context:       "40K",
		Summary:       "balanced general Work model for research, analysis, and writing",
		SourceURL:     "https://ollama.com/library/qwen3",
	},
	{
		Category:      "General Work",
		ID:            "llama3.1-8b",
		OllamaModel:   "llama3.1:8b",
		Name:          "Llama 3.1 8B",
		Download:      "4.9 GB",
		DownloadBytes: 4_900_000_000,
		Context:       "128K",
		Summary:       "long-context source synthesis, multilingual writing, and tool use",
		SourceURL:     "https://ollama.com/library/llama3.1",
	},
	{
		Category:      "General Work",
		ID:            "qwen3-30b",
		OllamaModel:   "qwen3:30b",
		Name:          "Qwen3 30B",
		Download:      "19 GB",
		DownloadBytes: 19_000_000_000,
		Context:       "256K",
		Summary:       "long-context general Work model for research and source synthesis",
		SourceURL:     "https://ollama.com/library/qwen3",
	},
	{
		Category:      "General Work",
		ID:            "mistral-small3.2-24b",
		OllamaModel:   "mistral-small3.2:24b",
		Name:          "Mistral Small 3.2 24B",
		Download:      "15 GB",
		DownloadBytes: 15_000_000_000,
		Context:       "128K",
		Summary:       "instruction-following general Work model with robust tool calls",
		SourceURL:     "https://ollama.com/library/mistral-small3.2",
	},
	{
		Category:      "Coding",
		ID:            "qwen2.5-coder-0.5b",
		OllamaModel:   "qwen2.5-coder:0.5b",
		Name:          "Qwen2.5-Coder 0.5B",
		Download:      "398 MB",
		DownloadBytes: 398_000_000,
		Context:       "32K",
		Summary:       "minimal coding option for constrained hardware",
		SourceURL:     "https://ollama.com/library/qwen2.5-coder",
	},
	{
		Category:      "Coding",
		ID:            "qwen2.5-coder-1.5b",
		OllamaModel:   "qwen2.5-coder:1.5b",
		Name:          "Qwen2.5-Coder 1.5B",
		Download:      "986 MB",
		DownloadBytes: 986_000_000,
		Context:       "32K",
		Summary:       "compact code-focused option for low-memory hosts",
		SourceURL:     "https://ollama.com/library/qwen2.5-coder",
	},
	{
		Category:      "Coding",
		ID:            "qwen2.5-coder-3b",
		OllamaModel:   "qwen2.5-coder:3b",
		Name:          "Qwen2.5-Coder 3B",
		Download:      "1.9 GB",
		DownloadBytes: 1_900_000_000,
		Context:       "32K",
		Summary:       "small code-focused option for everyday laptops",
		SourceURL:     "https://ollama.com/library/qwen2.5-coder",
	},
	{
		Category:      "Coding",
		ID:            "qwen2.5-coder-7b",
		OllamaModel:   "qwen2.5-coder:7b",
		Name:          "Qwen2.5-Coder 7B",
		Download:      "4.7 GB",
		DownloadBytes: 4_700_000_000,
		Context:       "32K",
		Summary:       "balanced code-focused option",
		SourceURL:     "https://ollama.com/library/qwen2.5-coder",
	},
	{
		Category:      "Coding",
		ID:            "qwen2.5-coder-14b",
		OllamaModel:   "qwen2.5-coder:14b",
		Name:          "Qwen2.5-Coder 14B",
		Download:      "9.0 GB",
		DownloadBytes: 9_000_000_000,
		Context:       "32K",
		Summary:       "larger code-focused option for capable local hardware",
		SourceURL:     "https://ollama.com/library/qwen2.5-coder",
	},
	{
		Category:      "Coding",
		ID:            "qwen2.5-coder-32b",
		OllamaModel:   "qwen2.5-coder:32b",
		Name:          "Qwen2.5-Coder 32B",
		Download:      "20 GB",
		DownloadBytes: 20_000_000_000,
		Context:       "32K",
		Summary:       "largest Qwen2.5-Coder option for high-memory hosts",
		SourceURL:     "https://ollama.com/library/qwen2.5-coder",
	},
	{
		Category:      "Coding",
		ID:            "devstral-24b",
		OllamaModel:   "devstral:24b",
		Name:          "Devstral 24B",
		Download:      "14 GB",
		DownloadBytes: 14_000_000_000,
		Context:       "128K",
		Summary:       "agentic coding model with tool use",
		SourceURL:     "https://ollama.com/library/devstral",
	},
	{
		Category:      "Coding",
		ID:            "qwen3-coder-30b",
		OllamaModel:   "qwen3-coder:30b",
		Name:          "Qwen3-Coder 30B",
		Download:      "19 GB",
		DownloadBytes: 19_000_000_000,
		Context:       "256K",
		Summary:       "long-context agentic coding model",
		SourceURL:     "https://ollama.com/library/qwen3-coder",
	},
}

// Catalog returns a copy so callers cannot alter Gator's reviewed selection.
func Catalog() []Model {
	return append([]Model(nil), catalog...)
}

// Resolve accepts a stable Gator catalog ID or its exact Ollama model tag.
func Resolve(value string) (Model, bool) {
	value = strings.TrimSpace(value)
	for _, model := range catalog {
		if value == model.ID || value == model.OllamaModel {
			return model, true
		}
	}
	return Model{}, false
}

// IsCatalogModel reports whether name exactly identifies a reviewed model.
func IsCatalogModel(name string) bool {
	_, found := Resolve(name)
	return found
}

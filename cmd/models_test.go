package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/llm"
)

func TestModelsListPrintsBrainAndDrone(t *testing.T) {
	isolateEnv(t)
	fake := &fakeModelLister{
		models: [][]llm.ModelInfo{
			{{ID: "brain-model"}},
			{{ID: "drone-model"}},
		},
	}
	withModelsLister(t, fake)

	out := executeRoot(t, append(configArgs(t), "models", "list", "--query", "gpt", "--provider", "openai"), "")
	if !strings.Contains(out, "brain: ollama\n  brain-model") {
		t.Fatalf("missing brain output:\n%s", out)
	}
	if !strings.Contains(out, "drone: ollama\n  drone-model") {
		t.Fatalf("missing drone output:\n%s", out)
	}
	if len(fake.opts) != 2 || fake.opts[0].Query != "gpt" || fake.opts[0].Provider != "openai" {
		t.Fatalf("opts = %#v", fake.opts)
	}
}

func TestModelsListUnsupportedConfiguredCLIError(t *testing.T) {
	isolateEnv(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`
[brain]
transport = "qwen-cli"
model = "qwen-test"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out, stderr, err := executeRootErr(t, []string{"--config", path, "models", "list", "brain"}, "")
	if err == nil || !strings.Contains(err.Error(), "model listing failed") {
		t.Fatalf("err = %v stderr=%s", err, stderr)
	}
	if !strings.Contains(out, `model listing unsupported for transport "qwen-cli"`) || !strings.Contains(out, "PAW_BRAIN_MODEL") {
		t.Fatalf("out = %s", out)
	}
}

type fakeModelLister struct {
	models    [][]llm.ModelInfo
	endpoints []llm.EndpointConfig
	opts      []llm.ModelListOptions
}

func (f *fakeModelLister) ListEndpointModels(_ context.Context, endpoint llm.EndpointConfig, opts llm.ModelListOptions) ([]llm.ModelInfo, error) {
	f.endpoints = append(f.endpoints, endpoint)
	f.opts = append(f.opts, opts)
	if len(f.models) == 0 {
		return nil, nil
	}
	models := f.models[0]
	f.models = f.models[1:]
	return models, nil
}

func withModelsLister(t *testing.T, lister endpointModelLister) {
	t.Helper()
	old := modelsLister
	modelsLister = lister
	t.Cleanup(func() { modelsLister = old })
}

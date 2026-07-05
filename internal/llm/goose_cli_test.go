package llm

import "testing"

func TestFactorySupportsGooseCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "goose-cli", Provider: "ollama", Model: "qwen3:8b"}, 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}

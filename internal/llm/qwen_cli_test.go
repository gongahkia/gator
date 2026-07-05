package llm

import "testing"

func TestFactorySupportsQwenCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "qwen-cli", Model: "qwen3-coder-plus"}, 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}

package llm

import "testing"

func TestFactorySupportsAiderCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "aider-cli", Model: "aider-model"}, 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}

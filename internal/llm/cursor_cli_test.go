package llm

import "testing"

func TestFactorySupportsCursorCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "cursor-cli", Model: "cursor-model"}, 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}

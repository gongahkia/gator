package providerhttp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

func TestDiagnosticRetainsUsefulMetadataWithoutErrorBodyOrCredentials(t *testing.T) {
	request, _ := http.NewRequest("POST", "https://user:password@example.test/v1/responses?key=QUERY_SECRET#fragment", nil)
	request.Header.Set("Authorization", "Bearer sk-proj-HEADER_SECRET")
	request.Header.Set("Accept", "text/event-stream")
	response := &http.Response{StatusCode: 401, Header: http.Header{"X-Request-Id": {"req_abc123"}}}
	err := NewError("openai", "OpenAI Responses API", "test-model", "environment", request, response,
		[]byte(`{"error":{"message":"Incorrect API key sk-proj-HEADER_SECRET; input PRIVATE_PROMPT","code":"invalid_api_key","type":"invalid_request_error","param":"model"},"private":"RESPONSE_CONTENT"}`), time.Now().Add(-time.Second))
	d := agent.ProviderDiagnosticFrom(err)
	if d == nil || d.Status != 401 || d.Endpoint != "https://example.test/v1/responses" || d.RequestID != "req_abc123" || d.Code != "invalid_api_key" || d.Type != "invalid_request_error" || d.Parameter != "model" || !d.Streaming || d.Auth != "bearer" || d.ElapsedMS < 1000 {
		t.Fatalf("diagnostic = %+v", d)
	}
	data, _ := json.Marshal(d)
	for _, secret := range []string{"password", "QUERY_SECRET", "HEADER_SECRET", "PRIVATE_PROMPT", "RESPONSE_CONTENT", "Incorrect API key"} {
		if strings.Contains(err.Error()+string(data), secret) {
			t.Errorf("private value was retained: %q", secret)
		}
	}
}

func TestDiagnosticRejectsArbitraryFieldsAndUsesRespondingEndpoint(t *testing.T) {
	original, _ := http.NewRequest("POST", "https://original.test/responses", nil)
	final, _ := http.NewRequest("POST", "https://final.test/path/sk-proj-PATH_SECRET?token=QUERY_SECRET", nil)
	response := &http.Response{Request: final, StatusCode: 403, Header: http.Header{"X-Request-Id": {"sk-proj-ECHOED_SECRET"}, "Request-Id": {"req_safe"}}}
	err := NewError("openai", "OpenAI Responses API", strings.Repeat("m", 300)+"\n", "", original, response,
		[]byte(`{"error":{"code":"PRIVATE_PROMPT","type":"SECRET_TYPE","param":"SECRET_PARAMETER","message":"SECRET_BODY"}}`), time.Now())
	d := agent.ProviderDiagnosticFrom(err)
	if d.RequestID != "req_safe" || d.Code != "" || d.Type != "" || d.Parameter != "" || len(d.Model) != 256 || d.Auth != "none" || !strings.HasPrefix(d.Endpoint, "https://final.test/") {
		t.Fatalf("diagnostic = %+v", d)
	}
	if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "PRIVATE_PROMPT") || strings.Contains(err.Error(), "\n") {
		t.Fatalf("unsafe diagnostic: %v", err)
	}
	for _, body := range []string{"<html>SECRET_BODY</html>", `{"error":{"code":401,"status":"UNAUTHENTICATED","details":[{"reason":"API_KEY_INVALID"}]}}`} {
		d = agent.ProviderDiagnosticFrom(NewError("gemini", "Gemini API", "test", "", original, response, []byte(body), time.Now()))
		if strings.Contains(body, "API_KEY_INVALID") && d.Code != "API_KEY_INVALID" {
			t.Fatalf("Gemini reason lost: %+v", d)
		}
	}
}

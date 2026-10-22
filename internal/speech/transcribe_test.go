package speech

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIDictationUsesMultipartAudioAndPrivateAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("wrong transcription endpoint or credential")
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if string(data) != "audio-fixture" || header.Filename != "dictation.webm" || r.FormValue("model") != DefaultOpenAIModel || r.FormValue("language") != "en" {
			t.Error("multipart recording was not preserved")
		}
		io.WriteString(w, `{"text":"Change my model please.","usage":{"total_tokens":17}}`)
	}))
	defer server.Close()
	result, err := Transcribe(context.Background(), Config{Provider: "openai", APIKey: "test-key", BaseURL: server.URL + "/v1/responses"}, Recording{MediaType: "audio/webm;codecs=opus", Data: []byte("audio-fixture"), Language: "en-SG"})
	if err != nil || result.Text != "Change my model please." || result.Tokens != 17 || !result.Reported {
		t.Fatalf("transcription=%#v %v", result, err)
	}
}

func TestGeminiDictationExcludesThoughtPartsAndNeverSendsTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/configured-model:generateContent" || r.Header.Get("x-goog-api-key") != "gemini-test-key" {
			t.Error("wrong Gemini request")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if _, ok := payload["tools"]; ok {
			t.Error("transcription must never have tools")
		}
		if !strings.Contains(payload["systemInstruction"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"].(string), "Do not answer") {
			t.Error("recording was not treated as transcription data")
		}
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"private thought","thought":true},{"text":"What is my usage?"}]}}],"usageMetadata":{"totalTokenCount":23}}`)
	}))
	defer server.Close()
	result, err := Transcribe(context.Background(), Config{Provider: "gemini", Model: "configured-model", APIKey: "gemini-test-key", BaseURL: server.URL + "/v1beta"}, Recording{MediaType: "audio/webm", Data: []byte("audio-fixture")})
	if err != nil || result.Text != "What is my usage?" || result.Tokens != 23 {
		t.Fatalf("transcription=%#v %v", result, err)
	}
}

func TestDictationRejectsInvalidInputAndDoesNotEchoProviderErrors(t *testing.T) {
	for _, recording := range []Recording{{MediaType: "audio/webm"}, {MediaType: "text/plain", Data: []byte("secret")}, {MediaType: "audio/webm", Data: make([]byte, MaxAudioBytes+1)}} {
		if _, err := Transcribe(context.Background(), Config{}, recording); err == nil {
			t.Fatal("invalid recording accepted")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, "private-provider-body")
	}))
	defer server.Close()
	_, err := Transcribe(context.Background(), Config{Provider: "openai", APIKey: "test-key", BaseURL: server.URL}, Recording{MediaType: "audio/wav", Data: []byte("audio")})
	if err == nil || strings.Contains(err.Error(), "private-provider-body") {
		t.Fatalf("unsafe provider error: %v", err)
	}
}

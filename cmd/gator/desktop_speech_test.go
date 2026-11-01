package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/speech"
	sharedusage "github.com/gongahkia/gator/internal/usage"
)

func TestDesktopSpeechKeepsKeysPrivateAndChargesSharedUsage(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GEMINI_API_KEY", "private-voice-fixture-key")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"Show my usage."}]}}],"usageMetadata":{"totalTokenCount":33}}`)
	}))
	defer server.Close()
	store, _ := config.DefaultStore()
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "gemini", Model: "fixture-speech"}
	settings.ProviderEndpoints = map[string]string{"gemini": server.URL + "/v1beta"}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runDesktopSpeech(strings.NewReader(`{"action":"status"}`), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "private-voice-fixture-key") || strings.Contains(output.String(), server.URL) {
		t.Fatal("voice status exposed private configuration")
	}
	var status desktopVoiceStatus
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Ready || status.Route == "" {
		t.Fatalf("voice not ready: %#v", status)
	}
	payload, _ := json.Marshal(desktopSpeechRequest{Action: "transcribe", Route: status.Route, Recording: &speech.Recording{MediaType: "audio/webm", Data: []byte("fake-audio")}})
	output.Reset()
	if err := runDesktopSpeech(bytes.NewReader(payload), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Show my usage.") || calls != 1 {
		t.Fatalf("transcription=%s calls=%d", output.String(), calls)
	}
	ledger, _ := sharedusage.Open(root)
	snapshot, err := ledger.Snapshot(time.Now())
	if err != nil || snapshot.Categories["voice"] != 1 || snapshot.Tokens != 33 {
		t.Fatalf("voice usage not charged: %#v %v", snapshot, err)
	}
	settings.Desktop.Voice.Language = "en"
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	if err := runDesktopSpeech(bytes.NewReader(payload), io.Discard); err == nil || calls != 1 {
		t.Fatal("recording was uploaded after its selected voice route changed")
	}
	if err := ledger.SavePolicy(sharedusage.Policy{Paused: true}); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := runDesktopSpeech(strings.NewReader(`{"action":"status"}`), &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &status); err != nil || status.Ready || status.Issue == "" {
		t.Fatalf("paused usage allowed microphone setup: %s (%v)", output.String(), err)
	}
}

func TestDesktopLocalVoiceRequiresNoAPIKeyAndDoesNotTouchRemoteUsage(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GATOR_WHISPER_BINARY", "/no-such-gator-whisper-runtime")
	store, _ := config.DefaultStore()
	settings := config.Default()
	settings.Desktop.Voice.Provider = speech.LocalProvider
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runDesktopSpeech(strings.NewReader(`{"action":"status"}`), &output); err != nil {
		t.Fatal(err)
	}
	var status desktopVoiceStatus
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Provider != speech.LocalProvider || status.Model != "base" || status.Ready || status.Local == nil || status.Local.RuntimeReady || len(status.Local.Models) != 3 || status.Route == "" {
		t.Fatalf("local status: %#v", status)
	}
	if strings.Contains(output.String(), root) || strings.Contains(output.String(), "no-such") {
		t.Fatal("local status disclosed host paths")
	}
	if err := runDesktopSpeech(strings.NewReader(`{"action":"download","model":"../../private"}`), io.Discard); err == nil {
		t.Fatal("unselected download accepted")
	}
	if err := runDesktopSpeech(strings.NewReader(`{"action":"status","model":"tiny"}`), io.Discard); err == nil {
		t.Fatal("status silently changed model")
	}
}

func TestNativeDesktopLocalVoice(t *testing.T) {
	if os.Getenv("GATOR_NATIVE_WHISPER_TEST") != "1" {
		t.Skip("opt-in native public-sample voice helper acceptance")
	}
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	var output bytes.Buffer
	if err := runDesktopSpeech(strings.NewReader(`{"action":"configure","preferences":{"provider":"whisper-local","model":"tiny","language":"en"}}`), &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := runDesktopSpeech(strings.NewReader(`{"action":"download","model":"tiny"}`), &output); err != nil {
		t.Fatal(err)
	}
	ledger, err := sharedusage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.SavePolicy(sharedusage.Policy{Paused: true}); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := runDesktopSpeech(strings.NewReader(`{"action":"status"}`), &output); err != nil {
		t.Fatal(err)
	}
	var status desktopVoiceStatus
	if err := json.Unmarshal(output.Bytes(), &status); err != nil || !status.Ready {
		t.Fatalf("local setup unavailable: %v %s", err, output.Bytes())
	}
	wav, err := os.ReadFile(os.Getenv("GATOR_NATIVE_WHISPER_WAV"))
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(desktopSpeechRequest{Action: "transcribe", Route: status.Route, Recording: &speech.Recording{MediaType: "audio/wav", Data: wav}})
	output.Reset()
	if err := runDesktopSpeech(bytes.NewReader(payload), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(output.String()), "country") {
		t.Fatal("public sample speech was not recognized")
	}
	snapshot, err := ledger.Snapshot(time.Now())
	if err != nil || snapshot.Tokens != 0 || snapshot.Categories["voice"] != 0 {
		t.Fatalf("local inference charged remote usage: %#v %v", snapshot, err)
	}
	t.Log("native Go voice-helper setup/download/route/transcription passed with no API key and paused commercial usage")
}

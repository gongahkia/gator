package speech

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt-in: downloads 77.7 MB of pinned public weights into a disposable profile
// and transcribes a caller-selected public WAV. Never accesses a microphone.
func TestNativeLocalWhisper(t *testing.T) {
	if os.Getenv("GATOR_NATIVE_WHISPER_TEST") != "1" {
		t.Skip("opt-in real whisper.cpp/public-model acceptance")
	}
	executable, err := ResolveLocalRuntime()
	if err != nil {
		t.Fatal(err)
	}
	wav, err := os.ReadFile(os.Getenv("GATOR_NATIVE_WHISPER_WAV"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := store.Download(ctx, "tiny"); err != nil {
		t.Fatal(err)
	}
	result, err := Transcribe(ctx, Config{Provider: LocalProvider, Model: "tiny", Local: &LocalConfig{Store: store, Executable: executable}}, Recording{MediaType: "audio/wav", Language: "en", Data: wav})
	if err != nil {
		t.Fatal(err)
	}
	// Upstream samples/jfk.wav: require actual recognition, not nonempty logs.
	if !strings.Contains(strings.ToLower(result.Text), "country") || result.Tokens != 0 {
		t.Fatalf("public JFK sample was not recognized: %#v", result)
	}
	t.Logf("real CPU Whisper recognized public sample (%d characters); no API tokens", len(result.Text))
}

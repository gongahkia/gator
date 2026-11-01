package speech

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func localWAV() []byte {
	data := make([]byte, 44+3200)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 16000)
	binary.LittleEndian.PutUint32(data[28:], 32000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 3200)
	return data
}
func TestLocalWAVStrictBoundsAndFormat(t *testing.T) {
	if err := ValidateLocalWAV(localWAV()); err != nil {
		t.Fatal(err)
	}
	for _, modify := range []func([]byte){func(b []byte) { copy(b, "fake") }, func(b []byte) { binary.LittleEndian.PutUint16(b[22:], 2) }, func(b []byte) { binary.LittleEndian.PutUint32(b[24:], 48000) }, func(b []byte) { binary.LittleEndian.PutUint32(b[40:], 999999) }} {
		data := localWAV()
		modify(data)
		if err := ValidateLocalWAV(data); err == nil {
			t.Fatal("invalid WAV was admitted")
		}
	}
	tooLong := append(localWAV(), make([]byte, 120*32000)...)
	binary.LittleEndian.PutUint32(tooLong[4:], uint32(len(tooLong)-8))
	binary.LittleEndian.PutUint32(tooLong[40:], uint32(len(tooLong)-44))
	if err := ValidateLocalWAV(tooLong); err == nil {
		t.Fatal("overlong audio was admitted")
	}
}
func TestLocalSpeechHelperProcess(t *testing.T) {
	// This is a process fixture, not a claim that real Whisper recognized speech.
	if !strings.Contains(strings.Join(os.Args, " "), "--model") {
		return
	}
	if os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "" {
		os.Exit(3)
	}
	data := new(bytes.Buffer)
	_, _ = data.ReadFrom(os.Stdin)
	if ValidateLocalWAV(data.Bytes()) != nil {
		os.Exit(4)
	}
	args := strings.Join(os.Args, " ")
	switch {
	case strings.Contains(args, "--language fr"):
		_, _ = os.Stdout.WriteString(strings.Repeat("x", MaxTranscriptBytes+1))
	case strings.Contains(args, "--language de"):
		_, _ = os.Stderr.WriteString("private recording or runtime diagnostic")
		os.Exit(6)
	case strings.Contains(args, "--language es"):
		time.Sleep(10 * time.Second)
	default:
		_, _ = os.Stdout.WriteString("Local fixture transcript.")
	}
	os.Exit(0)
}
func TestLocalDictationNeedsNoAPIKeyAndDoesNotUseHTTP(t *testing.T) {
	store, model := localFixture(t)
	if err := os.WriteFile(filepath.Join(store.directory, "ggml-tiny.bin"), model, 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// A tiny wrapper invokes only this test's process fixture. It receives the
	// production argv/stdin path and inherits no provider credentials.
	if filepath.Separator == '\\' {
		t.Skip("Unix fixture launcher; Windows local inference is unsupported")
	}
	wrapper := filepath.Join(t.TempDir(), "whisper-cli")
	script := "#!/bin/sh\nexec " + shellTestQuote(executable) + " -test.run=TestLocalSpeechHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "must-not-reach-native-process")
	t.Setenv("GEMINI_API_KEY", "also-private")
	config := Config{Provider: LocalProvider, Model: "tiny", Local: &LocalConfig{Store: store, Executable: wrapper}}
	result, err := Transcribe(context.Background(), config, Recording{MediaType: "audio/wav", Data: localWAV()})
	if err != nil || result.Text != "Local fixture transcript." || result.Provider != LocalProvider || result.Tokens != 0 {
		t.Fatalf("local result=%#v %v", result, err)
	}
	for _, language := range []string{"fr", "de", "es"} {
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		_, failure := Transcribe(ctx, config, Recording{MediaType: "audio/wav", Language: language, Data: localWAV()})
		cancel()
		if failure == nil || strings.Contains(failure.Error(), "private recording") {
			t.Fatalf("unsafe native failure for %s: %v", language, failure)
		}
	}
	files, _ := os.ReadDir(store.directory)
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".runtime-") {
			t.Fatal("runtime scratch was retained")
		}
	}
}
func shellTestQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

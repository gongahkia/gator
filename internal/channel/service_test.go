package channel

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func TestSlackSignature(t *testing.T) {
	secret, timestamp, body := "secret", strconv.FormatInt(time.Now().UTC().Unix(), 10), []byte(`{"event_id":"e"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + timestamp + ":" + string(body)))
	if !validSlack(secret, timestamp, "v0="+hex.EncodeToString(mac.Sum(nil)), body) {
		t.Fatal("valid signature rejected")
	}
	if validSlack(secret, timestamp, "v0=00", body) {
		t.Fatal("invalid signature accepted")
	}
}

func TestDiscordSignature(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	stamp, body := "1234567890", []byte(`{"type":1}`)
	signature := ed25519.Sign(private, append([]byte(stamp), body...))
	if !validDiscord(hex.EncodeToString(public), hex.EncodeToString(signature), stamp, body) {
		t.Fatal("valid discord signature rejected")
	}
	if validDiscord(hex.EncodeToString(public), hex.EncodeToString(signature), stamp, []byte("changed")) {
		t.Fatal("invalid discord body accepted")
	}
}

func TestSafeFileName(t *testing.T) {
	if got := safeFileName("../../unsafe.txt"); got != "unsafe.txt" {
		t.Fatalf("got %q", got)
	}
	if got := safeFileName("/"); got != "" {
		t.Fatalf("got %q", got)
	}
}

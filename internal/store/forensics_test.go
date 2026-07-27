package store

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestForensicCipherRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	t.Setenv("NORBOT_FORENSICS_TEST_KEY", base64.StdEncoding.EncodeToString(key))
	cipher, err := newForensicCipher("NORBOT_FORENSICS_TEST_KEY")
	if err != nil {
		t.Fatal(err)
	}
	nonce, ciphertext, err := cipher.seal([]byte("raw secret"), []byte("42"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := cipher.open(nonce, ciphertext, []byte("42"))
	if err != nil || string(plaintext) != "raw secret" {
		t.Fatalf("plaintext=%q err=%v", plaintext, err)
	}
	if _, err := cipher.open(nonce, ciphertext, []byte("wrong")); err == nil {
		t.Fatal("expected associated-data failure")
	}
}

func TestTraceCursorIsOpaque(t *testing.T) {
	cursor := encodeTraceCursor(42)
	if cursor == "42" {
		t.Fatal("cursor exposed database id")
	}
	value, err := decodeTraceCursor(cursor)
	if err != nil || value != 42 {
		t.Fatalf("value=%d err=%v", value, err)
	}
}

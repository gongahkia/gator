package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

type forensicCipher struct {
	aead       cipher.AEAD
	keyVersion string
}

func newForensicCipher(keyEnv string) (*forensicCipher, error) {
	value := strings.TrimSpace(os.Getenv(keyEnv))
	if value == "" {
		return nil, fmt.Errorf("forensics master key env %q is empty", keyEnv)
	}
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("forensics master key must be base64-encoded 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &forensicCipher{aead: aead, keyVersion: "local-v1"}, nil
}

func (c *forensicCipher) seal(plaintext, additionalData []byte) ([]byte, []byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return nonce, c.aead.Seal(nil, nonce, plaintext, additionalData), nil
}

func (c *forensicCipher) open(nonce, ciphertext, additionalData []byte) ([]byte, error) {
	return c.aead.Open(nil, nonce, ciphertext, additionalData)
}

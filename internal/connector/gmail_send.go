package connector

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

type gmailSendInput struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// prepareGmailMessage accepts only a single-recipient, plain-text message.
// The human approves the exact typed fields before the resulting MIME is sent.
func prepareGmailMessage(payload []byte, correlationID string) ([]byte, error) {
	messageID, err := gmailMessageID(correlationID)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var input gmailSendInput
	if err := decoder.Decode(&input); err != nil {
		return nil, fmt.Errorf("decode Gmail send fields: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("Gmail send fields have trailing JSON")
	}
	from, err := singleGmailAddress(input.From)
	if err != nil {
		return nil, fmt.Errorf("invalid Gmail sender: %w", err)
	}
	to, err := singleGmailAddress(input.To)
	if err != nil {
		return nil, fmt.Errorf("invalid Gmail recipient: %w", err)
	}
	if !validGmailHeader(input.Subject, 200) || strings.TrimSpace(input.Subject) == "" {
		return nil, errors.New("Gmail subject is invalid")
	}
	if input.Body == "" || len(input.Body) > 10_000 || !utf8.ValidString(input.Body) || strings.ContainsRune(input.Body, 0) {
		return nil, errors.New("Gmail body is invalid or too long")
	}
	var message bytes.Buffer
	fmt.Fprintf(&message, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n", from, to, mime.QEncoding.Encode("utf-8", input.Subject), messageID)
	body := base64.StdEncoding.EncodeToString([]byte(input.Body))
	for len(body) > 76 {
		message.WriteString(body[:76])
		message.WriteString("\r\n")
		body = body[76:]
	}
	message.WriteString(body)
	message.WriteString("\r\n")
	return json.Marshal(struct {
		Raw string `json:"raw"`
	}{Raw: base64.RawURLEncoding.EncodeToString(message.Bytes())})
}

func newGmailCorrelationID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create Gmail reconciliation identity: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func gmailMessageID(correlationID string) (string, error) {
	if len(correlationID) != 32 {
		return "", errors.New("Gmail reconciliation identity is invalid")
	}
	value, err := hex.DecodeString(correlationID)
	if err != nil || hex.EncodeToString(value) != correlationID {
		return "", errors.New("Gmail reconciliation identity is invalid")
	}
	return "<gator-" + correlationID + "@gator.invalid>", nil
}

func singleGmailAddress(value string) (string, error) {
	if !validGmailHeader(value, 254) {
		return "", errors.New("address contains invalid characters")
	}
	for _, letter := range value {
		if letter > unicode.MaxASCII {
			return "", errors.New("address must be ASCII")
		}
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address == nil || address.Name != "" || address.Address != value || !strings.Contains(value, "@") {
		return "", errors.New("use one plain email address without a display name")
	}
	return value, nil
}

func validGmailHeader(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, letter := range value {
		if unicode.IsControl(letter) {
			return false
		}
	}
	return true
}

func validGmailReceiptID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, letter := range value {
		if letter >= 'a' && letter <= 'z' || letter >= 'A' && letter <= 'Z' || letter >= '0' && letter <= '9' || letter == '-' || letter == '_' {
			continue
		}
		return false
	}
	return true
}

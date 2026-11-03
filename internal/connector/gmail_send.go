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
	From        string                     `json:"from"`
	To          gmailRecipientList         `json:"to"`
	CC          []string                   `json:"cc,omitempty"`
	BCC         []string                   `json:"bcc,omitempty"`
	Subject     string                     `json:"subject"`
	Body        string                     `json:"body"`
	Reply       *gmailReply                `json:"reply,omitempty"`
	Attachments []gmailAttachmentReference `json:"attachments,omitempty"`
}

func gmailSendSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","required":["payload"],"additionalProperties":false,"properties":{"payload":{"type":"object","required":["from","to","subject","body"],"additionalProperties":false,"properties":{
"from":{"type":"string","minLength":3,"maxLength":254},
"to":{"anyOf":[{"type":"string","minLength":3,"maxLength":254},{"type":"array","minItems":1,"maxItems":50,"items":{"type":"string","minLength":3,"maxLength":254}}]},
"cc":{"type":"array","maxItems":50,"items":{"type":"string","minLength":3,"maxLength":254}},
"bcc":{"type":"array","maxItems":50,"items":{"type":"string","minLength":3,"maxLength":254}},
"subject":{"type":"string","minLength":1,"maxLength":200},"body":{"type":"string","minLength":1,"maxLength":10000},
"reply":{"type":"object","required":["thread_id","in_reply_to","references"],"additionalProperties":false,"properties":{"thread_id":{"type":"string","minLength":1,"maxLength":128},"in_reply_to":{"type":"string","minLength":3,"maxLength":254},"references":{"type":"array","minItems":1,"maxItems":32,"items":{"type":"string","maxLength":254}}}},
"attachments":{"type":"array","maxItems":10,"items":{"type":"object","additionalProperties":false,"required":["source"],"properties":{"source":{"type":"string","maxLength":1024,"description":"Selected source/... file, staged output/... file, or retained previous/... file. No absolute paths or encoded bytes."},"filename":{"type":"string","maxLength":200},"media_type":{"type":"string","maxLength":128},"sha256":{"type":"string","maxLength":64},"bytes":{"type":"integer","minimum":0,"maximum":8388608}}}}
}}}}`)
}

// GmailPreviewEnvelope projects an approved envelope without message body or
// attachment data. Old single-recipient previews remain readable.
func GmailPreviewEnvelope(preview string) (recipients, subject string) {
	var fields struct {
		To      gmailRecipientList
		CC, BCC []string
		Subject string
	}
	if json.Unmarshal([]byte(preview), &fields) != nil {
		return "", ""
	}
	if len(fields.To) == 1 && len(fields.CC)+len(fields.BCC) == 0 {
		recipients = fields.To[0]
	} else {
		recipients = fmt.Sprintf("%d recipients (To %d, Cc %d, Bcc %d)", len(fields.To)+len(fields.CC)+len(fields.BCC), len(fields.To), len(fields.CC), len(fields.BCC))
	}
	return recipients, fields.Subject
}

type gmailRecipientList []string

// Preserve the original single-address payload while making multiple recipients
// explicit. Comma-separated strings are rejected by address validation.
func (r *gmailRecipientList) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	var single string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &single); err != nil {
			return err
		}
		*r = []string{single}
		return nil
	}
	return json.Unmarshal(data, (*[]string)(r))
}

type gmailReply struct {
	ThreadID   string   `json:"thread_id"`
	InReplyTo  string   `json:"in_reply_to"`
	References []string `json:"references"`
}

func decodeGmailInput(payload []byte) (gmailSendInput, error) {
	var input gmailSendInput
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, fmt.Errorf("decode Gmail fields: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return input, errors.New("Gmail fields have trailing JSON")
	}
	return input, nil
}

// prepareGmailMessage accepts typed recipients and optional reply metadata.
// The human approves the exact typed fields before the resulting MIME is sent.
func prepareGmailMessage(payload []byte, correlationID string, attachments ...gmailAttachment) ([]byte, error) {
	messageID, err := gmailMessageID(correlationID)
	if err != nil {
		return nil, err
	}
	input, err := decodeGmailInput(payload)
	if err != nil {
		return nil, err
	}
	from, err := singleGmailAddress(input.From)
	if err != nil {
		return nil, fmt.Errorf("invalid Gmail sender: %w", err)
	}
	if len(input.To) == 0 || len(input.To)+len(input.CC)+len(input.BCC) > 50 {
		return nil, errors.New("Gmail needs a To recipient and allows at most 50 total recipients")
	}
	seen := map[string]bool{}
	for _, group := range [][]string{input.To, input.CC, input.BCC} {
		for _, address := range group {
			if _, err := singleGmailAddress(address); err != nil {
				return nil, fmt.Errorf("invalid Gmail recipient: %w", err)
			}
			key := strings.ToLower(address)
			if seen[key] {
				return nil, errors.New("Gmail recipients must be distinct across To, Cc and Bcc")
			}
			seen[key] = true
		}
	}
	if err := validateGmailReply(input.Reply); err != nil {
		return nil, err
	}
	if err := validateGmailAttachments(input.Attachments, attachments); err != nil {
		return nil, err
	}
	if !validGmailHeader(input.Subject, 200) || strings.TrimSpace(input.Subject) == "" {
		return nil, errors.New("Gmail subject is invalid")
	}
	if input.Body == "" || len(input.Body) > 10_000 || !utf8.ValidString(input.Body) || strings.ContainsRune(input.Body, 0) {
		return nil, errors.New("Gmail body is invalid or too long")
	}
	var message bytes.Buffer
	fmt.Fprintf(&message, "From: %s\r\n", from)
	writeGmailHeaderList(&message, "To", input.To, ", ")
	writeGmailHeaderList(&message, "Cc", input.CC, ", ")
	writeGmailHeaderList(&message, "Bcc", input.BCC, ", ")
	fmt.Fprintf(&message, "Subject: %s\r\nMessage-ID: %s\r\n", mime.QEncoding.Encode("utf-8", input.Subject), messageID)
	if input.Reply != nil {
		fmt.Fprintf(&message, "In-Reply-To: %s\r\n", input.Reply.InReplyTo)
		writeGmailHeaderList(&message, "References", input.Reply.References, " ")
	}
	if err := writeGmailBody(&message, input.Body, correlationID, attachments); err != nil {
		return nil, err
	}
	threadID := ""
	if input.Reply != nil {
		threadID = input.Reply.ThreadID
	}
	return json.Marshal(struct {
		Raw      string `json:"raw"`
		ThreadID string `json:"threadId,omitempty"`
	}{Raw: base64.RawURLEncoding.EncodeToString(message.Bytes()), ThreadID: threadID})
}

func writeGmailHeaderList(message *bytes.Buffer, name string, values []string, separator string) {
	if len(values) == 0 {
		return
	}
	message.WriteString(name + ": ")
	column := len(name) + 2
	for i, value := range values {
		if i > 0 {
			if column+len(separator)+len(value) > 76 {
				message.WriteString(strings.TrimRight(separator, " ") + "\r\n ")
				column = 1
			} else {
				message.WriteString(separator)
				column += len(separator)
			}
		}
		message.WriteString(value)
		column += len(value)
	}
	message.WriteString("\r\n")
}

func validateGmailReply(reply *gmailReply) error {
	if reply == nil {
		return nil
	}
	if !validGmailReceiptID(reply.ThreadID) || !validGmailMessageID(reply.InReplyTo) || len(reply.References) == 0 || len(reply.References) > 32 || reply.References[len(reply.References)-1] != reply.InReplyTo {
		return errors.New("Gmail reply needs a thread ID, parent Message-ID and References ending with that parent")
	}
	seen := map[string]bool{}
	for _, id := range reply.References {
		if !validGmailMessageID(id) || seen[id] {
			return errors.New("Gmail reply References must be distinct valid Message-IDs")
		}
		seen[id] = true
	}
	return nil
}

func validGmailMessageID(id string) bool {
	if !validGmailHeader(id, 254) || !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, ">") {
		return false
	}
	inner := id[1 : len(id)-1]
	address, err := mail.ParseAddress(inner)
	return err == nil && address.Name == "" && address.Address == inner && strings.Contains(inner, "@") && !strings.ContainsAny(inner, " <>\t\"\\")
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

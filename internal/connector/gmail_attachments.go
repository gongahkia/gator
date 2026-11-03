package connector

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"net/textproto"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxGmailAttachmentBytes int64 = 8 * 1024 * 1024
const maxGmailTotalAttachmentBytes int64 = 16 * 1024 * 1024

type gmailAttachmentReference struct {
	Source    string `json:"source"`
	Filename  string `json:"filename,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256,omitempty"`
}

type gmailAttachment struct {
	reference gmailAttachmentReference
	data      []byte
}

func validGmailAttachmentSource(source string) bool {
	prefix, _, found := strings.Cut(source, "/")
	return found && (prefix == "source" || prefix == "output" || prefix == "previous") && len(source) <= 1024 && fs.ValidPath(source) && !strings.ContainsAny(source, "\\\x00\r\n")
}

func validGmailFilename(name string) bool {
	if name == "" || name == "." || name == ".." || len(name) > 200 || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\") {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Resolve before review and keep private immutable copies. Only metadata enters
// the approval/journal; no file bytes or local absolute paths are exposed.
func (r Runtime) prepareGmailAttachments(payload []byte) ([]byte, []gmailAttachment, error) {
	input, err := decodeGmailInput(payload)
	if err != nil {
		return nil, nil, err
	}
	if len(input.Attachments) == 0 {
		return payload, nil, nil
	}
	if len(input.Attachments) > 10 || r.ResolveAttachment == nil {
		return nil, nil, errors.New("Gmail attachments require selected Work files and at most ten attachments")
	}
	var files []gmailAttachment
	var total int64
	seen := map[string]bool{}
	for i, ref := range input.Attachments {
		if !validGmailAttachmentSource(ref.Source) {
			return nil, nil, errors.New("Gmail attachment source must be a selected source, output or previous file")
		}
		if ref.Filename == "" {
			ref.Filename = path.Base(ref.Source)
		}
		if !validGmailFilename(ref.Filename) || seen[strings.ToLower(ref.Filename)] {
			return nil, nil, errors.New("Gmail attachment names must be distinct ordinary filenames")
		}
		seen[strings.ToLower(ref.Filename)] = true
		data, err := r.ResolveAttachment(ref.Source, maxGmailAttachmentBytes)
		if err != nil {
			return nil, nil, errors.New("Gmail attachment unavailable; inspect the selected file before preparing the message")
		}
		if int64(len(data)) > maxGmailAttachmentBytes {
			return nil, nil, errors.New("Gmail attachment exceeds 8 MiB")
		}
		total += int64(len(data))
		if total > maxGmailTotalAttachmentBytes {
			return nil, nil, errors.New("Gmail attachments exceed 16 MiB total")
		}
		digest := sha256.Sum256(data)
		hash := hex.EncodeToString(digest[:])
		if ref.SHA256 != "" && ref.SHA256 != hash || ref.Bytes != 0 && ref.Bytes != int64(len(data)) {
			return nil, nil, errors.New("Gmail attachment changed; review its current bytes and digest")
		}
		ref.SHA256, ref.Bytes = hash, int64(len(data))
		if ref.MediaType == "" {
			ref.MediaType = mime.TypeByExtension(strings.ToLower(path.Ext(ref.Filename)))
			if ref.MediaType == "" {
				ref.MediaType = "application/octet-stream"
			}
		}
		media, params, err := mime.ParseMediaType(ref.MediaType)
		if err != nil || !strings.Contains(media, "/") || len(params) > 0 && !(strings.HasPrefix(media, "text/") && params["charset"] == "utf-8" && len(params) == 1) || strings.HasPrefix(media, "multipart/") || strings.ContainsAny(ref.MediaType, "\r\n") {
			return nil, nil, errors.New("Gmail attachment media type is invalid")
		}
		ref.MediaType = media
		input.Attachments[i] = ref
		files = append(files, gmailAttachment{reference: ref, data: bytes.Clone(data)})
	}
	// Keep the original envelope representation; normalize only attachment
	// metadata, so single-recipient legacy previews keep their shape.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, nil, err
	}
	fields["attachments"], err = json.Marshal(input.Attachments)
	if err != nil {
		return nil, nil, err
	}
	normalized, err := json.Marshal(fields)
	if err != nil || len(normalized) > maxHTTPActionBytes {
		return nil, nil, errors.New("Gmail envelope and attachment metadata exceed the approval limit")
	}
	return normalized, files, nil
}

func validateGmailAttachments(refs []gmailAttachmentReference, files []gmailAttachment) error {
	if len(refs) != len(files) || len(files) > 10 {
		return errors.New("Gmail attachment bytes are not bound to the reviewed message")
	}
	var total int64
	for i, file := range files {
		ref := refs[i]
		digest := sha256.Sum256(file.data)
		if ref != file.reference || !validGmailAttachmentSource(ref.Source) || !validGmailFilename(ref.Filename) || ref.Bytes != int64(len(file.data)) || ref.SHA256 != hex.EncodeToString(digest[:]) || ref.Bytes > maxGmailAttachmentBytes {
			return errors.New("Gmail attachment does not match its approved bytes")
		}
		total += ref.Bytes
	}
	if total > maxGmailTotalAttachmentBytes {
		return errors.New("Gmail attachments exceed 16 MiB total")
	}
	return nil
}

func writeGmailBody(message *bytes.Buffer, body, correlationID string, files []gmailAttachment) error {
	message.WriteString("MIME-Version: 1.0\r\n")
	if len(files) == 0 {
		message.WriteString("Content-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
		return writeGmailBase64(message, []byte(body))
	}
	writer := multipart.NewWriter(message)
	if err := writer.SetBoundary("gator-" + correlationID); err != nil {
		return err
	}
	message.WriteString("Content-Type: multipart/mixed; boundary=" + writer.Boundary() + "\r\n\r\n")
	part, err := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=UTF-8"}, "Content-Transfer-Encoding": {"base64"}})
	if err != nil {
		return err
	}
	if err := writeGmailBase64(part, []byte(body)); err != nil {
		return err
	}
	for _, file := range files {
		header := textproto.MIMEHeader{"Content-Type": {file.reference.MediaType}, "Content-Transfer-Encoding": {"base64"}, "Content-Disposition": {mime.FormatMediaType("attachment", map[string]string{"filename": file.reference.Filename})}}
		part, err := writer.CreatePart(header)
		if err != nil {
			return err
		}
		if err := writeGmailBase64(part, file.data); err != nil {
			return err
		}
	}
	return writer.Close()
}

func writeGmailBase64(writer io.Writer, data []byte) error {
	var encoded [76]byte
	for len(data) > 0 {
		n := min(57, len(data))
		size := base64.StdEncoding.EncodedLen(n)
		base64.StdEncoding.Encode(encoded[:size], data[:n])
		if _, err := writer.Write(encoded[:size]); err != nil {
			return err
		}
		if _, err := io.WriteString(writer, "\r\n"); err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

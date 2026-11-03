package attachment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Original is an explicitly selected input's original bytes, before document
// extraction or provider adaptation. It stays host-private and is never a model
// Attachment. The Work host assigns an ephemeral reference for exact review.
type Original struct {
	Name string
	Data []byte `json:"-"`
}

type OriginalMetadata struct {
	Source    string `json:"source"`
	Filename  string `json:"filename"`
	MediaType string `json:"media_type"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
}

// Selection has no filesystem root and cannot look up an unselected upload.
// Every reference is private to this task, including files with duplicate names.
type Selection struct {
	files []OriginalMetadata
	data  map[string][]byte
}

func ValidateOriginals(files []Original) error {
	if len(files) > MaxInputs {
		return errors.New("too many original attachments")
	}
	total := 0
	for _, file := range files {
		if file.Name == "" || file.Name == "." || file.Name == ".." || len(file.Name) > 200 || !utf8.ValidString(file.Name) || strings.ContainsAny(file.Name, "/\\") {
			return errors.New("original attachment needs an ordinary filename")
		}
		for _, c := range file.Name {
			if unicode.IsControl(c) {
				return errors.New("original attachment name contains control characters")
			}
		}
		limit := MaxDocumentBytes
		if IsImage(file.Name) {
			limit = MaxImageBytes
		}
		if len(file.Data) == 0 || len(file.Data) > limit {
			return errors.New("original attachment size is invalid")
		}
		total += len(file.Data)
		if total > MaxTotalBytes {
			return errors.New("original attachments exceed the 16 MiB total limit")
		}
	}
	return nil
}

func SelectOriginals(files []Original) (Selection, error) {
	if err := ValidateOriginals(files); err != nil {
		return Selection{}, err
	}
	selected := Selection{data: make(map[string][]byte, len(files))}
	for i, file := range files {
		data := bytes.Clone(file.Data)
		digest := sha256.Sum256(data)
		source := fmt.Sprintf("input/%d/%s", i+1, file.Name)
		media := mime.TypeByExtension(strings.ToLower(path.Ext(file.Name)))
		if media == "" {
			media = "application/octet-stream"
		}
		selected.files = append(selected.files, OriginalMetadata{Source: source, Filename: file.Name, MediaType: media, Bytes: int64(len(data)), SHA256: hex.EncodeToString(digest[:])})
		selected.data[source] = data
	}
	return selected, nil
}

func (s Selection) Files() []OriginalMetadata { return append([]OriginalMetadata{}, s.files...) }

func (s Selection) Read(source string, limit int64) ([]byte, error) {
	data, found := s.data[source]
	if !found || limit < 1 || int64(len(data)) > limit {
		return nil, errors.New("original attachment is not selected or exceeds the read limit")
	}
	return bytes.Clone(data), nil
}

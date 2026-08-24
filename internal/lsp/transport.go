package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// readFrame enforces LSP's Content-Length framing and Gator's message bound.
func readFrame(reader *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if len(line) > 8192 {
			return nil, errors.New("LSP header exceeds 8 KiB")
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			break
		}
		name, value, found := strings.Cut(line, ":")
		if !found {
			return nil, errors.New("invalid LSP header")
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || parsed < 1 || parsed > maxFrame {
				return nil, errors.New("invalid LSP Content-Length")
			}
			length = parsed
		}
	}
	if length < 0 {
		return nil, errors.New("LSP message has no Content-Length")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func requireEOF(decoder *json.Decoder) error {
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected second JSON value")
		}
		return err
	}
	return nil
}

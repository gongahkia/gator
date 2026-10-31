// Copyright 2026 gongahkia
// Licensed under the Apache License, Version 2.0. See third_party/onibi/LICENSE.
// Adapted from Onibi internal/render/text.go. Modified: conservative removal of
// OSC/DCS/SOS/PM/APC/CSI, C0/C1 controls and Unicode format controls; explicit bounds.
package tmux

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func PlainTail(in []byte, lines, maxBytes int) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(in); {
		if in[i] == 0x1b {
			i++
			if i >= len(in) {
				break
			}
			kind := in[i]
			i++
			if kind == '[' {
				for i < len(in) {
					c := in[i]
					i++
					if c >= 0x40 && c <= 0x7e {
						break
					}
				}
			} else if strings.ContainsRune("]PX^_", rune(kind)) {
				for i < len(in) {
					if in[i] == 7 {
						i++
						break
					}
					if in[i] == 0x1b && i+1 < len(in) && in[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
			} else if kind == '(' || kind == ')' {
				if i < len(in) {
					i++
				}
			}
			continue
		}
		r, n := utf8.DecodeRune(in[i:])
		i += n
		if r == utf8.RuneError && n == 1 {
			b.WriteRune('�')
			continue
		}
		if r == '\n' || r == '\t' {
			b.WriteRune(r)
		} else if !unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) {
			b.WriteRune(r)
		}
	}
	text := strings.TrimRight(b.String(), "\n")
	parts := strings.Split(text, "\n")
	truncated := len(parts) > lines
	if truncated {
		text = strings.Join(parts[len(parts)-lines:], "\n")
	}
	if len(text) > maxBytes {
		truncated = true
		text = text[len(text)-maxBytes:]
		for len(text) > 0 && !utf8.RuneStart(text[0]) {
			text = text[1:]
		}
	}
	return text, truncated
}

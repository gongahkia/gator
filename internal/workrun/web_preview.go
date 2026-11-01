package workrun

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const webPreviewBytes = 8 * 1024

// Display-only extraction. The evidence catalog retains the exact fetched
// bytes; normalized HTML text is not a new or independently verified source.
func webPreview(body, contentType string) (string, bool) {
	if !strings.Contains(strings.ToLower(contentType), "html") {
		return boundedEvidenceText(body, 0, webPreviewBytes)
	}
	z := html.NewTokenizer(strings.NewReader(body))
	var out strings.Builder
	var hidden []string
	for {
		t := z.Next()
		if t == html.ErrorToken {
			break
		}
		switch t {
		case html.StartTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if tag == "script" || tag == "style" || tag == "nav" || tag == "footer" || tag == "template" {
				hidden = append(hidden, tag)
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if len(hidden) > 0 && hidden[len(hidden)-1] == string(name) {
				hidden = hidden[:len(hidden)-1]
			}
		case html.TextToken:
			if len(hidden) > 0 {
				continue
			}
			text := strings.Join(strings.Fields(string(z.Text())), " ")
			if text == "" {
				continue
			}
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(text)
			if out.Len() > webPreviewBytes {
				preview, _ := boundedEvidenceText(out.String(), 0, webPreviewBytes)
				return preview, true
			}
		}
	}
	return out.String(), false
}

func boundedEvidenceText(body string, offset, limit int) (string, bool) {
	end := min(len(body), offset+limit)
	for end > offset && end < len(body) && !utf8.RuneStart(body[end]) {
		end--
	}
	return body[offset:end], end < len(body)
}

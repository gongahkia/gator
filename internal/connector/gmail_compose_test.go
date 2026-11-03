package connector

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/action"
)

func composeFixture() map[string]any {
	return map[string]any{"from": "me@example.com", "to": []string{"one@example.com", "two@example.com"}, "cc": []string{"cc@example.com"}, "bcc": []string{"hidden@example.com"}, "subject": "Project update", "body": "The new café plan is ready.", "reply": map[string]any{"thread_id": "thread-1", "in_reply_to": "<parent@example.com>", "references": []string{"<root@example.com>", "<parent@example.com>"}}}
}

func TestGmailReplyRecipientsAndFrozenAttachments(t *testing.T) {
	attachment := []byte("private frozen report bytes\x00\xff")
	original := bytes.Clone(attachment)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/gmail/v1/users/me/messages/send" {
			t.Error("unexpected endpoint", r.Method, r.URL.Path)
		}
		var request struct {
			Raw      string
			ThreadID string `json:"threadId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.ThreadID != "thread-1" {
			t.Error("reply thread lost")
		}
		data, err := base64.RawURLEncoding.DecodeString(request.Raw)
		if err != nil {
			t.Error(err)
			return
		}
		message, err := mail.ReadMessage(bytes.NewReader(data))
		if err != nil {
			t.Error(err)
			return
		}
		for field, want := range map[string][]string{"To": {"one@example.com", "two@example.com"}, "Cc": {"cc@example.com"}, "Bcc": {"hidden@example.com"}} {
			addresses, err := mail.ParseAddressList(message.Header.Get(field))
			if err != nil || len(addresses) != len(want) {
				t.Error("invalid recipient header", field, err)
				continue
			}
			for i, a := range addresses {
				if a.Address != want[i] {
					t.Error("recipient changed", field)
				}
			}
		}
		if message.Header.Get("In-Reply-To") != "<parent@example.com>" || message.Header.Get("References") != "<root@example.com> <parent@example.com>" {
			t.Error("reply MIME headers changed")
		}
		media, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
		if err != nil || media != "multipart/mixed" {
			t.Error("invalid multipart header", err)
			return
		}
		reader := multipart.NewReader(message.Body, params["boundary"])
		body, err := reader.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		text, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, body))
		if err != nil || string(text) != "The new café plan is ready." {
			t.Error("body bytes changed", err)
		}
		file, err := reader.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		_, disposition, err := mime.ParseMediaType(file.Header.Get("Content-Disposition"))
		if err != nil || disposition["filename"] != "café-report.bin" {
			t.Error("attachment filename changed", err)
		}
		content, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, file))
		if err != nil || !bytes.Equal(content, original) {
			t.Error("approved attachment bytes changed", err)
		}
		if _, err := reader.NextPart(); err != io.EOF {
			t.Error("unexpected MIME part", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"sent-1","threadId":"thread-1"}`)
	}))
	defer server.Close()
	d := Descriptor{Version: 1, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{d})
	if err != nil {
		t.Fatal(err)
	}
	r := Runtime{Registry: registry, HTTPClient: server.Client(), ResolveAttachment: func(source string, limit int64) ([]byte, error) {
		if source != "output/report.bin" || limit != maxGmailAttachmentBytes {
			t.Fatal("attachment scope changed")
		}
		return attachment, nil
	}}
	payload := composeFixture()
	payload["attachments"] = []map[string]any{{"source": "output/report.bin", "filename": "café-report.bin"}}
	input, _ := json.Marshal(map[string]any{"payload": payload})
	prepared, err := r.PrepareAction("google", "gmail_send", input)
	if err != nil || calls != 0 {
		t.Fatal("prepare failed or sent data", err)
	}
	if !strings.Contains(prepared.Proposal.Preview, `"sha256":`) || !strings.Contains(prepared.Proposal.Preview, "hidden@example.com") || strings.Contains(prepared.Proposal.Preview, "private frozen") {
		t.Fatal("approval did not show exact envelope and attachment metadata")
	}
	attachment[0] = 'X' // Even a shared resolver buffer cannot alter the approved send.
	if _, err := r.ExecutePreparedWithReceipt(context.Background(), action.Draft, prepared); err == nil || calls != 0 {
		t.Fatal("draft bypassed approval")
	}
	if receipt, err := r.ExecutePreparedWithReceipt(context.Background(), action.Act, prepared); err != nil || receipt != "sent-1" || calls != 1 {
		t.Fatal("approved reply failed", receipt, calls, err)
	}
}

func TestGmailRejectsAmbiguousRecipientsAndReplyHeaders(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"duplicate":     func(p map[string]any) { p["cc"] = []string{"ONE@example.com"} },
		"bcc injection": func(p map[string]any) { p["bcc"] = []string{"bad@example.com\r\nTo: another@example.com"} },
		"missing To":    func(p map[string]any) { p["to"] = []string{} },
		"too many": func(p map[string]any) {
			var a []string
			for i := 0; i < 51; i++ {
				a = append(a, "person"+strings.Repeat("x", i)+"@example.com")
			}
			p["to"] = a
		},
		"missing thread": func(p map[string]any) {
			p["reply"] = map[string]any{"in_reply_to": "<a@example.com>", "references": []string{"<a@example.com>"}}
		},
		"parent mismatch": func(p map[string]any) {
			p["reply"] = map[string]any{"thread_id": "thread", "in_reply_to": "<a@example.com>", "references": []string{"<b@example.com>"}}
		},
		"header injection": func(p map[string]any) {
			p["reply"] = map[string]any{"thread_id": "thread", "in_reply_to": "<a@example.com>\r\nBcc: bad@example.com", "references": []string{"<a@example.com>"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := composeFixture()
			mutate(p)
			data, _ := json.Marshal(p)
			if _, err := prepareGmailMessage(data, strings.Repeat("0", 32)); err == nil {
				t.Fatal("invalid envelope accepted")
			}
		})
	}
}

func TestGmailAttachmentsRefuseUnselectedPathsAndChangedData(t *testing.T) {
	for _, ref := range []map[string]any{
		{"source": "/private/file"}, {"source": "source/../outside"}, {"source": "output/x", "filename": "bad\r\nBcc.txt"}, {"source": "output/x", "media_type": "multipart/mixed"}, {"source": "output/x", "sha256": strings.Repeat("0", 64)}, {"source": "output/x", "bytes": 999},
	} {
		p := composeFixture()
		p["attachments"] = []map[string]any{ref}
		data, _ := json.Marshal(p)
		r := Runtime{ResolveAttachment: func(string, int64) ([]byte, error) { return []byte("actual bytes"), nil }}
		if _, _, err := r.prepareGmailAttachments(data); err == nil {
			t.Fatal("invalid attachment accepted", ref)
		}
	}
	p := composeFixture()
	p["attachments"] = []map[string]any{{"source": "output/report.pdf"}}
	data, _ := json.Marshal(p)
	if _, _, err := (Runtime{}).prepareGmailAttachments(data); err == nil {
		t.Fatal("unbound attachment reader accepted")
	}
	r := Runtime{ResolveAttachment: func(string, int64) ([]byte, error) { return make([]byte, maxGmailAttachmentBytes+1), nil }}
	if _, _, err := r.prepareGmailAttachments(data); err == nil {
		t.Fatal("oversized attachment accepted")
	}
	r.ResolveAttachment = func(string, int64) ([]byte, error) { return []byte("frozen"), nil }
	normalized, files, err := r.prepareGmailAttachments(data)
	if err != nil {
		t.Fatal(err)
	}
	files[0].data[0] = 'X'
	if _, err := prepareGmailMessage(normalized, strings.Repeat("0", 32), files...); err == nil {
		t.Fatal("tampered private attachment sent")
	}
}

func TestGmailReplyThreadMismatchIsUncertain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"sent-1","threadId":"other-thread"}`)
	}))
	defer server.Close()
	d := Descriptor{Version: 1, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, _ := NewRegistry([]Descriptor{d})
	r := Runtime{Registry: registry, HTTPClient: server.Client()}
	input, _ := json.Marshal(map[string]any{"payload": composeFixture()})
	p, err := r.PrepareAction("google", "gmail_send", input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.ExecutePreparedWithReceipt(context.Background(), action.Act, p)
	var unknown interface{ OutcomeUncertain() bool }
	if !errors.As(err, &unknown) || !unknown.OutcomeUncertain() {
		t.Fatal("thread mismatch treated as confirmed send", err)
	}
}

func TestGmailAttachmentAggregateAndCountLimits(t *testing.T) {
	for _, c := range []struct {
		name        string
		count, size int
		wantError   bool
	}{
		{"at total limit", 2, int(maxGmailAttachmentBytes), false},
		{"over total limit", 3, int(maxGmailAttachmentBytes), true},
		{"too many empty files", 11, 0, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := composeFixture()
			refs := make([]map[string]any, c.count)
			for i := range refs {
				refs[i] = map[string]any{"source": fmt.Sprintf("output/file-%d.bin", i)}
			}
			p["attachments"] = refs
			payload, _ := json.Marshal(p)
			data := make([]byte, c.size)
			r := Runtime{ResolveAttachment: func(string, int64) ([]byte, error) { return data, nil }}
			_, files, err := r.prepareGmailAttachments(payload)
			if (err != nil) != c.wantError || err == nil && len(files) != c.count {
				t.Fatal("attachment limit result", len(files), err)
			}
		})
	}
}

func TestGmailRecipientHeadersFoldWithoutChangingAddresses(t *testing.T) {
	p := composeFixture()
	to := make([]string, 50)
	for i := range to {
		to[i] = fmt.Sprintf("person-%02d-with-a-long-name@example.com", i)
	}
	p["to"], p["cc"], p["bcc"] = to, nil, nil
	payload, _ := json.Marshal(p)
	raw, err := prepareGmailMessage(payload, strings.Repeat("0", 32))
	if err != nil {
		t.Fatal(err)
	}
	var request struct{ Raw string }
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	data, err := base64.RawURLEncoding.DecodeString(request.Raw)
	if err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	addresses, err := mail.ParseAddressList(message.Header.Get("To"))
	if err != nil || len(addresses) != len(to) {
		t.Fatal("folded recipients lost", len(addresses), err)
	}
	for i, address := range addresses {
		if address.Address != to[i] {
			t.Fatal("folded address changed", i)
		}
	}
}

func TestGmailThreadReadIsScopedAndKeepsProvenance(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/gmail/v1/users/me/threads/thread-1" || r.URL.Query().Get("format") != "full" {
			t.Error("wrong thread read endpoint")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"thread-1","messages":[]}`)
	}))
	defer server.Close()
	d := Descriptor{Version: 1, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, _ := NewRegistry([]Descriptor{d})
	r := Runtime{Registry: registry, HTTPClient: server.Client()}
	result, err := r.Invoke(context.Background(), action.Inspect, "google", "gmail_thread_get", []byte(`{"resource_id":"thread-1"}`))
	if err != nil || calls != 1 || result.Provenance.Operation != "gmail_thread_get" {
		t.Fatal("thread read failed", err)
	}
	if _, err := r.Invoke(context.Background(), action.Inspect, "google", "gmail_thread_get", []byte(`{"resource_id":"../bad"}`)); err == nil || calls != 1 {
		t.Fatal("invalid thread sent network request")
	}
	d.Authentication = AuthOAuth
	d.OAuthClientID = "client"
	d.OAuthAuthorizeURL = server.URL + "/auth"
	d.OAuthTokenURL = server.URL + "/token"
	d.OAuthRedirectURL = GoogleOAuthRedirectURL
	d.OAuthScopes = "openid"
	for _, op := range d.Operations() {
		if op.ID == "gmail_thread_get" {
			t.Fatal("thread read exposed without mail scope")
		}
	}
}

func TestGmailThreadReadInheritsMessageContentRestrictions(t *testing.T) {
	for _, restriction := range []Permission{PermissionAsk, PermissionDeny} {
		policy := PermissionSet{{ConnectorID: "google", Operation: "gmail_get", Read: restriction}, {ConnectorID: "google", Operation: "gmail_thread_get", Read: PermissionAllow}}
		if got := policy.Resolve("google", "gmail_thread_get", action.ConnectedRead); got != restriction {
			t.Fatal("thread read bypassed prior message restriction", restriction, got)
		}
	}
}

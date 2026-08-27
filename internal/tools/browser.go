package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/gongahkia/gator/internal/agent"
	"golang.org/x/net/html"
)

const (
	maxBrowserTextBytes      = 32 * 1024
	maxBrowserMetadataBytes  = 24 * 1024
	maxBrowserReferenceBytes = 2 * 1024
	maxBrowserActionBytes    = 32 * 1024
	maxBrowserLinks          = 128
	maxBrowserForms          = 32
	maxBrowserFields         = 64
)

// boundedBrowser is an ephemeral document browser, not a general computer-use
// surface. It does not execute JavaScript, load subresources, use cookies,
// follow redirects, persist a profile, or expose arbitrary DOM evaluation.
// Every network transition is an independently approved, DNS-pinned request.
type boundedBrowser struct {
	mu      sync.Mutex
	policy  CommandPolicy
	options HTTPFetchOptions
	budget  *httpFetchBudget
	memory  *httpApprovalMemory
	page    browserPage
}

type browserPage struct {
	URL           string        `json:"url"`
	Status        int           `json:"status"`
	ContentType   string        `json:"content_type,omitempty"`
	Title         string        `json:"title,omitempty"`
	Text          string        `json:"text"`
	Links         []browserLink `json:"links,omitempty"`
	Forms         []browserForm `json:"forms,omitempty"`
	BodyTruncated bool          `json:"response_truncated"`
}

type browserLink struct {
	Ref  string `json:"ref"`
	Text string `json:"text,omitempty"`
	URL  string `json:"url"`
}

type browserForm struct {
	Ref    string         `json:"ref"`
	Method string         `json:"method"`
	Action string         `json:"action"`
	Fields []browserField `json:"fields,omitempty"`
}

type browserField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type browserNavigate struct{ browser *boundedBrowser }
type browserSnapshot struct{ browser *boundedBrowser }
type browserExtract struct{ browser *boundedBrowser }
type browserAct struct{ browser *boundedBrowser }

func newBrowserTools(policy CommandPolicy, options HTTPFetchOptions, budget *httpFetchBudget) []agent.Tool {
	browser := &boundedBrowser{policy: policy, options: options, budget: budget, memory: newHTTPApprovalMemory()}
	return []agent.Tool{
		browserNavigate{browser}, browserSnapshot{browser}, browserExtract{browser}, browserAct{browser},
	}
}

func (browserNavigate) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "browser_navigate",
		Description: "Navigate the ephemeral bounded document browser to one public HTTPS URL. Each exact destination requires developer approval. The browser fetches only the main document through a DNS-pinned, proxy-free connection; it does not execute JavaScript, load subresources, follow redirects, use cookies, or persist a profile.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["url"],"properties":{"url":{"type":"string","minLength":1,"maxLength":8192}}}`),
	}
}

func (t browserNavigate) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		URL string `json:"url"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	return t.browser.navigate(ctx, http.MethodGet, arguments.URL, nil, "browser_navigate")
}

func (browserSnapshot) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "browser_snapshot",
		Description: "Return a bounded accessibility-style snapshot of the currently loaded document: URL, title, normalized visible text, and stable references for links and forms. It performs no network request and returns no raw DOM, scripts, hidden values, cookies, or credentials.",
		Parameters:  schema(`{"type":"object","additionalProperties":false}`),
	}
}

func (t browserSnapshot) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct{}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	t.browser.mu.Lock()
	defer t.browser.mu.Unlock()
	if t.browser.page.URL == "" {
		return agent.ToolResult{}, errors.New("browser has no loaded document")
	}
	content, err := success(t.browser.page)
	return agent.ToolResult{Content: content}, err
}

func (browserExtract) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "browser_extract",
		Description: "Extract bounded text, links, or forms from the current document. The optional case-insensitive query filters visible text or reference metadata. This is read-only and performs no JavaScript or network request.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["kind"],"properties":{"kind":{"type":"string","enum":["text","links","forms"]},"query":{"type":"string","maxLength":400}}}`),
	}
}

func (t browserExtract) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Kind  string `json:"kind"`
		Query string `json:"query"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	query := strings.ToLower(strings.TrimSpace(arguments.Query))
	t.browser.mu.Lock()
	defer t.browser.mu.Unlock()
	page := t.browser.page
	if page.URL == "" {
		return agent.ToolResult{}, errors.New("browser has no loaded document")
	}
	var value any
	switch arguments.Kind {
	case "text":
		text := page.Text
		if query != "" {
			lines := strings.Split(text, "\n")
			matches := lines[:0]
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), query) {
					matches = append(matches, line)
				}
			}
			text = strings.Join(matches, "\n")
		}
		value = struct {
			URL  string `json:"url"`
			Text string `json:"text"`
		}{page.URL, boundedBrowserText(text)}
	case "links":
		links := make([]browserLink, 0, len(page.Links))
		for _, link := range page.Links {
			if query == "" || strings.Contains(strings.ToLower(link.Text+" "+link.URL), query) {
				links = append(links, link)
			}
		}
		value = links
	case "forms":
		forms := make([]browserForm, 0, len(page.Forms))
		for _, form := range page.Forms {
			encoded, _ := json.Marshal(form)
			if query == "" || strings.Contains(strings.ToLower(string(encoded)), query) {
				forms = append(forms, form)
			}
		}
		value = forms
	default:
		return agent.ToolResult{}, fmt.Errorf("unsupported browser extraction kind %q", arguments.Kind)
	}
	content, err := success(value)
	return agent.ToolResult{Content: content}, err
}

func (browserAct) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "browser_act",
		Description: "Perform one bounded action against a reference from the latest snapshot: click a link, or submit a GET/POST HTML form with explicitly supplied non-secret fields. The destination and method require developer approval before any request. Password/file inputs, JavaScript URLs, cookies, redirects, popups, downloads, and arbitrary DOM or computer control are unsupported.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["action","ref"],"properties":{"action":{"type":"string","enum":["click","submit"]},"ref":{"type":"string","pattern":"^(link|form)-[1-9][0-9]*$"},"fields":{"type":"object","maxProperties":64,"additionalProperties":{"type":"string","maxLength":8192}}}}`),
	}
}

func (t browserAct) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Action string            `json:"action"`
		Ref    string            `json:"ref"`
		Fields map[string]string `json:"fields"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	t.browser.mu.Lock()
	defer t.browser.mu.Unlock()
	if t.browser.page.URL == "" {
		return agent.ToolResult{}, errors.New("browser has no loaded document")
	}
	switch arguments.Action {
	case "click":
		if len(arguments.Fields) > 0 {
			return agent.ToolResult{}, errors.New("browser click does not accept fields")
		}
		for _, link := range t.browser.page.Links {
			if link.Ref == arguments.Ref {
				return t.browser.navigateLocked(ctx, http.MethodGet, link.URL, nil, "browser_act")
			}
		}
		return agent.ToolResult{}, fmt.Errorf("browser link reference %q is not in the latest snapshot", arguments.Ref)
	case "submit":
		for _, form := range t.browser.page.Forms {
			if form.Ref != arguments.Ref {
				continue
			}
			values := make(url.Values)
			allowed := make(map[string]string, len(form.Fields))
			for _, field := range form.Fields {
				allowed[field.Name] = field.Type
			}
			for name, value := range arguments.Fields {
				fieldType, found := allowed[name]
				if !found {
					return agent.ToolResult{}, fmt.Errorf("browser form field %q is not declared in the latest snapshot", name)
				}
				if fieldType == "password" || fieldType == "file" {
					return agent.ToolResult{}, fmt.Errorf("browser refuses %s field %q", fieldType, name)
				}
				values.Set(name, value)
			}
			if form.Method == http.MethodGet {
				target, err := url.Parse(form.Action)
				if err != nil {
					return agent.ToolResult{}, err
				}
				target.RawQuery = values.Encode()
				return t.browser.navigateLocked(ctx, http.MethodGet, target.String(), nil, "browser_act")
			}
			return t.browser.navigateLocked(ctx, http.MethodPost, form.Action, values, "browser_act")
		}
		return agent.ToolResult{}, fmt.Errorf("browser form reference %q is not in the latest snapshot", arguments.Ref)
	default:
		return agent.ToolResult{}, fmt.Errorf("unsupported browser action %q", arguments.Action)
	}
}

func (b *boundedBrowser) navigate(ctx context.Context, method, target string, values url.Values, tool string) (agent.ToolResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.navigateLocked(ctx, method, target, values, tool)
}

func (b *boundedBrowser) navigateLocked(ctx context.Context, method, target string, values url.Values, tool string) (agent.ToolResult, error) {
	if len(target) > maxHTTPURLBytes {
		return agent.ToolResult{}, fmt.Errorf("browser URL exceeds the %d-byte limit", maxHTTPURLBytes)
	}
	requestURL, canonicalURL, err := normalizeHTTPURL(target)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if method != http.MethodGet && method != http.MethodPost {
		return agent.ToolResult{}, errors.New("browser permits GET and POST only")
	}
	requestBody := ""
	if method == http.MethodPost {
		requestBody = values.Encode()
		if len(requestBody) > maxBrowserActionBytes {
			return agent.ToolResult{}, fmt.Errorf("browser form body exceeds the %d KiB limit", maxBrowserActionBytes/1024)
		}
	}
	if !b.budget.available() {
		return agent.ToolResult{}, fmt.Errorf("browser run budget exceeded; at most %d web requests may run", maxHTTPFetchesPerRun)
	}
	approvalKey := method + " " + canonicalURL
	argv := []string{"browser", strings.ToLower(method), canonicalURL}
	if !b.memory.allows(approvalKey) {
		b.emit(agent.Event{Kind: agent.EventCommandApprovalRequested, ToolCall: &agent.ToolCall{Name: tool}, Text: strings.Join(argv, " "), Argv: cloneArgv(argv)})
		if b.policy.Approve == nil {
			b.emitResolved(tool, argv, CommandDeny)
			return agent.ToolResult{}, errors.New("browser navigation requires developer approval")
		}
		decision, approveErr := b.policy.Approve(ctx, cloneArgv(argv))
		if approveErr != nil {
			b.emitResolved(tool, argv, CommandDeny)
			return agent.ToolResult{}, approveErr
		}
		b.emitResolved(tool, argv, decision)
		if decision == CommandAllowAlways {
			b.memory.remember(approvalKey)
		}
		if decision != CommandAllowOnce && decision != CommandAllowAlways {
			return agent.ToolResult{}, fmt.Errorf("browser request %q denied by developer", canonicalURL)
		}
	}
	addresses, err := resolvePublicHost(ctx, requestURL.Hostname(), b.options.Resolver)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if !b.budget.reserve() {
		return agent.ToolResult{}, errors.New("browser request budget changed before the request could start")
	}
	var body *strings.Reader
	if method == http.MethodPost {
		body = strings.NewReader(requestBody)
	} else {
		body = strings.NewReader("")
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("create browser request: %w", err)
	}
	request.Header.Set("Accept", "text/html, application/xhtml+xml, text/plain;q=0.8")
	request.Header.Set("User-Agent", "Gator/1.0 bounded-browser (+https://github.com/gongahkia/gator)")
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	client := b.options.Client
	if client == nil {
		client = newPublicHTTPClient(requestURL.Hostname(), addresses, b.options.Timeout)
	}
	response, err := client.Do(request)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("browser request %s: %w", canonicalURL, err)
	}
	if response == nil || response.Body == nil {
		return agent.ToolResult{}, errors.New("browser client returned no response body")
	}
	defer response.Body.Close()
	contentType := response.Header.Get("Content-Type")
	if !textHTTPContentType(contentType) {
		return agent.ToolResult{}, fmt.Errorf("browser refuses non-text response with content type %q", contentType)
	}
	contents, truncated, err := readHTTPBody(response.Body, b.options.MaxResponseBytes)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("read browser response: %w", err)
	}
	if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		result, encodeErr := success(struct {
			URL      string `json:"url"`
			Status   int    `json:"status"`
			Redirect string `json:"redirect,omitempty"`
			Loaded   bool   `json:"loaded"`
		}{canonicalURL, response.StatusCode, response.Header.Get("Location"), false})
		return agent.ToolResult{Content: result}, encodeErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return agent.ToolResult{}, fmt.Errorf("browser destination returned HTTP %d", response.StatusCode)
	}
	page, err := parseBrowserPage(canonicalURL, response.StatusCode, contentType, contents, truncated)
	if err != nil {
		return agent.ToolResult{}, err
	}
	b.page = page
	result, encodeErr := success(struct {
		URL               string `json:"url"`
		Status            int    `json:"status"`
		Title             string `json:"title,omitempty"`
		Links             int    `json:"links"`
		Forms             int    `json:"forms"`
		ResponseTruncated bool   `json:"response_truncated"`
	}{page.URL, page.Status, page.Title, len(page.Links), len(page.Forms), page.BodyTruncated})
	return agent.ToolResult{Content: result}, encodeErr
}

func parseBrowserPage(base string, status int, contentType string, contents []byte, truncated bool) (browserPage, error) {
	document, err := html.Parse(strings.NewReader(strings.ToValidUTF8(string(contents), "�")))
	if err != nil {
		return browserPage{}, fmt.Errorf("parse browser document: %w", err)
	}
	baseURL, _ := url.Parse(base)
	page := browserPage{URL: base, Status: status, ContentType: contentType, BodyTruncated: truncated}
	var text strings.Builder
	metadataBytes := 0
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "script", "style", "noscript", "template":
				return
			case "title":
				page.Title = boundedBrowserLabel(nodeText(node))
			case "a":
				if len(page.Links) < maxBrowserLinks {
					if target := safeBrowserReference(baseURL, attribute(node, "href")); target != "" {
						link := browserLink{Ref: fmt.Sprintf("link-%d", len(page.Links)+1), Text: boundedBrowserLabel(nodeText(node)), URL: target}
						encoded, _ := json.Marshal(link)
						if metadataBytes+len(encoded) <= maxBrowserMetadataBytes {
							page.Links = append(page.Links, link)
							metadataBytes += len(encoded)
						}
					}
				}
			case "form":
				if len(page.Forms) < maxBrowserForms {
					if form, ok := parseBrowserForm(baseURL, node, len(page.Forms)+1); ok {
						encoded, _ := json.Marshal(form)
						if metadataBytes+len(encoded) <= maxBrowserMetadataBytes {
							page.Forms = append(page.Forms, form)
							metadataBytes += len(encoded)
						}
					}
				}
			}
		}
		if node.Type == html.TextNode {
			value := strings.Join(strings.Fields(node.Data), " ")
			if value != "" && text.Len() < maxBrowserTextBytes {
				text.WriteString(value)
				text.WriteByte('\n')
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	page.Text = boundedBrowserText(text.String())
	return page, nil
}

func parseBrowserForm(base *url.URL, node *html.Node, index int) (browserForm, bool) {
	method := strings.ToUpper(strings.TrimSpace(attribute(node, "method")))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodPost {
		return browserForm{}, false
	}
	rawAction := strings.TrimSpace(attribute(node, "action"))
	action := ""
	if rawAction == "" {
		action = base.String()
	} else {
		action = safeBrowserReference(base, rawAction)
		if action == "" {
			return browserForm{}, false
		}
	}
	form := browserForm{Ref: fmt.Sprintf("form-%d", index), Method: method, Action: action}
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if len(form.Fields) >= maxBrowserFields {
			return
		}
		if current.Type == html.ElementNode && (current.Data == "input" || current.Data == "textarea" || current.Data == "select") {
			name := strings.TrimSpace(attribute(current, "name"))
			if name != "" && len(name) <= 256 {
				fieldType := strings.ToLower(strings.TrimSpace(attribute(current, "type")))
				if fieldType == "" {
					fieldType = current.Data
				}
				form.Fields = append(form.Fields, browserField{Name: name, Type: fieldType})
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	sort.Slice(form.Fields, func(i, j int) bool { return form.Fields[i].Name < form.Fields[j].Name })
	return form, true
}

func safeBrowserReference(base *url.URL, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	reference, err := url.Parse(value)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(reference)
	_, canonical, err := normalizeHTTPURL(resolved.String())
	if err != nil || len(canonical) > maxBrowserReferenceBytes {
		return ""
	}
	return canonical
}

func nodeText(node *html.Node) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			text.WriteString(current.Data)
			text.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return text.String()
}

func attribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return attribute.Val
		}
	}
	return ""
}

func boundedBrowserText(value string) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, "�"))
	if len(value) <= maxBrowserTextBytes {
		return value
	}
	return value[:maxBrowserTextBytes-len("\n[truncated]")] + "\n[truncated]"
}

func boundedBrowserLabel(value string) string {
	value = strings.Join(strings.Fields(strings.ToValidUTF8(value, "�")), " ")
	if len(value) <= 512 {
		return value
	}
	return value[:512-len("[truncated]")] + "[truncated]"
}

func (b *boundedBrowser) emit(event agent.Event) {
	if b.policy.OnEvent != nil {
		b.policy.OnEvent(event)
	}
}

func (b *boundedBrowser) emitResolved(tool string, argv []string, decision CommandDecision) {
	b.emit(agent.Event{Kind: agent.EventCommandApprovalResolved, ToolCall: &agent.ToolCall{Name: tool}, Text: decision.String(), Argv: cloneArgv(argv)})
}

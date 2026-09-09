package connector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/mcp"
)

const maxHTTPJSONBytes = 256 * 1024

const maxHTTPActionBytes = 12 * 1024

// PreparedAction contains a proposal and connector-private payload. Callers
// can review the proposal but can execute the payload only through Runtime,
// which revalidates their binding before any network request.
type PreparedAction struct {
	Proposal   action.Proposal
	descriptor Descriptor
	operation  Operation
	payload    []byte
}

// Runtime invokes user-configured connectors under a work-mode capability
// ceiling. Credentials are resolved only through a descriptor's resource-bound
// reference and never appear in results or provenance.
type Runtime struct {
	Registry    Registry
	Credentials auth.Store
	HTTPClient  *http.Client
	Now         func() time.Time
}

// Invoke validates a connector operation and its exact input before any I/O.
func (r Runtime) Invoke(ctx context.Context, mode action.Mode, connectorID, operationID string, input json.RawMessage) (Result, error) {
	if err := mode.Validate(); err != nil {
		return Result{}, err
	}
	descriptor, operation, err := r.Registry.Operation(connectorID, operationID)
	if err != nil {
		return Result{}, err
	}
	if !mode.Allows(operation.Capability) {
		return Result{}, fmt.Errorf("work mode %q does not allow capability %q", mode, operation.Capability)
	}
	switch {
	case descriptor.Kind == KindHTTPJSON && operation.ID == "fetch":
		if err := decodeEmptyInput(input); err != nil {
			return Result{}, err
		}
		return r.fetchHTTPJSON(ctx, descriptor)
	case isServiceKind(descriptor.Kind):
		return r.invokeService(ctx, descriptor, operation, input)
	case descriptor.Kind == KindRemoteMCP:
		return r.invokeRemoteMCP(ctx, descriptor, operation, input)
	default:
		return Result{}, fmt.Errorf("connector operation %s/%s has no runtime", descriptor.ID, operation.ID)
	}
}

// PrepareAction validates and pins a high-risk connector payload without
// performing network I/O. The returned proposal contains the exact compact
// JSON body as its human-reviewable preview and binds it by SHA-256.
func (r Runtime) PrepareAction(connectorID, operationID string, input json.RawMessage) (PreparedAction, error) {
	descriptor, operation, err := r.Registry.Operation(connectorID, operationID)
	if err != nil {
		return PreparedAction{}, err
	}
	if !action.RequiresFreshApproval(operation.Capability) {
		return PreparedAction{}, fmt.Errorf("connector operation %s/%s is not an external action", descriptor.ID, operation.ID)
	}
	if descriptor.Kind != KindHTTPWebhook && !isServiceKind(descriptor.Kind) && descriptor.Kind != KindRemoteMCP {
		return PreparedAction{}, fmt.Errorf("connector operation %s/%s has no action runtime", descriptor.ID, operation.ID)
	}
	payload, err := decodeActionInput(input)
	if err != nil {
		return PreparedAction{}, err
	}
	target := descriptor.Resource
	if isServiceKind(descriptor.Kind) {
		target, payload, err = serviceAction(descriptor, operation.ID, payload)
		if err != nil {
			return PreparedAction{}, err
		}
	} else if descriptor.Kind == KindRemoteMCP {
		target = descriptor.Resource + "#tool=" + descriptor.ActionTool
	}
	digest := sha256.Sum256(append([]byte(descriptor.ID+"\x00"+operation.ID+"\x00"+target+"\x00"), payload...))
	proposal, err := action.NewProposal(
		"action-"+hex.EncodeToString(digest[:8]), operation.Capability,
		descriptor.ID, operation.ID, target, string(payload), payload,
	)
	if err != nil {
		return PreparedAction{}, err
	}
	return PreparedAction{
		Proposal: proposal, descriptor: descriptor, operation: operation,
		payload: append([]byte(nil), payload...),
	}, nil
}

// ExecutePrepared performs the first mutating operation for a prepared action.
// It is intended to be called only inside action.Broker's approved closure.
func (r Runtime) ExecutePrepared(ctx context.Context, mode action.Mode, prepared PreparedAction) error {
	if err := mode.Validate(); err != nil {
		return err
	}
	if !mode.Allows(prepared.operation.Capability) || !action.RequiresFreshApproval(prepared.operation.Capability) {
		return fmt.Errorf("work mode %q does not allow capability %q", mode, prepared.operation.Capability)
	}
	descriptor, operation, err := r.Registry.Operation(prepared.descriptor.ID, prepared.operation.ID)
	if err != nil {
		return err
	}
	if descriptor != prepared.descriptor || operation.ID != prepared.operation.ID || operation.Capability != prepared.operation.Capability {
		return errors.New("prepared connector action no longer matches the configured operation")
	}
	payloadDigest := sha256.Sum256(prepared.payload)
	if hex.EncodeToString(payloadDigest[:]) != prepared.Proposal.PayloadSHA256 {
		return errors.New("prepared connector action payload does not match its proposal")
	}
	if prepared.descriptor.Kind == KindHTTPWebhook {
		return r.publishHTTPJSON(ctx, descriptor, prepared.payload)
	}
	if prepared.descriptor.Kind == KindRemoteMCP {
		token, tokenErr := r.bearerToken(ctx, descriptor)
		if tokenErr != nil {
			return tokenErr
		}
		_, callErr := mcp.InvokeRemoteTool(ctx, descriptor.Resource, token, descriptor.ID, descriptor.ActionTool, prepared.payload)
		if callErr != nil {
			return action.MarkUncertain(callErr)
		}
		return nil
	}
	method := http.MethodPost
	if descriptor.Kind == KindNotion && operation.ID == "page_update" || descriptor.Kind == KindSlack && operation.ID == "update_message" {
		method = http.MethodPatch
		if descriptor.Kind == KindSlack {
			method = http.MethodPost
		}
	}
	_, err = r.requestJSON(ctx, descriptor, operation.ID, method, prepared.Proposal.Target, prepared.payload, true)
	return err
}

func (r Runtime) invokeRemoteMCP(ctx context.Context, descriptor Descriptor, operation Operation, raw json.RawMessage) (Result, error) {
	var input struct {
		Arguments json.RawMessage `json:"arguments"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || !json.Valid(input.Arguments) {
		return Result{}, errors.New("remote MCP connector requires one arguments object")
	}
	tool := descriptor.SearchTool
	if operation.ID == "read" {
		tool = descriptor.ReadTool
	}
	token, err := r.bearerToken(ctx, descriptor)
	if err != nil {
		return Result{}, err
	}
	result, err := mcp.InvokeRemoteTool(ctx, descriptor.Resource, token, descriptor.ID, tool, input.Arguments)
	if err != nil {
		return Result{}, err
	}
	digest := sha256.Sum256(result)
	return Result{Data: result, Provenance: Provenance{ConnectorID: descriptor.ID, Operation: operation.ID, Resource: descriptor.Resource, RetrievedAt: r.now(), Bytes: int64(len(result)), SHA256: hex.EncodeToString(digest[:])}}, nil
}

type serviceInput struct {
	Query      string `json:"query,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

func (r Runtime) invokeService(ctx context.Context, descriptor Descriptor, operation Operation, raw json.RawMessage) (Result, error) {
	var input serviceInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return Result{}, fmt.Errorf("decode connector operation input: %w", err)
	}
	if input.Limit < 0 || input.Limit > 100 || len(input.Query) > 4096 || len(input.ResourceID) > 2048 || len(input.Cursor) > 2048 {
		return Result{}, errors.New("connector operation input exceeds its limits")
	}
	endpoint, method, body, err := serviceRead(descriptor, operation.ID, input)
	if err != nil {
		return Result{}, err
	}
	return r.requestJSON(ctx, descriptor, operation.ID, method, endpoint, body, false)
}

func serviceRead(descriptor Descriptor, operation string, input serviceInput) (string, string, []byte, error) {
	query := url.Values{}
	path := ""
	method := http.MethodGet
	var body []byte
	switch descriptor.Kind + "/" + operation {
	case KindSlack + "/whoami":
		path = "/auth.test"
	case KindSlack + "/search":
		if input.Query == "" {
			return "", "", nil, errors.New("Slack search requires query")
		}
		path = "/search.messages"
		query.Set("query", input.Query)
		if input.Limit > 0 {
			query.Set("count", fmt.Sprint(input.Limit))
		}
	case KindSlack + "/history":
		if input.ResourceID == "" {
			return "", "", nil, errors.New("Slack history requires resource_id channel")
		}
		path = "/conversations.history"
		query.Set("channel", input.ResourceID)
		if input.Limit > 0 {
			query.Set("limit", fmt.Sprint(input.Limit))
		}
		if input.Cursor != "" {
			query.Set("cursor", input.Cursor)
		}
	case KindGoogle + "/drive_search":
		path = "/drive/v3/files"
		query.Set("q", input.Query)
		if input.Limit > 0 {
			query.Set("pageSize", fmt.Sprint(input.Limit))
		}
		if input.Cursor != "" {
			query.Set("pageToken", input.Cursor)
		}
	case KindGoogle + "/drive_get":
		path = "/drive/v3/files/" + url.PathEscape(input.ResourceID)
	case KindGoogle + "/docs_get":
		path = "/v1/documents/" + url.PathEscape(input.ResourceID)
	case KindGoogle + "/sheets_get":
		path = "/v4/spreadsheets/" + url.PathEscape(input.ResourceID)
	case KindAtlassian + "/jira_search":
		path = "/rest/api/3/search/jql"
		query.Set("jql", input.Query)
		if input.Limit > 0 {
			query.Set("maxResults", fmt.Sprint(input.Limit))
		}
		if input.Cursor != "" {
			query.Set("nextPageToken", input.Cursor)
		}
	case KindAtlassian + "/jira_get":
		path = "/rest/api/3/issue/" + url.PathEscape(input.ResourceID)
	case KindAtlassian + "/confluence_search":
		path = "/wiki/rest/api/search"
		query.Set("cql", input.Query)
		if input.Limit > 0 {
			query.Set("limit", fmt.Sprint(input.Limit))
		}
		if input.Cursor != "" {
			query.Set("cursor", input.Cursor)
		}
	case KindNotion + "/search":
		path = "/search"
		method = http.MethodPost
		parameters := map[string]any{}
		if input.Query != "" {
			parameters["query"] = input.Query
		}
		if input.Limit > 0 {
			parameters["page_size"] = input.Limit
		}
		if input.Cursor != "" {
			parameters["start_cursor"] = input.Cursor
		}
		body, _ = json.Marshal(parameters)
	case KindNotion + "/page_get":
		path = "/pages/" + url.PathEscape(input.ResourceID)
	case KindNotion + "/block_children":
		path = "/blocks/" + url.PathEscape(input.ResourceID) + "/children"
		if input.Limit > 0 {
			query.Set("page_size", fmt.Sprint(input.Limit))
		}
		if input.Cursor != "" {
			query.Set("start_cursor", input.Cursor)
		}
	default:
		return "", "", nil, fmt.Errorf("connector operation %s/%s has no read runtime", descriptor.ID, operation)
	}
	if strings.Contains(path, "//") || strings.HasSuffix(path, "/") && input.ResourceID == "" && operation != "drive_search" {
		return "", "", nil, errors.New("connector operation requires resource_id")
	}
	endpoint := serviceBase(descriptor, operation) + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint, method, body, nil
}

func serviceAction(descriptor Descriptor, operation string, payload []byte) (string, []byte, error) {
	path := ""
	switch descriptor.Kind + "/" + operation {
	case KindSlack + "/post_message":
		path = "/chat.postMessage"
	case KindSlack + "/update_message":
		path = "/chat.update"
	case KindGoogle + "/docs_create":
		path = "/v1/documents"
	case KindGoogle + "/sheets_create":
		path = "/v4/spreadsheets"
	case KindAtlassian + "/jira_create":
		path = "/rest/api/3/issue"
	case KindAtlassian + "/confluence_create":
		path = "/wiki/api/v2/pages"
	case KindAtlassian + "/jira_comment":
		id, sanitized, err := extractResourceID(payload)
		if err != nil {
			return "", nil, err
		}
		path = "/rest/api/3/issue/" + url.PathEscape(id) + "/comment"
		payload = sanitized
	case KindNotion + "/page_create":
		path = "/pages"
	case KindNotion + "/page_update":
		id, sanitized, err := extractResourceID(payload)
		if err != nil {
			return "", nil, err
		}
		path = "/pages/" + url.PathEscape(id)
		payload = sanitized
	default:
		return "", nil, fmt.Errorf("connector operation %s/%s has no action runtime", descriptor.ID, operation)
	}
	return serviceBase(descriptor, operation) + path, payload, nil
}

func serviceBase(descriptor Descriptor, operation string) string {
	base := strings.TrimRight(descriptor.Resource, "/")
	if descriptor.Kind != KindGoogle || base != "https://www.googleapis.com" {
		return base
	}
	if strings.HasPrefix(operation, "docs_") {
		return "https://docs.googleapis.com"
	}
	if strings.HasPrefix(operation, "sheets_") {
		return "https://sheets.googleapis.com"
	}
	return base
}

func extractResourceID(payload []byte) (string, []byte, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		return "", nil, err
	}
	var id string
	if err := json.Unmarshal(object["resource_id"], &id); err != nil || strings.TrimSpace(id) == "" {
		return "", nil, errors.New("connector action payload requires resource_id")
	}
	delete(object, "resource_id")
	result, err := json.Marshal(object)
	return id, result, err
}

func isServiceKind(kind string) bool {
	return kind == KindSlack || kind == KindGoogle || kind == KindAtlassian || kind == KindNotion
}

func (r Runtime) requestJSON(ctx context.Context, descriptor Descriptor, operation, method, endpoint string, body []byte, uncertain bool) (Result, error) {
	token, err := r.bearerToken(ctx, descriptor)
	if err != nil {
		return Result{}, err
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "gator-work/2")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if descriptor.Kind == KindNotion {
		request.Header.Set("Notion-Version", "2026-03-11")
	}
	response, err := r.httpClient().Do(request)
	if err != nil {
		if uncertain {
			return Result{}, action.MarkUncertain(fmt.Errorf("invoke connector %q: %w", descriptor.ID, err))
		}
		return Result{}, fmt.Errorf("invoke connector %q: %w", descriptor.ID, err)
	}
	defer response.Body.Close()
	contents, readErr := io.ReadAll(io.LimitReader(response.Body, maxHTTPJSONBytes+1))
	if readErr != nil {
		if uncertain {
			return Result{}, action.MarkUncertain(readErr)
		}
		return Result{}, readErr
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		err := fmt.Errorf("connector %q returned HTTP %d", descriptor.ID, response.StatusCode)
		if uncertain {
			return Result{}, action.MarkUncertain(err)
		}
		return Result{}, err
	}
	if len(contents) == 0 {
		contents = []byte(`{}`)
	}
	if len(contents) > maxHTTPJSONBytes || !json.Valid(contents) {
		if uncertain {
			return Result{}, action.MarkUncertain(errors.New("connector returned invalid or oversized JSON after action"))
		}
		return Result{}, errors.New("connector returned invalid or oversized JSON")
	}
	if descriptor.Kind == KindSlack {
		var slack struct {
			OK    *bool  `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(contents, &slack); err == nil && slack.OK != nil && !*slack.OK {
			message := strings.TrimSpace(slack.Error)
			if message == "" {
				message = "unknown_error"
			}
			return Result{}, fmt.Errorf("Slack connector %q rejected %s: %s", descriptor.ID, operation, message)
		}
	}
	digest := sha256.Sum256(contents)
	return Result{Data: append(json.RawMessage(nil), contents...), Provenance: Provenance{ConnectorID: descriptor.ID, Operation: operation, Resource: endpoint, RetrievedAt: r.now(), Bytes: int64(len(contents)), SHA256: hex.EncodeToString(digest[:])}}, nil
}

func (r Runtime) fetchHTTPJSON(ctx context.Context, descriptor Descriptor) (Result, error) {
	if err := descriptor.Validate(); err != nil {
		return Result{}, err
	}
	token, err := r.bearerToken(ctx, descriptor)
	if err != nil {
		return Result{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, descriptor.Resource, nil)
	if err != nil {
		return Result{}, fmt.Errorf("create connector request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "gator-work/1")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := r.httpClient().Do(request)
	if err != nil {
		return Result{}, fmt.Errorf("fetch connector %q: %w", descriptor.ID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("connector %q returned HTTP %d", descriptor.ID, response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
		return Result{}, fmt.Errorf("connector %q returned non-JSON content type %q", descriptor.ID, response.Header.Get("Content-Type"))
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxHTTPJSONBytes+1))
	if err != nil {
		return Result{}, fmt.Errorf("read connector %q response: %w", descriptor.ID, err)
	}
	if len(contents) > maxHTTPJSONBytes {
		return Result{}, fmt.Errorf("connector %q response exceeds 256 KiB", descriptor.ID)
	}
	if err := validateSingleJSON(contents); err != nil {
		return Result{}, fmt.Errorf("connector %q returned invalid JSON: %w", descriptor.ID, err)
	}
	digest := sha256.Sum256(contents)
	return Result{
		Data: append(json.RawMessage(nil), contents...),
		Provenance: Provenance{
			ConnectorID: descriptor.ID, Operation: "fetch", Resource: descriptor.Resource,
			RetrievedAt: r.now(), Bytes: int64(len(contents)), SHA256: hex.EncodeToString(digest[:]),
		},
	}, nil
}

func (r Runtime) publishHTTPJSON(ctx context.Context, descriptor Descriptor, payload []byte) error {
	if err := descriptor.Validate(); err != nil {
		return err
	}
	token, err := r.bearerToken(ctx, descriptor)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, descriptor.Resource, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create connector action request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "gator-work/1")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := r.httpClient()
	response, err := client.Do(request)
	if err != nil {
		return action.MarkUncertain(fmt.Errorf("publish connector %q: %w", descriptor.ID, err))
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return action.MarkUncertain(fmt.Errorf("connector %q returned HTTP %d after publish", descriptor.ID, response.StatusCode))
	}
	return nil
}

func decodeEmptyInput(input json.RawMessage) error {
	if len(input) == 0 {
		return errors.New("connector operation input is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	var value map[string]json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode connector operation input: %w", err)
	}
	if value == nil || len(value) != 0 {
		return errors.New("connector operation input must be an empty object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("connector operation input contains trailing JSON")
	}
	return nil
}

func decodeActionInput(input json.RawMessage) ([]byte, error) {
	if len(input) == 0 || len(input) > maxHTTPActionBytes*2 {
		return nil, errors.New("connector action input is missing or too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	var value struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode connector action input: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("connector action input contains trailing JSON")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(value.Payload, &object); err != nil || object == nil {
		return nil, errors.New("connector action payload must be one JSON object")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value.Payload); err != nil {
		return nil, fmt.Errorf("compact connector action payload: %w", err)
	}
	if compact.Len() == 0 || compact.Len() > maxHTTPActionBytes {
		return nil, fmt.Errorf("connector action payload exceeds %d KiB", maxHTTPActionBytes/1024)
	}
	return compact.Bytes(), nil
}

func actionInput(payload []byte) json.RawMessage {
	result := make([]byte, 0, len(payload)+16)
	result = append(result, `{"payload":`...)
	result = append(result, payload...)
	result = append(result, '}')
	return result
}

func (r Runtime) bearerToken(ctx context.Context, descriptor Descriptor) (string, error) {
	if descriptor.Authentication != AuthBearer && descriptor.Authentication != AuthOAuth {
		return "", nil
	}
	credential, found, err := r.Credentials.Read(descriptor.CredentialRef())
	if err != nil {
		return "", fmt.Errorf("read connector credential: %w", err)
	}
	if !found {
		return "", fmt.Errorf("connector %q is not authenticated", descriptor.ID)
	}
	if credential.Expired(r.now()) {
		if descriptor.Authentication != AuthOAuth || !credential.IsOAuth() || credential.Refresh == "" {
			return "", fmt.Errorf("connector %q credential has expired", descriptor.ID)
		}
		flow := auth.BrowserFlow{ClientID: descriptor.OAuthClientID, TokenURL: descriptor.OAuthTokenURL, AllowMissingExpiry: true, RequireBearerToken: true, HTTPClient: r.HTTPClient}
		refreshed, refreshErr := flow.Refresh(ctx, credential)
		if refreshErr != nil {
			return "", fmt.Errorf("refresh connector %q credential: %w", descriptor.ID, refreshErr)
		}
		if err := r.Credentials.Put(descriptor.CredentialRef(), refreshed); err != nil {
			return "", fmt.Errorf("store refreshed connector credential: %w", err)
		}
		credential = refreshed
	}
	if !credential.IsBearerToken() && !credential.IsOAuth() {
		return "", fmt.Errorf("connector %q requires a bearer credential", descriptor.ID)
	}
	return credential.Access, nil
}

func (r Runtime) httpClient() *http.Client {
	base := r.HTTPClient
	if base == nil {
		base = &http.Client{Timeout: 30 * time.Second}
	}
	client := *base
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}

func validateSingleJSON(contents []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("response contains more than one JSON value")
		}
		return err
	}
	return nil
}

func (r Runtime) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

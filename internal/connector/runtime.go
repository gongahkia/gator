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
	"github.com/gongahkia/gator/internal/googlework"
	"github.com/gongahkia/gator/internal/mcp"
)

const maxHTTPJSONBytes = 256 * 1024

const maxHTTPActionBytes = 12 * 1024

// PreparedAction contains a proposal and connector-private payload. Callers
// can review the proposal but can execute the payload only through Runtime,
// which revalidates their binding before any network request.
type PreparedAction struct {
	Proposal       action.Proposal
	descriptor     Descriptor
	operation      Operation
	target         string
	payload        []byte // approval-bound payload, including any ETag
	requestPayload []byte // endpoint-specific body, never exposed to the approver
	ifMatch        string
	googleSteps    []googlePreparedStep
}

type googlePreparedStep struct {
	operation string
	target    string
	payload   []byte
	ifMatch   string
}

// Runtime invokes user-configured connectors under a work-mode capability
// ceiling. Credentials are resolved only through a descriptor's resource-bound
// reference and never appear in results or provenance.
type Runtime struct {
	Registry    Registry
	Credentials auth.Store
	HTTPClient  *http.Client
	Now         func() time.Time
	// StateDir is Gator's resolved state root. Google uses it only for its
	// private mirror; it never imports another application's state or stores
	// credentials there.
	StateDir string
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
	case descriptor.Kind == KindGoogle && operation.ID == "local_search":
		return r.invokeGoogleLocalSearch(ctx, descriptor, operation, input)
	case descriptor.Kind == KindGoogle && operation.ID == "quick_capture":
		return r.invokeGoogleQuickCapture(descriptor, operation, input)
	case descriptor.Kind == KindGoogle && (operation.ID == "task_metadata_encode" || operation.ID == "task_metadata_decode"):
		return r.invokeGoogleTaskMetadata(descriptor, operation, input)
	case descriptor.Kind == KindGoogle && operation.ID == "reminders_due":
		return r.invokeGoogleReminders(ctx, descriptor, operation, input)
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
	requestPayload := append([]byte(nil), payload...)
	ifMatch := ""
	var googleSteps []googlePreparedStep
	if descriptor.Kind == KindGoogle && operation.ID == "batch" {
		googleSteps, payload, err = prepareGoogleBatch(descriptor, payload)
		if err != nil {
			return PreparedAction{}, err
		}
		target = strings.TrimRight(descriptor.Resource, "/") + "/gator/google-batch"
	}
	if isServiceKind(descriptor.Kind) {
		if !(descriptor.Kind == KindGoogle && operation.ID == "batch") {
			preconditionPayload, precondition, preconditionErr := googlePrecondition(descriptor, payload)
			if preconditionErr != nil {
				return PreparedAction{}, preconditionErr
			}
			target, requestPayload, err = serviceAction(descriptor, operation.ID, preconditionPayload)
			if err != nil {
				return PreparedAction{}, err
			}
			ifMatch = precondition
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
		target: target, payload: append([]byte(nil), payload...), requestPayload: append([]byte(nil), requestPayload...), ifMatch: ifMatch, googleSteps: googleSteps,
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
	actionDigest := sha256.Sum256(append([]byte(descriptor.ID+"\x00"+operation.ID+"\x00"+prepared.target+"\x00"), prepared.payload...))
	if prepared.Proposal.ConnectorID != descriptor.ID || prepared.Proposal.Operation != operation.ID || prepared.Proposal.Capability != operation.Capability || prepared.Proposal.Target != prepared.target || prepared.Proposal.Preview != string(prepared.payload) || prepared.Proposal.ID != "action-"+hex.EncodeToString(actionDigest[:8]) || hex.EncodeToString(payloadDigest[:]) != prepared.Proposal.PayloadSHA256 {
		return errors.New("prepared connector action payload does not match its proposal")
	}
	if prepared.descriptor.Kind == KindHTTPWebhook {
		return r.publishHTTPJSON(ctx, descriptor, prepared.requestPayload)
	}
	if prepared.descriptor.Kind == KindRemoteMCP {
		token, tokenErr := r.bearerToken(ctx, descriptor)
		if tokenErr != nil {
			return tokenErr
		}
		_, callErr := mcp.InvokeRemoteTool(ctx, descriptor.Resource, token, descriptor.ID, descriptor.ActionTool, prepared.requestPayload)
		if callErr != nil {
			return action.MarkUncertain(callErr)
		}
		return nil
	}
	if descriptor.Kind == KindGoogle && operation.ID == "batch" {
		return r.executeGoogleBatch(ctx, descriptor, prepared.googleSteps)
	}
	method := serviceActionMethod(descriptor, operation.ID)
	result, err := r.requestJSONWithPrecondition(ctx, descriptor, operation.ID, method, prepared.target, prepared.requestPayload, prepared.ifMatch, true)
	if err != nil {
		return err
	}
	r.mirrorGoogle(ctx, descriptor, operation.ID, result.Data)
	return nil
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
	Query            string          `json:"query,omitempty"`
	ResourceID       string          `json:"resource_id,omitempty"`
	Cursor           string          `json:"cursor,omitempty"`
	Limit            int             `json:"limit,omitempty"`
	Payload          json.RawMessage `json:"payload,omitempty"`
	TimeMin          string          `json:"time_min,omitempty"`
	TimeMax          string          `json:"time_max,omitempty"`
	SingleEvents     *bool           `json:"single_events,omitempty"`
	IncludeCompleted *bool           `json:"include_completed,omitempty"`
	IncludeHidden    *bool           `json:"include_hidden,omitempty"`
	IncludeDeleted   *bool           `json:"include_deleted,omitempty"`
	Fields           string          `json:"fields,omitempty"`
}

func (r Runtime) invokeService(ctx context.Context, descriptor Descriptor, operation Operation, raw json.RawMessage) (Result, error) {
	var input serviceInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return Result{}, fmt.Errorf("decode connector operation input: %w", err)
	}
	if input.Limit < 0 || input.Limit > 100 || len(input.Query) > 4096 || len(input.ResourceID) > 2048 || len(input.Cursor) > 2048 || len(input.Payload) > maxHTTPActionBytes || len(input.TimeMin) > 64 || len(input.TimeMax) > 64 || len(input.Fields) > 4096 {
		return Result{}, errors.New("connector operation input exceeds its limits")
	}
	for _, value := range []string{input.TimeMin, input.TimeMax} {
		if value != "" {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return Result{}, errors.New("Google time bounds must be RFC3339 timestamps")
			}
		}
	}
	if len(input.Payload) > 0 {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(input.Payload, &object); err != nil || object == nil {
			return Result{}, errors.New("connector operation payload must be one JSON object")
		}
	}
	endpoint, method, body, err := serviceRead(descriptor, operation.ID, input)
	if err != nil {
		return Result{}, err
	}
	result, err := r.requestJSON(ctx, descriptor, operation.ID, method, endpoint, body, false)
	if err != nil {
		return Result{}, err
	}
	r.mirrorGoogle(ctx, descriptor, operation.ID, result.Data)
	return result, nil
}

func serviceRead(descriptor Descriptor, operation string, input serviceInput) (string, string, []byte, error) {
	query := url.Values{}
	path := ""
	method := http.MethodGet
	var body []byte
	switch descriptor.Kind + "/" + operation {
	case KindGoogle + "/health":
		path = "/oauth2/v3/userinfo"
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
		if input.Fields != "" {
			query.Set("fields", input.Fields)
		}
	case KindGoogle + "/drive_get":
		if input.ResourceID == "" {
			return "", "", nil, errors.New("Google Drive metadata requires resource_id")
		}
		path = "/drive/v3/files/" + url.PathEscape(input.ResourceID)
		if input.Fields != "" {
			query.Set("fields", input.Fields)
		}
	case KindGoogle + "/docs_get":
		if input.ResourceID == "" {
			return "", "", nil, errors.New("Google Docs read requires resource_id")
		}
		path = "/v1/documents/" + url.PathEscape(input.ResourceID)
	case KindGoogle + "/sheets_get":
		if input.ResourceID == "" {
			return "", "", nil, errors.New("Google Sheets read requires resource_id")
		}
		path = "/v4/spreadsheets/" + url.PathEscape(input.ResourceID)
	case KindGoogle + "/sheets_values_get":
		sheetID, cellRange, err := splitGoogleID(input.ResourceID, "sheet ID and range")
		if err != nil {
			return "", "", nil, err
		}
		path = "/v4/spreadsheets/" + url.PathEscape(sheetID) + "/values/" + url.PathEscape(cellRange)
	case KindGoogle + "/tasklists_list":
		path = "/tasks/v1/users/@me/lists"
		if input.Limit > 0 {
			query.Set("maxResults", fmt.Sprint(input.Limit))
		}
		if input.Cursor != "" {
			query.Set("pageToken", input.Cursor)
		}
	case KindGoogle + "/tasklists_get":
		if input.ResourceID == "" {
			return "", "", nil, errors.New("Google Task-list read requires resource_id")
		}
		path = "/tasks/v1/users/@me/lists/" + url.PathEscape(input.ResourceID)
	case KindGoogle + "/tasks_list":
		if input.ResourceID == "" {
			return "", "", nil, errors.New("Google Tasks list requires resource_id task list")
		}
		path = "/tasks/v1/lists/" + url.PathEscape(input.ResourceID) + "/tasks"
		if input.Limit > 0 {
			query.Set("maxResults", fmt.Sprint(input.Limit))
		}
		if input.Cursor != "" {
			query.Set("pageToken", input.Cursor)
		}
		showCompleted := true
		if input.IncludeCompleted != nil {
			showCompleted = *input.IncludeCompleted
		}
		showHidden := true
		if input.IncludeHidden != nil {
			showHidden = *input.IncludeHidden
		}
		query.Set("showCompleted", fmt.Sprint(showCompleted))
		query.Set("showHidden", fmt.Sprint(showHidden))
		if input.IncludeDeleted != nil {
			query.Set("showDeleted", fmt.Sprint(*input.IncludeDeleted))
		}
	case KindGoogle + "/tasks_get":
		listID, taskID, err := splitGoogleID(input.ResourceID, "task-list ID and task ID")
		if err != nil {
			return "", "", nil, err
		}
		path = "/tasks/v1/lists/" + url.PathEscape(listID) + "/tasks/" + url.PathEscape(taskID)
	case KindGoogle + "/calendars_list":
		path = "/calendar/v3/users/me/calendarList"
		if input.Limit > 0 {
			query.Set("maxResults", fmt.Sprint(input.Limit))
		}
		if input.Cursor != "" {
			query.Set("pageToken", input.Cursor)
		}
		if input.IncludeHidden != nil {
			query.Set("showHidden", fmt.Sprint(*input.IncludeHidden))
		}
		if input.IncludeDeleted != nil {
			query.Set("showDeleted", fmt.Sprint(*input.IncludeDeleted))
		}
	case KindGoogle + "/calendars_get":
		if input.ResourceID == "" {
			return "", "", nil, errors.New("Google Calendar read requires resource_id")
		}
		path = "/calendar/v3/calendars/" + url.PathEscape(input.ResourceID)
	case KindGoogle + "/calendar_colors":
		path = "/calendar/v3/colors"
	case KindGoogle + "/events_list":
		if input.ResourceID == "" {
			return "", "", nil, errors.New("Google Calendar event list requires resource_id calendar")
		}
		path = "/calendar/v3/calendars/" + url.PathEscape(input.ResourceID) + "/events"
		if input.Limit > 0 {
			query.Set("maxResults", fmt.Sprint(input.Limit))
		}
		if input.Cursor != "" {
			query.Set("pageToken", input.Cursor)
		}
		if input.TimeMin != "" {
			query.Set("timeMin", input.TimeMin)
		}
		if input.TimeMax != "" {
			query.Set("timeMax", input.TimeMax)
		}
		if input.SingleEvents != nil {
			query.Set("singleEvents", fmt.Sprint(*input.SingleEvents))
		}
		if input.IncludeDeleted != nil {
			query.Set("showDeleted", fmt.Sprint(*input.IncludeDeleted))
		}
	case KindGoogle + "/events_get":
		calendarID, eventID, err := splitGoogleID(input.ResourceID, "calendar ID and event ID")
		if err != nil {
			return "", "", nil, err
		}
		path = "/calendar/v3/calendars/" + url.PathEscape(calendarID) + "/events/" + url.PathEscape(eventID)
	case KindGoogle + "/freebusy":
		if len(input.Payload) == 0 {
			return "", "", nil, errors.New("Google free/busy requires payload")
		}
		path, method, body = "/calendar/v3/freeBusy", http.MethodPost, append([]byte(nil), input.Payload...)
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
	case KindGoogle + "/docs_update":
		id, sanitized, err := extractResourceID(payload)
		if err != nil {
			return "", nil, err
		}
		path, payload = "/v1/documents/"+url.PathEscape(id)+":batchUpdate", sanitized
	case KindGoogle + "/sheets_create":
		path = "/v4/spreadsheets"
	case KindGoogle + "/sheets_values_update":
		id, cellRange, sanitized, err := extractGoogleCompositeID(payload, "sheet ID and range")
		if err != nil {
			return "", nil, err
		}
		path, payload = "/v4/spreadsheets/"+url.PathEscape(id)+"/values/"+url.PathEscape(cellRange), sanitized
	case KindGoogle + "/sheets_batch_update":
		id, sanitized, err := extractResourceID(payload)
		if err != nil {
			return "", nil, err
		}
		path, payload = "/v4/spreadsheets/"+url.PathEscape(id)+":batchUpdate", sanitized
	case KindGoogle + "/tasklists_create":
		path = "/tasks/v1/users/@me/lists"
	case KindGoogle + "/tasklists_update", KindGoogle + "/tasklists_delete":
		id, sanitized, err := extractResourceID(payload)
		if err != nil {
			return "", nil, err
		}
		path, payload = "/tasks/v1/users/@me/lists/"+url.PathEscape(id), sanitized
	case KindGoogle + "/tasks_create":
		listID, sanitized, err := extractResourceID(payload)
		if err != nil {
			return "", nil, err
		}
		options, body, err := extractStringOptions(sanitized, "parent", "previous")
		if err != nil {
			return "", nil, err
		}
		path, payload = withQuery("/tasks/v1/lists/"+url.PathEscape(listID)+"/tasks", options), body
	case KindGoogle + "/tasks_update", KindGoogle + "/tasks_delete":
		listID, taskID, sanitized, err := extractGoogleCompositeID(payload, "task-list ID and task ID")
		if err != nil {
			return "", nil, err
		}
		path, payload = "/tasks/v1/lists/"+url.PathEscape(listID)+"/tasks/"+url.PathEscape(taskID), sanitized
	case KindGoogle + "/tasks_move":
		listID, taskID, sanitized, err := extractGoogleCompositeID(payload, "task-list ID and task ID")
		if err != nil {
			return "", nil, err
		}
		options, _, err := extractStringOptions(sanitized, "parent", "previous", "destination_tasklist")
		if err != nil {
			return "", nil, err
		}
		if destination := options.Get("destination_tasklist"); destination != "" {
			options.Del("destination_tasklist")
			options.Set("destinationTasklist", destination)
		}
		path, payload = withQuery("/tasks/v1/lists/"+url.PathEscape(listID)+"/tasks/"+url.PathEscape(taskID)+"/move", options), []byte(`{}`)
	case KindGoogle + "/calendars_create":
		path = "/calendar/v3/calendars"
	case KindGoogle + "/calendars_update", KindGoogle + "/calendars_delete":
		id, sanitized, err := extractResourceID(payload)
		if err != nil {
			return "", nil, err
		}
		path, payload = "/calendar/v3/calendars/"+url.PathEscape(id), sanitized
	case KindGoogle + "/calendars_subscribe":
		id, _, err := extractResourceID(payload)
		if err != nil {
			return "", nil, err
		}
		path, payload = "/calendar/v3/users/me/calendarList", []byte(`{"id":`+mustJSONString(id)+`}`)
	case KindGoogle + "/calendars_unsubscribe":
		id, _, err := extractResourceID(payload)
		if err != nil {
			return "", nil, err
		}
		path, payload = "/calendar/v3/users/me/calendarList/"+url.PathEscape(id), []byte(`{}`)
	case KindGoogle + "/calendars_list_update":
		id, sanitized, err := extractResourceID(payload)
		if err != nil {
			return "", nil, err
		}
		path, payload = "/calendar/v3/users/me/calendarList/"+url.PathEscape(id), sanitized
	case KindGoogle + "/events_create":
		calendarID, sanitized, err := extractResourceID(payload)
		if err != nil {
			return "", nil, err
		}
		options, body, err := extractEventOptions(sanitized)
		if err != nil {
			return "", nil, err
		}
		path, payload = withQuery("/calendar/v3/calendars/"+url.PathEscape(calendarID)+"/events", options), body
	case KindGoogle + "/events_update", KindGoogle + "/events_delete":
		calendarID, eventID, sanitized, err := extractGoogleCompositeID(payload, "calendar ID and event ID")
		if err != nil {
			return "", nil, err
		}
		options, body, err := extractEventOptions(sanitized)
		if err != nil {
			return "", nil, err
		}
		path, payload = withQuery("/calendar/v3/calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), options), body
	case KindGoogle + "/events_move":
		calendarID, eventID, sanitized, err := extractGoogleCompositeID(payload, "calendar ID and event ID")
		if err != nil {
			return "", nil, err
		}
		options, _, err := extractStringOptions(sanitized, "destination_calendar")
		if err != nil || options.Get("destination_calendar") == "" {
			return "", nil, errors.New("Google event move requires destination_calendar")
		}
		query := url.Values{"destination": {options.Get("destination_calendar")}}
		path, payload = withQuery("/calendar/v3/calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID)+"/move", query), []byte(`{}`)
	case KindGoogle + "/events_respond":
		calendarID, eventID, sanitized, err := extractGoogleCompositeID(payload, "calendar ID and event ID")
		if err != nil {
			return "", nil, err
		}
		options, body, err := extractEventResponse(sanitized)
		if err != nil {
			return "", nil, err
		}
		path, payload = withQuery("/calendar/v3/calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), options), body
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

func serviceActionMethod(descriptor Descriptor, operation string) string {
	switch descriptor.Kind + "/" + operation {
	case KindNotion + "/page_update", KindGoogle + "/tasklists_update", KindGoogle + "/tasks_update", KindGoogle + "/calendars_update", KindGoogle + "/calendars_list_update", KindGoogle + "/events_update", KindGoogle + "/events_respond":
		return http.MethodPatch
	case KindGoogle + "/tasklists_delete", KindGoogle + "/tasks_delete", KindGoogle + "/calendars_delete", KindGoogle + "/calendars_unsubscribe", KindGoogle + "/events_delete":
		return http.MethodDelete
	case KindGoogle + "/tasks_move", KindGoogle + "/events_move":
		return http.MethodPost
	case KindGoogle + "/sheets_values_update":
		return http.MethodPut
	default:
		return http.MethodPost
	}
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

func extractGoogleCompositeID(payload []byte, label string) (string, string, []byte, error) {
	id, sanitized, err := extractResourceID(payload)
	if err != nil {
		return "", "", nil, err
	}
	first, second, err := splitGoogleID(id, label)
	if err != nil {
		return "", "", nil, err
	}
	return first, second, sanitized, nil
}

func extractStringOptions(payload []byte, names ...string) (url.Values, []byte, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil || object == nil {
		return nil, nil, errors.New("Google action payload is invalid")
	}
	options := url.Values{}
	for _, name := range names {
		raw, found := object[name]
		if !found {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil || strings.TrimSpace(value) != value || len(value) > 2048 {
			return nil, nil, fmt.Errorf("Google action option %s is invalid", name)
		}
		delete(object, name)
		if value != "" {
			options.Set(name, value)
		}
	}
	body, err := json.Marshal(object)
	if err != nil {
		return nil, nil, err
	}
	return options, body, nil
}

func extractEventOptions(payload []byte) (url.Values, []byte, error) {
	options, body, err := extractStringOptions(payload, "send_updates")
	if err != nil {
		return nil, nil, err
	}
	if value := options.Get("send_updates"); value != "" {
		if value != "all" && value != "externalOnly" && value != "none" {
			return nil, nil, errors.New("Google event send_updates must be all, externalOnly, or none")
		}
		options.Del("send_updates")
		options.Set("sendUpdates", value)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return nil, nil, errors.New("Google event payload is invalid")
	}
	if raw, found := object["supports_attachments"]; found {
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, nil, errors.New("Google event supports_attachments must be boolean")
		}
		delete(object, "supports_attachments")
		options.Set("supportsAttachments", fmt.Sprint(value))
	}
	if raw, found := object["conference_data_version"]; found {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil || value < 0 || value > 1 {
			return nil, nil, errors.New("Google event conference_data_version must be 0 or 1")
		}
		delete(object, "conference_data_version")
		options.Set("conferenceDataVersion", fmt.Sprint(value))
	}
	body, err = json.Marshal(object)
	if err != nil {
		return nil, nil, err
	}
	return options, body, nil
}

func extractEventResponse(payload []byte) (url.Values, []byte, error) {
	options, body, err := extractStringOptions(payload, "response_status", "comment", "send_updates")
	if err != nil {
		return nil, nil, err
	}
	status := options.Get("response_status")
	if status != "accepted" && status != "declined" && status != "tentative" && status != "needsAction" {
		return nil, nil, errors.New("Google event response_status is invalid")
	}
	if len(body) != 2 || string(body) != "{}" {
		return nil, nil, errors.New("Google event response only accepts response_status, comment, and send_updates")
	}
	attendee := map[string]any{"self": true, "responseStatus": status}
	if comment := options.Get("comment"); comment != "" {
		attendee["comment"] = comment
	}
	updates := options.Get("send_updates")
	if updates == "" {
		updates = "all"
	}
	if updates != "all" && updates != "externalOnly" && updates != "none" {
		return nil, nil, errors.New("Google event send_updates must be all, externalOnly, or none")
	}
	return url.Values{"sendUpdates": {updates}}, mustJSON(map[string]any{"attendees": []any{attendee}}), nil
}

func withQuery(path string, values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func mustJSONString(value string) string {
	payload, _ := json.Marshal(value)
	return string(payload)
}

func mustJSON(value any) []byte {
	payload, _ := json.Marshal(value)
	return payload
}

func splitGoogleID(value, label string) (string, string, error) {
	first, second, found := strings.Cut(strings.TrimSpace(value), "/")
	if !found || strings.TrimSpace(first) == "" || strings.TrimSpace(second) == "" || strings.Contains(second, "/") || len(first) > 1024 || len(second) > 1024 {
		return "", "", fmt.Errorf("Google operation requires %s as resource_id separated by one slash", label)
	}
	return first, second, nil
}

// googlePrecondition consumes the optional ETag from a typed Google action
// payload and turns it into an HTTP If-Match precondition. The ETag remains in
// the approval-bound payload, but is never sent as an unrecognised JSON field.
func googlePrecondition(descriptor Descriptor, payload []byte) ([]byte, string, error) {
	if descriptor.Kind != KindGoogle {
		return payload, "", nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil || object == nil {
		return nil, "", errors.New("Google action payload is invalid")
	}
	raw, found := object["etag"]
	if !found {
		return payload, "", nil
	}
	var etag string
	if err := json.Unmarshal(raw, &etag); err != nil || strings.TrimSpace(etag) != etag || etag == "" || len(etag) > 4096 || strings.ContainsAny(etag, "\x00\r\n") {
		return nil, "", errors.New("Google action etag is invalid")
	}
	delete(object, "etag")
	body, err := json.Marshal(object)
	if err != nil {
		return nil, "", err
	}
	return body, etag, nil
}

func isServiceKind(kind string) bool {
	return kind == KindSlack || kind == KindGoogle || kind == KindAtlassian || kind == KindNotion
}

func (r Runtime) requestJSON(ctx context.Context, descriptor Descriptor, operation, method, endpoint string, body []byte, uncertain bool) (Result, error) {
	return r.requestJSONWithPrecondition(ctx, descriptor, operation, method, endpoint, body, "", uncertain)
}

func (r Runtime) requestJSONWithPrecondition(ctx context.Context, descriptor Descriptor, operation, method, endpoint string, body []byte, ifMatch string, uncertain bool) (Result, error) {
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
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
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

// prepareGoogleBatch pins an ordered collection of independent Google calls
// into one canonical approval payload. The batch is intentionally executed as
// individual API requests rather than Google's multipart batch endpoint so a
// timeout can never be mistaken for an offline queue or an automatically
// replayable operation.
func prepareGoogleBatch(descriptor Descriptor, payload []byte) ([]googlePreparedStep, []byte, error) {
	var input struct {
		Actions []struct {
			Operation string          `json:"operation"`
			Payload   json.RawMessage `json:"payload"`
		} `json:"actions"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || len(input.Actions) == 0 || len(input.Actions) > 25 {
		return nil, nil, errors.New("Google batch requires 1 through 25 typed actions")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, nil, errors.New("Google batch payload contains trailing JSON")
	}
	type canonicalAction struct {
		Operation string          `json:"operation"`
		Payload   json.RawMessage `json:"payload"`
	}
	canonicalActions := make([]canonicalAction, 0, len(input.Actions))
	steps := make([]googlePreparedStep, 0, len(input.Actions))
	for _, item := range input.Actions {
		operationID := strings.TrimSpace(item.Operation)
		_, operation, err := (&Registry{byID: map[string]Descriptor{descriptor.ID: descriptor}}).Operation(descriptor.ID, operationID)
		if err != nil || operationID == "batch" || !action.RequiresFreshApproval(operation.Capability) {
			return nil, nil, fmt.Errorf("Google batch action %q is not an allowed Google mutation", operationID)
		}
		canonical, err := canonicalJSONObject(item.Payload)
		if err != nil {
			return nil, nil, fmt.Errorf("Google batch %s payload: %w", operationID, err)
		}
		requestPayload, ifMatch, err := googlePrecondition(descriptor, canonical)
		if err != nil {
			return nil, nil, fmt.Errorf("Google batch %s: %w", operationID, err)
		}
		target, transportBody, err := serviceAction(descriptor, operationID, requestPayload)
		if err != nil {
			return nil, nil, fmt.Errorf("Google batch %s: %w", operationID, err)
		}
		steps = append(steps, googlePreparedStep{operation: operationID, target: target, payload: transportBody, ifMatch: ifMatch})
		canonicalActions = append(canonicalActions, canonicalAction{Operation: operationID, Payload: canonical})
	}
	canonical, err := json.Marshal(struct {
		Actions []canonicalAction `json:"actions"`
	}{Actions: canonicalActions})
	if err != nil {
		return nil, nil, fmt.Errorf("encode canonical Google batch: %w", err)
	}
	return steps, canonical, nil
}

func canonicalJSONObject(raw json.RawMessage) ([]byte, error) {
	var object map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, errors.New("must be one JSON object")
	}
	result, err := json.Marshal(object)
	if err != nil || len(result) == 0 || len(result) > maxHTTPActionBytes {
		return nil, errors.New("is invalid or exceeds 12 KiB")
	}
	return result, nil
}

func (r Runtime) executeGoogleBatch(ctx context.Context, descriptor Descriptor, steps []googlePreparedStep) error {
	if len(steps) == 0 || len(steps) > 25 {
		return errors.New("prepared Google batch is invalid")
	}
	for _, step := range steps {
		result, err := r.requestJSONWithPrecondition(ctx, descriptor, step.operation, serviceActionMethod(descriptor, step.operation), step.target, step.payload, step.ifMatch, true)
		if err != nil {
			// In particular, do not replay an earlier request after an unknown
			// timeout or connection close. The caller gets the uncertain record.
			return err
		}
		r.mirrorGoogle(ctx, descriptor, step.operation, result.Data)
	}
	return nil
}

func (r Runtime) invokeGoogleLocalSearch(ctx context.Context, descriptor Descriptor, operation Operation, raw json.RawMessage) (Result, error) {
	var input struct {
		Query string   `json:"query"`
		Kinds []string `json:"kinds"`
		Limit int      `json:"limit"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.Query) == "" || len(input.Query) > 512 || input.Limit < 1 || input.Limit > 100 || len(input.Kinds) > 16 {
		return Result{}, errors.New("Google local search input is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Result{}, errors.New("Google local search input contains trailing JSON")
	}
	// The live health call is deliberately before opening/searching the mirror.
	// A disconnected credential must never reveal stale connected data.
	healthEndpoint, method, body, err := serviceRead(descriptor, "health", serviceInput{})
	if err != nil {
		return Result{}, err
	}
	health, err := r.requestJSON(ctx, descriptor, "health", method, healthEndpoint, body, false)
	if err != nil {
		return Result{}, err
	}
	store, err := r.openGoogleMirror(descriptor)
	if err != nil {
		return Result{}, err
	}
	defer store.Close()
	records, err := store.Search(ctx, input.Query, input.Kinds, input.Limit)
	if err != nil {
		return Result{}, err
	}
	contents, err := json.Marshal(struct {
		Records []googlework.Record `json:"records"`
	}{Records: records})
	if err != nil {
		return Result{}, fmt.Errorf("encode Google local search result: %w", err)
	}
	digest := sha256.Sum256(contents)
	health.Provenance.Operation = operation.ID
	health.Provenance.Bytes = int64(len(contents))
	health.Provenance.SHA256 = hex.EncodeToString(digest[:])
	health.Data = contents
	return health, nil
}

func (r Runtime) invokeGoogleQuickCapture(descriptor Descriptor, operation Operation, raw json.RawMessage) (Result, error) {
	var input struct {
		Text string `json:"text"`
		Kind string `json:"kind"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return Result{}, errors.New("Google quick capture input is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Result{}, errors.New("Google quick capture input contains trailing JSON")
	}
	kind := googlework.CaptureTask
	if input.Kind != "" {
		kind = googlework.CaptureKind(input.Kind)
	}
	capture, err := googlework.ParseQuickCapture(input.Text, kind, r.now())
	if err != nil {
		return Result{}, err
	}
	contents, err := json.Marshal(capture)
	if err != nil {
		return Result{}, fmt.Errorf("encode Google quick capture: %w", err)
	}
	digest := sha256.Sum256(contents)
	return Result{Data: contents, Provenance: Provenance{ConnectorID: descriptor.ID, Operation: operation.ID, Resource: descriptor.Resource, RetrievedAt: r.now(), Bytes: int64(len(contents)), SHA256: hex.EncodeToString(digest[:])}}, nil
}

func (r Runtime) invokeGoogleTaskMetadata(descriptor Descriptor, operation Operation, raw json.RawMessage) (Result, error) {
	var input struct {
		Notes           string `json:"notes"`
		Priority        string `json:"priority"`
		RecurrenceRRule string `json:"recurrence_rrule"`
		ReminderTime    string `json:"reminder_time"`
		ReminderZone    string `json:"reminder_zone"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return Result{}, errors.New("Google task metadata input is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Result{}, errors.New("Google task metadata input contains trailing JSON")
	}
	var output any
	if operation.ID == "task_metadata_encode" {
		notes, err := googlework.EncodeTaskNotes(input.Notes, googlework.TaskMetadata{Priority: input.Priority, RecurrenceRRule: input.RecurrenceRRule, ReminderTime: input.ReminderTime, ReminderZone: input.ReminderZone})
		if err != nil {
			return Result{}, err
		}
		output = struct {
			Notes string `json:"notes"`
		}{Notes: notes}
	} else {
		output = googlework.DecodeTaskNotes(input.Notes)
	}
	contents, err := json.Marshal(output)
	if err != nil {
		return Result{}, fmt.Errorf("encode Google task metadata: %w", err)
	}
	digest := sha256.Sum256(contents)
	return Result{Data: contents, Provenance: Provenance{ConnectorID: descriptor.ID, Operation: operation.ID, Resource: descriptor.Resource, RetrievedAt: r.now(), Bytes: int64(len(contents)), SHA256: hex.EncodeToString(digest[:])}}, nil
}

func (r Runtime) invokeGoogleReminders(ctx context.Context, descriptor Descriptor, operation Operation, raw json.RawMessage) (Result, error) {
	var input struct {
		WindowMinutes int `json:"window_minutes"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.WindowMinutes < 0 || input.WindowMinutes > 1440 {
		return Result{}, errors.New("Google reminder input is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Result{}, errors.New("Google reminder input contains trailing JSON")
	}
	if input.WindowMinutes == 0 {
		input.WindowMinutes = 10
	}
	// Never expose a cached reminder while disconnected. This is intentionally
	// the same live gate as local_search and runs before the mirror is opened.
	healthEndpoint, method, body, err := serviceRead(descriptor, "health", serviceInput{})
	if err != nil {
		return Result{}, err
	}
	health, err := r.requestJSON(ctx, descriptor, "health", method, healthEndpoint, body, false)
	if err != nil {
		return Result{}, err
	}
	store, err := r.openGoogleMirror(descriptor)
	if err != nil {
		return Result{}, err
	}
	defer store.Close()
	tasks, err := store.List(ctx, "task", 1000)
	if err != nil {
		return Result{}, err
	}
	events, err := store.List(ctx, "event", 1000)
	if err != nil {
		return Result{}, err
	}
	now := r.now()
	due := googlework.DueReminders(tasks, events, now.Add(-time.Duration(input.WindowMinutes)*time.Minute), now, time.Local)
	delivered := make([]googlework.Reminder, 0, len(due))
	for _, reminder := range due {
		claimed, err := store.ClaimReminder(ctx, reminder.Key, now)
		if err != nil {
			return Result{}, err
		}
		if claimed {
			delivered = append(delivered, reminder)
		}
	}
	contents, err := json.Marshal(struct {
		Reminders []googlework.Reminder `json:"reminders"`
	}{Reminders: delivered})
	if err != nil {
		return Result{}, fmt.Errorf("encode Google reminders: %w", err)
	}
	digest := sha256.Sum256(contents)
	health.Provenance.Operation = operation.ID
	health.Provenance.Bytes = int64(len(contents))
	health.Provenance.SHA256 = hex.EncodeToString(digest[:])
	health.Data = contents
	return health, nil
}

func (r Runtime) mirrorGoogle(ctx context.Context, descriptor Descriptor, operation string, contents json.RawMessage) {
	if descriptor.Kind != KindGoogle || strings.TrimSpace(r.StateDir) == "" {
		return
	}
	store, err := r.openGoogleMirror(descriptor)
	if err != nil {
		return
	}
	defer store.Close()
	// A mirror failure must not turn a completed live action into a retryable
	// error. Google has already acknowledged the operation.
	_ = store.Mirror(ctx, operation, contents, r.now())
}

func (r Runtime) openGoogleMirror(descriptor Descriptor) (*googlework.Store, error) {
	if descriptor.Kind != KindGoogle || strings.TrimSpace(r.StateDir) == "" {
		return nil, errors.New("Google Work mirror is unavailable without Gator state")
	}
	return googlework.Open(r.StateDir, descriptor.ID)
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
		flow := auth.BrowserFlow{ClientID: descriptor.OAuthClientID, ClientSecret: credential.OAuthClientSecret, TokenURL: descriptor.OAuthTokenURL, AllowMissingExpiry: true, RequireBearerToken: true, HTTPClient: r.HTTPClient}
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

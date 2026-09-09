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
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
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
	if err := decodeEmptyInput(input); err != nil {
		return Result{}, err
	}
	switch {
	case descriptor.Kind == KindHTTPJSON && operation.ID == "fetch":
		return r.fetchHTTPJSON(ctx, descriptor)
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
	if descriptor.Kind != KindHTTPWebhook || operation.ID != "publish" {
		return PreparedAction{}, fmt.Errorf("connector operation %s/%s has no action runtime", descriptor.ID, operation.ID)
	}
	payload, err := decodeActionInput(input)
	if err != nil {
		return PreparedAction{}, err
	}
	digest := sha256.Sum256(append([]byte(descriptor.ID+"\x00"+operation.ID+"\x00"), payload...))
	proposal, err := action.NewProposal(
		"action-"+hex.EncodeToString(digest[:8]), operation.Capability,
		descriptor.ID, operation.ID, descriptor.Resource, string(payload), payload,
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
	rebuilt, err := r.PrepareAction(descriptor.ID, operation.ID, actionInput(prepared.payload))
	if err != nil {
		return err
	}
	if rebuilt.Proposal != prepared.Proposal {
		return errors.New("prepared connector action payload does not match its proposal")
	}
	return r.publishHTTPJSON(ctx, descriptor, prepared.payload)
}

func (r Runtime) fetchHTTPJSON(ctx context.Context, descriptor Descriptor) (Result, error) {
	if err := descriptor.Validate(); err != nil {
		return Result{}, err
	}
	token, err := r.bearerToken(descriptor)
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
	token, err := r.bearerToken(descriptor)
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

func (r Runtime) bearerToken(descriptor Descriptor) (string, error) {
	if descriptor.Authentication != AuthBearer {
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
		return "", fmt.Errorf("connector %q credential has expired", descriptor.ID)
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

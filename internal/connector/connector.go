// Package connector defines Gator's user-facing connected-source model. A
// transport such as HTTP or MCP is an implementation detail behind stable,
// capability-classified operations.
package connector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
)

const (
	DescriptorVersion = 1
	KindHTTPJSON      = "http_json"
	KindHTTPWebhook   = "http_webhook"
	KindSlack         = "slack"
	KindGoogle        = "google_workspace"
	KindAtlassian     = "atlassian"
	KindNotion        = "notion"
	AuthNone          = "none"
	AuthBearer        = "bearer"
	maxConnectors     = 128
)

var idPattern = regexp.MustCompile(`\A[a-z][a-z0-9-]{0,63}\z`)

// Descriptor is non-secret, user-owned connector configuration. Resource is
// exact and immutable for one credential identity; URLs supplied by source
// data or model output never redirect it.
type Descriptor struct {
	Version        int    `json:"version"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Resource       string `json:"resource"`
	Authentication string `json:"authentication"`
}

// Operation is a connector action surfaced to the product and model. Schemas
// are bounded exact JSON Schema objects owned by Gator, not a remote server.
type Operation struct {
	ID           string            `json:"id"`
	Description  string            `json:"description"`
	Capability   action.Capability `json:"capability"`
	InputSchema  json.RawMessage   `json:"input_schema"`
	OutputSchema json.RawMessage   `json:"output_schema"`
}

// Provenance binds returned untrusted data to the exact configured source.
type Provenance struct {
	ConnectorID string    `json:"connector_id"`
	Operation   string    `json:"operation"`
	Resource    string    `json:"resource"`
	RetrievedAt time.Time `json:"retrieved_at"`
	Bytes       int64     `json:"bytes"`
	SHA256      string    `json:"sha256"`
}

// Result keeps connector data explicitly untrusted and separately attributed.
type Result struct {
	Data       json.RawMessage `json:"data"`
	Provenance Provenance      `json:"provenance"`
}

// Validate checks the safe provenance envelope without contacting its source.
func (p Provenance) Validate() error {
	if !idPattern.MatchString(p.ConnectorID) || !idPattern.MatchString(strings.ReplaceAll(p.Operation, "_", "-")) {
		return errors.New("connector provenance identity is invalid")
	}
	resource, err := url.Parse(p.Resource)
	if err != nil || resource.Scheme == "" || resource.Host == "" || resource.User != nil || resource.Fragment != "" || resource.String() != p.Resource {
		return errors.New("connector provenance resource is invalid")
	}
	if p.RetrievedAt.IsZero() || p.Bytes < 0 || len(p.SHA256) != sha256.Size*2 {
		return errors.New("connector provenance evidence is invalid")
	}
	if _, err := hex.DecodeString(p.SHA256); err != nil {
		return errors.New("connector provenance digest is invalid")
	}
	return nil
}

func (d Descriptor) Validate() error {
	if d.Version != DescriptorVersion || !idPattern.MatchString(d.ID) {
		return errors.New("connector version or ID is invalid")
	}
	if strings.TrimSpace(d.Name) != d.Name || d.Name == "" || len(d.Name) > 128 || strings.ContainsAny(d.Name, "\x00\r\n") {
		return errors.New("connector display name is invalid")
	}
	if d.Kind != KindHTTPJSON && d.Kind != KindHTTPWebhook && d.Kind != KindSlack && d.Kind != KindGoogle && d.Kind != KindAtlassian && d.Kind != KindNotion {
		return fmt.Errorf("unsupported connector kind %q", d.Kind)
	}
	resource, err := url.Parse(d.Resource)
	if err != nil || resource.Scheme == "" || resource.Host == "" || resource.User != nil || resource.Fragment != "" || len(d.Resource) > 2048 {
		return errors.New("connector resource must be an absolute URL without credentials or fragment")
	}
	if resource.String() != d.Resource {
		return errors.New("connector resource URL must be canonical")
	}
	if resource.Scheme != "https" && !(resource.Scheme == "http" && isLoopbackHost(resource.Hostname())) {
		return errors.New("connector resource requires HTTPS except on loopback")
	}
	if d.Authentication != AuthNone && d.Authentication != AuthBearer {
		return fmt.Errorf("unsupported connector authentication %q", d.Authentication)
	}
	return nil
}

// CredentialRef derives a resource-bound key for Gator's private auth store.
// Changing a connector URL therefore cannot silently redirect an old token.
func (d Descriptor) CredentialRef() string {
	digest := sha256.Sum256([]byte(d.Resource))
	return "connector-" + d.ID + "-" + hex.EncodeToString(digest[:8])
}

// Operations returns independent, Gator-owned operation metadata.
func (d Descriptor) Operations() []Operation {
	switch d.Kind {
	case KindHTTPJSON:
		return []Operation{{
			ID:          "fetch",
			Description: "Fetch the exact configured JSON resource as untrusted connected source data.",
			Capability:  action.ConnectedRead,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			OutputSchema: json.RawMessage(
				`{"type":"object","required":["data","provenance"],"properties":{"data":{},"provenance":{"type":"object"}}}`,
			),
		}}
	case KindHTTPWebhook:
		return []Operation{{
			ID:          "publish",
			Description: "Prepare JSON for the exact configured endpoint. Draft mode records a proposal; act mode still requires fresh approval before POSTing.",
			Capability:  action.Publish,
			InputSchema: json.RawMessage(`{"type":"object","required":["payload"],"properties":{"payload":{"type":"object"}},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(
				`{"type":"object","required":["proposal","status"],"properties":{"proposal":{"type":"object"},"status":{"type":"string"}}}`,
			),
		}}
	case KindSlack:
		return serviceOperations([]serviceOperation{
			{"whoami", "Verify the Slack app identity and granted token.", action.ConnectedRead},
			{"search", "Search Slack messages available to the configured app.", action.ConnectedRead},
			{"history", "Read one Slack channel's message history.", action.ConnectedRead},
			{"post_message", "Prepare a Slack message for exact approval before posting.", action.Publish},
			{"update_message", "Prepare an update to one Slack message for exact approval.", action.ConnectedMutate},
		})
	case KindGoogle:
		return serviceOperations([]serviceOperation{
			{"drive_search", "Search files visible through Google Drive.", action.ConnectedRead},
			{"drive_get", "Read one Google Drive file or exported document.", action.ConnectedRead},
			{"docs_get", "Read one Google Doc.", action.ConnectedRead},
			{"sheets_get", "Read one Google Sheet.", action.ConnectedRead},
			{"docs_create", "Prepare creation of a Google Doc for exact approval.", action.ConnectedMutate},
			{"sheets_create", "Prepare creation of a Google Sheet for exact approval.", action.ConnectedMutate},
		})
	case KindAtlassian:
		return serviceOperations([]serviceOperation{
			{"jira_search", "Search Jira issues with JQL.", action.ConnectedRead},
			{"jira_get", "Read one Jira issue.", action.ConnectedRead},
			{"confluence_search", "Search Confluence content with CQL.", action.ConnectedRead},
			{"jira_create", "Prepare creation of a Jira issue for exact approval.", action.ConnectedMutate},
			{"jira_comment", "Prepare a Jira comment for exact approval.", action.Publish},
			{"confluence_create", "Prepare creation of a Confluence page for exact approval.", action.ConnectedMutate},
		})
	case KindNotion:
		return serviceOperations([]serviceOperation{
			{"search", "Search pages and databases shared with the Notion integration.", action.ConnectedRead},
			{"page_get", "Read one Notion page.", action.ConnectedRead},
			{"block_children", "Read the child blocks of a Notion block.", action.ConnectedRead},
			{"page_create", "Prepare creation of a Notion page for exact approval.", action.ConnectedMutate},
			{"page_update", "Prepare an update to a Notion page for exact approval.", action.ConnectedMutate},
		})
	default:
		return nil
	}
}

type serviceOperation struct {
	id          string
	description string
	capability  action.Capability
}

func serviceOperations(specifications []serviceOperation) []Operation {
	operations := make([]Operation, 0, len(specifications))
	for _, specification := range specifications {
		input := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"query":{"type":"string"},"resource_id":{"type":"string"},"cursor":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}}}`)
		if action.RequiresFreshApproval(specification.capability) {
			input = json.RawMessage(`{"type":"object","required":["payload"],"properties":{"payload":{"type":"object"}},"additionalProperties":false}`)
		}
		operations = append(operations, Operation{ID: specification.id, Description: specification.description, Capability: specification.capability, InputSchema: input, OutputSchema: json.RawMessage(`{"type":"object"}`)})
	}
	return operations
}

// Registry is an immutable, deterministic view of configured connectors.
type Registry struct {
	descriptors []Descriptor
	byID        map[string]Descriptor
}

func NewRegistry(descriptors []Descriptor) (Registry, error) {
	if len(descriptors) > maxConnectors {
		return Registry{}, fmt.Errorf("at most %d connectors may be configured", maxConnectors)
	}
	registry := Registry{descriptors: append([]Descriptor(nil), descriptors...), byID: make(map[string]Descriptor, len(descriptors))}
	for _, descriptor := range registry.descriptors {
		if err := descriptor.Validate(); err != nil {
			return Registry{}, fmt.Errorf("connector %q: %w", descriptor.ID, err)
		}
		if _, duplicate := registry.byID[descriptor.ID]; duplicate {
			return Registry{}, fmt.Errorf("connector ID %q is repeated", descriptor.ID)
		}
		registry.byID[descriptor.ID] = descriptor
	}
	sort.Slice(registry.descriptors, func(left, right int) bool { return registry.descriptors[left].ID < registry.descriptors[right].ID })
	return registry, nil
}

func (r Registry) List() []Descriptor { return append([]Descriptor(nil), r.descriptors...) }

func (r Registry) Get(id string) (Descriptor, bool) {
	descriptor, found := r.byID[id]
	return descriptor, found
}

func (r Registry) Operation(connectorID, operationID string) (Descriptor, Operation, error) {
	descriptor, found := r.Get(connectorID)
	if !found {
		return Descriptor{}, Operation{}, fmt.Errorf("connector %q is not configured", connectorID)
	}
	for _, operation := range descriptor.Operations() {
		if operation.ID == operationID {
			return descriptor, operation, nil
		}
	}
	return Descriptor{}, Operation{}, fmt.Errorf("connector %q has no operation %q", connectorID, operationID)
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

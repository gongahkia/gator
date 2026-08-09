package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CommandSpec struct {
	ID   string   `json:"id"`
	Argv []string `json:"argv"`
}

type RoutingPolicy struct {
	DefaultProvider string            `json:"default_provider"`
	RoleProviders   map[string]string `json:"role_providers,omitempty"`
}

type ContextPolicy struct {
	MaxFiles    int  `json:"max_files"`
	MaxBytes    int  `json:"max_bytes"`
	IncludeDiff bool `json:"include_diff"`
}

type WorkflowPolicy struct {
	Topology string `json:"topology"`
}

type VerificationPolicy struct {
	Commands []CommandSpec `json:"commands"`
}

// Policy is deliberately limited to orchestration decisions. Security controls,
// credentials, provider sandbox settings, consent, and destructive Git behavior
// are not policy dimensions and cannot be optimized through this document.
type Policy struct {
	SchemaVersion int                `json:"schema_version"`
	Version       string             `json:"version"`
	Routing       RoutingPolicy      `json:"routing"`
	Context       ContextPolicy      `json:"context"`
	Workflow      WorkflowPolicy     `json:"workflow"`
	Verification  VerificationPolicy `json:"verification"`
}

func DefaultPolicy() Policy {
	policy := Policy{
		SchemaVersion: SchemaVersion,
		Routing:       RoutingPolicy{DefaultProvider: "codex", RoleProviders: map[string]string{}},
		Context:       ContextPolicy{MaxFiles: 12, MaxBytes: 131072, IncludeDiff: true},
		Workflow:      WorkflowPolicy{Topology: "direct_writer"},
		Verification:  VerificationPolicy{Commands: []CommandSpec{}},
	}
	_ = policy.Validate()
	return policy
}

func (p *Policy) Validate() error {
	if p.SchemaVersion != SchemaVersion {
		return fmt.Errorf("policy schema version %d is unsupported", p.SchemaVersion)
	}
	if !identifier(p.Routing.DefaultProvider) {
		return fmt.Errorf("policy default provider must be a lowercase identifier")
	}
	if p.Routing.RoleProviders == nil {
		p.Routing.RoleProviders = map[string]string{}
	}
	for role, provider := range p.Routing.RoleProviders {
		if !identifier(role) || !identifier(provider) {
			return fmt.Errorf("policy role providers must use lowercase identifiers")
		}
	}
	if p.Context.MaxFiles < 0 || p.Context.MaxFiles > 256 {
		return fmt.Errorf("policy context max_files must be between 0 and 256")
	}
	if p.Context.MaxBytes < 0 || p.Context.MaxBytes > 4*1024*1024 {
		return fmt.Errorf("policy context max_bytes must be between 0 and 4194304")
	}
	if p.Workflow.Topology != "direct_writer" && p.Workflow.Topology != "research_writer_reviewer" {
		return fmt.Errorf("policy workflow topology is unsupported")
	}
	seen := map[string]bool{}
	for _, command := range p.Verification.Commands {
		if !identifier(command.ID) || seen[command.ID] {
			return fmt.Errorf("verification commands require unique lowercase ids")
		}
		seen[command.ID] = true
		if len(command.Argv) == 0 {
			return fmt.Errorf("verification command %s requires argv", command.ID)
		}
		for _, value := range command.Argv {
			if value == "" || strings.ContainsRune(value, '\x00') {
				return fmt.Errorf("verification command %s has an invalid argv value", command.ID)
			}
		}
	}
	p.Version = p.fingerprint()
	return nil
}

func (p Policy) fingerprint() string {
	p.Version = ""
	encoded, err := json.Marshal(p)
	if err != nil {
		panic(fmt.Sprintf("marshal validated policy: %v", err))
	}
	digest := sha256.Sum256(encoded)
	return "policy-" + hex.EncodeToString(digest[:12])
}

func identifier(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if index == 0 && !(character >= 'a' && character <= 'z') {
			return false
		}
		if index > 0 && !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-') {
			return false
		}
	}
	return true
}

func PolicyPath(root string) string {
	return filepath.Join(root, StandaloneStatePath, "policy.json")
}

func LoadPolicy(root string) (Policy, PolicyRef, error) {
	path := PolicyPath(root)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		policy := DefaultPolicy()
		return policy, PolicyRef{SchemaVersion: policy.SchemaVersion, Version: policy.Version, Source: "default"}, nil
	}
	if err != nil {
		return Policy{}, PolicyRef{}, fmt.Errorf("read policy: %w", err)
	}
	policy, err := decodePolicy(data)
	if err != nil {
		return Policy{}, PolicyRef{}, fmt.Errorf("decode policy: %w", err)
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, PolicyRef{}, err
	}
	return policy, PolicyRef{SchemaVersion: policy.SchemaVersion, Version: policy.Version, Source: path}, nil
}

func decodePolicy(data []byte) (Policy, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, err
	}
	if decoder.More() {
		return Policy{}, fmt.Errorf("policy contains trailing JSON values")
	}
	return policy, nil
}

func SavePolicy(root string, policy Policy) (PolicyRef, error) {
	if err := policy.Validate(); err != nil {
		return PolicyRef{}, err
	}
	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return PolicyRef{}, fmt.Errorf("encode policy: %w", err)
	}
	path := PolicyPath(root)
	if err := atomicWrite(path, data, 0o600); err != nil {
		return PolicyRef{}, err
	}
	return PolicyRef{SchemaVersion: policy.SchemaVersion, Version: policy.Version, Source: path}, nil
}

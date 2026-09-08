// Package artifact owns the durable contract and evidence for Gator Work
// deliverables. It deliberately has no model- or provider-specific behavior.
package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"path"
	"strings"

	"github.com/gongahkia/gator/internal/action"
)

const (
	ContractVersion = 1
	ManifestVersion = 1

	DefaultMaxArtifactBytes int64 = 32 * 1024 * 1024
	DefaultMaxTotalBytes    int64 = 128 * 1024 * 1024
	maximumArtifactBytes    int64 = 256 * 1024 * 1024
	maximumTotalBytes       int64 = 1024 * 1024 * 1024
	maxRequirements               = 64
	maxValidations                = 16
)

// Contract describes the deterministic evidence required before a work run
// may report success. It is developer-owned input and must not be weakened by
// model output or repository content.
type Contract struct {
	Version          int                `json:"version"`
	Artifacts        []Requirement      `json:"artifacts"`
	MaxArtifactBytes int64              `json:"max_artifact_bytes,omitempty"`
	MaxTotalBytes    int64              `json:"max_total_bytes,omitempty"`
	ExternalActions  action.Disposition `json:"external_actions"`
}

// Requirement defines one required output-relative artifact and the
// validations that trusted Gator code must perform on it.
type Requirement struct {
	Path        string       `json:"path"`
	MediaTypes  []string     `json:"media_types,omitempty"`
	Validations []Validation `json:"validations,omitempty"`
}

// ValidationKind names one deterministic built-in artifact validator.
type ValidationKind string

const (
	ArtifactExists ValidationKind = "artifact_exists"
	NonEmpty       ValidationKind = "non_empty"
	UTF8           ValidationKind = "utf8"
	JSON           ValidationKind = "json"
	CSV            ValidationKind = "csv"
	Contains       ValidationKind = "contains"
	MediaType      ValidationKind = "media_type"
)

var validationKinds = map[ValidationKind]struct{}{
	ArtifactExists: {}, NonEmpty: {}, UTF8: {}, JSON: {}, CSV: {}, Contains: {}, MediaType: {},
}

// Validation configures one deterministic check. Value is accepted only by
// validators that require an argument, currently Contains.
type Validation struct {
	Kind  ValidationKind `json:"kind"`
	Value string         `json:"value,omitempty"`
}

// DefaultContract creates the smallest useful draft contract for paths. Each
// artifact must exist and be non-empty. Callers may add more validations
// before validating or executing the contract.
func DefaultContract(paths ...string) Contract {
	requirements := make([]Requirement, 0, len(paths))
	for _, artifactPath := range paths {
		requirements = append(requirements, Requirement{
			Path: artifactPath,
			Validations: []Validation{
				{Kind: ArtifactExists},
				{Kind: NonEmpty},
			},
		})
	}
	return Contract{
		Version:          ContractVersion,
		Artifacts:        requirements,
		MaxArtifactBytes: DefaultMaxArtifactBytes,
		MaxTotalBytes:    DefaultMaxTotalBytes,
		ExternalActions:  action.Forbid,
	}
}

// Normalize fills optional limits and policy with conservative defaults and
// returns independent slices safe for a caller to modify.
func (c Contract) Normalize() Contract {
	if c.MaxArtifactBytes == 0 {
		c.MaxArtifactBytes = DefaultMaxArtifactBytes
	}
	if c.MaxTotalBytes == 0 {
		c.MaxTotalBytes = DefaultMaxTotalBytes
	}
	if c.ExternalActions == "" {
		c.ExternalActions = action.Forbid
	}
	c.Artifacts = cloneRequirements(c.Artifacts)
	return c
}

// Validate rejects ambiguous paths, unsupported checks, and unbounded output
// policies before a work directory is created.
func (c Contract) Validate() error {
	c = c.Normalize()
	if c.Version != ContractVersion {
		return fmt.Errorf("unsupported artifact contract version %d", c.Version)
	}
	if len(c.Artifacts) == 0 || len(c.Artifacts) > maxRequirements {
		return fmt.Errorf("artifact contract requires 1-%d artifacts", maxRequirements)
	}
	if c.MaxArtifactBytes < 1 || c.MaxArtifactBytes > maximumArtifactBytes {
		return fmt.Errorf("artifact byte limit must be between 1 and %d", maximumArtifactBytes)
	}
	if c.MaxTotalBytes < c.MaxArtifactBytes || c.MaxTotalBytes > maximumTotalBytes {
		return fmt.Errorf("total artifact byte limit must be between the per-artifact limit and %d", maximumTotalBytes)
	}
	if err := c.ExternalActions.Validate(); err != nil {
		return err
	}

	seenPaths := make(map[string]struct{}, len(c.Artifacts))
	for index, requirement := range c.Artifacts {
		if err := validateArtifactPath(requirement.Path); err != nil {
			return fmt.Errorf("artifact requirement %d: %w", index+1, err)
		}
		if _, duplicate := seenPaths[requirement.Path]; duplicate {
			return fmt.Errorf("artifact path %q is required more than once", requirement.Path)
		}
		seenPaths[requirement.Path] = struct{}{}
		if err := validateMediaTypes(requirement.MediaTypes); err != nil {
			return fmt.Errorf("artifact %q: %w", requirement.Path, err)
		}
		if len(requirement.Validations) > maxValidations {
			return fmt.Errorf("artifact %q has more than %d validations", requirement.Path, maxValidations)
		}
		seenValidations := make(map[string]struct{}, len(requirement.Validations))
		for _, validation := range requirement.Validations {
			if err := validateValidation(validation); err != nil {
				return fmt.Errorf("artifact %q: %w", requirement.Path, err)
			}
			key := string(validation.Kind) + "\x00" + validation.Value
			if _, duplicate := seenValidations[key]; duplicate {
				return fmt.Errorf("artifact %q repeats validation %q", requirement.Path, validation.Kind)
			}
			seenValidations[key] = struct{}{}
		}
	}
	return nil
}

// Digest returns a stable digest of the normalized validated contract. It can
// be recorded in a manifest without trusting a model-supplied summary.
func (c Contract) Digest() (string, error) {
	c = c.Normalize()
	if err := c.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode artifact contract: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func validateArtifactPath(value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\\\x00\r\n") {
		return errors.New("artifact path is missing or invalid")
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return fmt.Errorf("artifact path %q must be a clean output-relative path", value)
	}
	return nil
}

// ValidateOutputPath checks that a model- or connector-supplied artifact path
// is portable, normalized, and relative to the isolated output root.
func ValidateOutputPath(value string) error {
	return validateArtifactPath(value)
}

func validateMediaTypes(values []string) error {
	if len(values) > 8 {
		return errors.New("artifact has more than 8 allowed media types")
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != value || strings.ToLower(value) != value || strings.Contains(value, "*") {
			return fmt.Errorf("media type %q must be a normalized exact type", value)
		}
		mediaType, parameters, err := mime.ParseMediaType(value)
		if err != nil || mediaType != value || len(parameters) != 0 {
			return fmt.Errorf("media type %q is invalid", value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("media type %q is repeated", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateValidation(validation Validation) error {
	if _, known := validationKinds[validation.Kind]; !known {
		return fmt.Errorf("unknown artifact validation %q", validation.Kind)
	}
	if validation.Kind == Contains {
		if validation.Value == "" || len(validation.Value) > 4*1024 || strings.ContainsRune(validation.Value, 0) {
			return errors.New("contains validation requires a bounded non-empty value")
		}
		return nil
	}
	if validation.Kind == MediaType {
		return errors.New("media_type validation is derived from the artifact's allowed media types")
	}
	if validation.Value != "" {
		return fmt.Errorf("artifact validation %q does not accept a value", validation.Kind)
	}
	return nil
}

func cloneRequirements(source []Requirement) []Requirement {
	if source == nil {
		return nil
	}
	result := make([]Requirement, len(source))
	for index, requirement := range source {
		result[index] = requirement
		result[index].MediaTypes = append([]string(nil), requirement.MediaTypes...)
		result[index].Validations = append([]Validation(nil), requirement.Validations...)
	}
	return result
}

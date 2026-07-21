package egress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/envelope"
)

const ProviderApprovalSchemaVersion = "paw.provider-approval/1"

var ErrInvalidProviderApproval = errors.New("invalid provider approval receipt")

type Finding = envelope.EgressFinding
type UnitManifest = envelope.EgressUnit
type Manifest = envelope.EgressManifest

type RedactionResult struct {
	Raw      *envelope.RawContext
	Findings []Finding
}

var detectors = []detector{
	{kind: "github_token", re: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`)},
	{kind: "openai_key", re: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`)},
	{kind: "aws_access_key", re: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{kind: "private_key", re: regexp.MustCompile(`-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----[\s\S]*?-----END (?:[A-Z ]+ )?PRIVATE KEY-----`)},
	{kind: "jwt", re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
}

type detector struct {
	kind string
	re   *regexp.Regexp
}

func Build(raw *envelope.RawContext) Manifest {
	if raw == nil {
		return Manifest{}
	}
	manifest := Manifest{Units: make([]UnitManifest, 0, len(raw.Units))}
	counts := map[string]int{}
	for _, unit := range raw.Units {
		sum := sha256.Sum256([]byte(unit.Text))
		manifest.Units = append(manifest.Units, UnitManifest{
			ID:     unit.ID,
			Kind:   unit.Kind,
			Path:   unit.Path,
			Bytes:  len(unit.Text),
			SHA256: hex.EncodeToString(sum[:]),
		})
		manifest.TotalBytes += len(unit.Text)
		for _, finding := range scan(unit.Text) {
			counts[finding.Kind] += finding.Count
		}
	}
	manifest.Findings = findingsFromCounts(counts)
	return manifest
}

func Redact(raw *envelope.RawContext) RedactionResult {
	if raw == nil {
		return RedactionResult{}
	}
	out := *raw
	out.Units = make([]envelope.RawUnit, len(raw.Units))
	out.TotalBytes = 0
	counts := map[string]int{}
	for i, unit := range raw.Units {
		text, findings := redact(unit.Text)
		unit.Text = text
		out.Units[i] = unit
		out.TotalBytes += len(text)
		for _, finding := range findings {
			counts[finding.Kind] += finding.Count
		}
	}
	return RedactionResult{Raw: &out, Findings: findingsFromCounts(counts)}
}

func Enforce(cfg config.EgressPolicy, manifest Manifest) error {
	if len(manifest.Units) > cfg.MaxFiles {
		return fmt.Errorf("egress units %d exceed policy limit %d", len(manifest.Units), cfg.MaxFiles)
	}
	if manifest.TotalBytes > cfg.MaxBytes {
		return fmt.Errorf("egress bytes %d exceed policy limit %d", manifest.TotalBytes, cfg.MaxBytes)
	}
	if cfg.BlockSecrets && len(manifest.Findings) > 0 {
		return fmt.Errorf("egress blocked: %d high-confidence secret kinds detected", len(manifest.Findings))
	}
	return nil
}

func Prepare(cfg config.EgressPolicy, raw *envelope.RawContext) (RedactionResult, Manifest, error) {
	manifest := Build(raw)
	if err := Enforce(cfg, manifest); err != nil {
		return RedactionResult{}, manifest, err
	}
	return Redact(raw), manifest, nil
}

func Approve(manifest Manifest, transport, baseURL string, now time.Time, autoApproved bool) (Manifest, error) {
	receipt, err := NewProviderApprovalReceipt(manifest, transport, baseURL, now, autoApproved)
	if err != nil {
		return Manifest{}, err
	}
	manifest.ProviderApproval = &receipt
	return manifest, nil
}

func NewProviderApprovalReceipt(manifest Manifest, transport, baseURL string, now time.Time, autoApproved bool) (envelope.ProviderApprovalReceipt, error) {
	if err := validateReceiptDestination(transport, baseURL); err != nil {
		return envelope.ProviderApprovalReceipt{}, err
	}
	if now.IsZero() {
		return envelope.ProviderApprovalReceipt{}, fmt.Errorf("%w: approved_at is required", ErrInvalidProviderApproval)
	}
	digest, err := manifestDigest(manifest)
	if err != nil {
		return envelope.ProviderApprovalReceipt{}, err
	}
	return envelope.ProviderApprovalReceipt{
		SchemaVersion:  ProviderApprovalSchemaVersion,
		Transport:      transport,
		BaseURL:        baseURL,
		ManifestSHA256: digest,
		ApprovedAt:     now.UTC(),
		AutoApproved:   autoApproved,
	}, nil
}

func VerifyProviderApproval(manifest Manifest, transport, baseURL string) error {
	if err := validateReceiptDestination(transport, baseURL); err != nil {
		return err
	}
	receipt := manifest.ProviderApproval
	if receipt == nil {
		return fmt.Errorf("%w: receipt is required", ErrInvalidProviderApproval)
	}
	if receipt.SchemaVersion != ProviderApprovalSchemaVersion {
		return fmt.Errorf("%w: unsupported schema %q", ErrInvalidProviderApproval, receipt.SchemaVersion)
	}
	if receipt.Transport != transport || receipt.BaseURL != baseURL {
		return fmt.Errorf("%w: provider destination does not match receipt", ErrInvalidProviderApproval)
	}
	if receipt.ApprovedAt.IsZero() {
		return fmt.Errorf("%w: approved_at is required", ErrInvalidProviderApproval)
	}
	digest, err := manifestDigest(manifest)
	if err != nil {
		return err
	}
	if receipt.ManifestSHA256 != digest {
		return fmt.Errorf("%w: manifest digest does not match receipt", ErrInvalidProviderApproval)
	}
	return nil
}

func validateReceiptDestination(transport, baseURL string) error {
	if transport == "" || transport != strings.TrimSpace(transport) {
		return fmt.Errorf("%w: transport is required", ErrInvalidProviderApproval)
	}
	if baseURL == "" || baseURL != strings.TrimSpace(baseURL) {
		return fmt.Errorf("%w: base_url is required", ErrInvalidProviderApproval)
	}
	return nil
}

func manifestDigest(manifest Manifest) (string, error) {
	manifest.ProviderApproval = nil
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("encode egress manifest: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func scan(text string) []Finding {
	counts := map[string]int{}
	for _, detector := range detectors {
		counts[detector.kind] = len(detector.re.FindAllStringIndex(text, -1))
	}
	return findingsFromCounts(counts)
}

func redact(text string) (string, []Finding) {
	counts := map[string]int{}
	for _, detector := range detectors {
		matches := detector.re.FindAllStringIndex(text, -1)
		counts[detector.kind] += len(matches)
		text = detector.re.ReplaceAllString(text, "[REDACTED:"+detector.kind+"]")
	}
	return text, findingsFromCounts(counts)
}

func findingsFromCounts(counts map[string]int) []Finding {
	findings := make([]Finding, 0, len(counts))
	for _, detector := range detectors {
		if count := counts[detector.kind]; count > 0 {
			findings = append(findings, Finding{Kind: detector.kind, Count: count})
		}
	}
	return findings
}

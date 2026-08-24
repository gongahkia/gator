package localmodel

import (
	"fmt"
	"strings"
)

const (
	// MemoryMultiplier deliberately reserves one model-sized allocation in
	// addition to the published pull size. It is Gator's conservative
	// admission guardrail, not an upstream model requirement or a benchmark.
	MemoryMultiplier uint64 = 2

	// DiskOverheadPercent reserves pull-time filesystem headroom beyond the
	// published package size.
	DiskOverheadPercent uint64 = 20
)

// Host is the non-secret, local machine evidence used to decide whether Gator
// may pull, select, or run a reviewed local model. Zero resource values mean
// that the corresponding measurement was unavailable.
type Host struct {
	OS                   string
	Architecture         string
	TotalMemoryBytes     uint64
	AvailableMemoryBytes uint64
	ModelDirectory       string
	ModelDirectorySource string
	AvailableDiskBytes   uint64
	MemoryError          string
	DiskError            string
}

// Eligibility is the auditable result of applying Gator's local-model
// admission guardrail to one model and one host.
type Eligibility struct {
	Allowed             bool
	Reason              string
	RequiredMemoryBytes uint64
	RequiredDiskBytes   uint64
	Advice              []string
}

// Assess applies Gator's intentionally conservative local-model guardrail.
// It blocks unsupported OS/architectures and resource shortfalls, but it does
// not claim to identify an Ollama-compatible GPU or predict generation speed.
func Assess(model Model, host Host) Eligibility {
	result := Eligibility{
		Allowed:             true,
		RequiredMemoryBytes: requiredMemoryBytes(model),
		RequiredDiskBytes:   requiredDiskBytes(model),
		Advice: []string{
			"Gator's RAM limit is a conservative admission guardrail, not an upstream performance guarantee.",
			"GPU compatibility and VRAM are selected by Ollama at load time and are not inferred by Gator.",
		},
	}
	if !supportedOS(host.OS) {
		return result.block(fmt.Sprintf("%s is not a supported local-model OS; Gator verifies Ollama hosts only on Linux, macOS, and Windows", host.OS))
	}
	if !supportedArchitecture(host.Architecture) {
		return result.block(fmt.Sprintf("%s is not a supported 64-bit local-model architecture", host.Architecture))
	}
	if model.DownloadBytes == 0 {
		return result.block("this catalog entry has no published package-size metadata")
	}
	if host.TotalMemoryBytes == 0 {
		message := "Gator could not inspect total system memory"
		if host.MemoryError != "" {
			message += ": " + host.MemoryError
		}
		return result.block(message)
	}
	if host.TotalMemoryBytes < result.RequiredMemoryBytes {
		return result.block(fmt.Sprintf("requires at least %s total RAM under Gator's guardrail; detected %s", FormatBytes(result.RequiredMemoryBytes), FormatBytes(host.TotalMemoryBytes)))
	}
	if host.AvailableMemoryBytes != 0 && host.AvailableMemoryBytes < result.RequiredMemoryBytes {
		return result.block(fmt.Sprintf("requires %s currently available RAM under Gator's guardrail; detected %s", FormatBytes(result.RequiredMemoryBytes), FormatBytes(host.AvailableMemoryBytes)))
	}
	if host.AvailableDiskBytes != 0 && host.AvailableDiskBytes < result.RequiredDiskBytes {
		return result.block(fmt.Sprintf("requires %s free in the Ollama model filesystem; detected %s", FormatBytes(result.RequiredDiskBytes), FormatBytes(host.AvailableDiskBytes)))
	}
	if host.AvailableMemoryBytes == 0 {
		result.Advice = append(result.Advice, "Current available RAM could not be measured; Gator checked physical RAM only.")
	}
	if host.AvailableDiskBytes == 0 {
		advice := "Ollama model filesystem capacity could not be measured"
		if host.DiskError != "" {
			advice += ": " + host.DiskError
		}
		result.Advice = append(result.Advice, advice+".")
	}
	return result
}

func (result Eligibility) block(reason string) Eligibility {
	result.Allowed = false
	result.Reason = reason
	return result
}

func requiredMemoryBytes(model Model) uint64 {
	if model.DownloadBytes > ^uint64(0)/MemoryMultiplier {
		return ^uint64(0)
	}
	return model.DownloadBytes * MemoryMultiplier
}

func requiredDiskBytes(model Model) uint64 {
	if model.DownloadBytes > ^uint64(0)/(100+DiskOverheadPercent) {
		return ^uint64(0)
	}
	return model.DownloadBytes * (100 + DiskOverheadPercent) / 100
}

func supportedOS(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "linux", "darwin", "windows":
		return true
	default:
		return false
	}
}

func supportedArchitecture(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "amd64", "arm64":
		return true
	default:
		return false
	}
}

// FormatBytes produces a stable, display-safe binary unit string.
func FormatBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	converted := float64(value)
	index := -1
	for converted >= unit && index+1 < len(units) {
		converted /= unit
		index++
	}
	return fmt.Sprintf("%.1f %s", converted, units[index])
}

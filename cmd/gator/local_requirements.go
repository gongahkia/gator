package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/gongahkia/gator/internal/localmodel"
)

// inspectLocalModelHost is replaceable by tests. Production always gathers
// fresh host facts so a model cannot be enabled while the machine is under
// memory pressure.
var inspectLocalModelHost = localmodel.DetectHost

func localModelEligibility(model localmodel.Model) localmodel.Eligibility {
	return localmodel.Assess(model, inspectLocalModelHost())
}

func requireLocalModelEligibility(model localmodel.Model) error {
	eligibility := localModelEligibility(model)
	if eligibility.Allowed {
		return nil
	}
	return fmt.Errorf("%s is disabled on this host: %s; run 'gator doctor' for the detected requirements", model.Name, eligibility.Reason)
}

func writeLocalModelDoctor(out io.Writer, host localmodel.Host) error {
	if _, err := fmt.Fprintf(out, "Local model host: %s/%s\n", host.OS, host.Architecture); err != nil {
		return err
	}
	if host.TotalMemoryBytes == 0 {
		message := "unavailable"
		if host.MemoryError != "" {
			message += " (" + host.MemoryError + ")"
		}
		if _, err := fmt.Fprintf(out, "Local model memory: %s\n", message); err != nil {
			return err
		}
	} else {
		memory := localmodel.FormatBytes(host.TotalMemoryBytes) + " total"
		if host.AvailableMemoryBytes != 0 {
			memory += ", " + localmodel.FormatBytes(host.AvailableMemoryBytes) + " available"
		} else if host.MemoryError != "" {
			memory += " (available memory unavailable: " + host.MemoryError + ")"
		}
		if _, err := fmt.Fprintf(out, "Local model memory: %s\n", memory); err != nil {
			return err
		}
	}
	storage := "unavailable"
	if host.ModelDirectory != "" {
		storage = host.ModelDirectory
		if host.ModelDirectorySource != "" {
			storage += " (" + host.ModelDirectorySource + ")"
		}
	}
	if host.AvailableDiskBytes != 0 {
		storage += ", " + localmodel.FormatBytes(host.AvailableDiskBytes) + " free"
	} else if host.DiskError != "" {
		storage += " (free space unavailable: " + host.DiskError + ")"
	}
	if _, err := fmt.Fprintf(out, "Ollama model storage: %s\n", storage); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Reviewed local-model eligibility (Gator guardrail):"); err != nil {
		return err
	}
	for _, model := range localmodel.Catalog() {
		eligibility := localmodel.Assess(model, host)
		state := "enabled"
		if !eligibility.Allowed {
			state = "disabled: " + eligibility.Reason
		}
		if _, err := fmt.Fprintf(out, "  %s: %s · needs %s RAM and %s free disk\n", model.ID, state, localmodel.FormatBytes(eligibility.RequiredMemoryBytes), localmodel.FormatBytes(eligibility.RequiredDiskBytes)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(out, "Local-model advice: Gator blocks reviewed models that fail this conservative RAM/disk/OS guardrail. Context length, parallel requests, and GPU/VRAM can increase actual requirements; use 'ollama ps' to inspect a loaded model's processor."); err != nil {
		return err
	}
	return nil
}

func localModelHostSummary(host localmodel.Host) (string, []string) {
	summary := host.OS + "/" + host.Architecture
	if host.TotalMemoryBytes != 0 {
		summary += " · " + localmodel.FormatBytes(host.TotalMemoryBytes) + " RAM"
		if host.AvailableMemoryBytes != 0 {
			summary += " · " + localmodel.FormatBytes(host.AvailableMemoryBytes) + " available"
		}
	}
	if host.AvailableDiskBytes != 0 {
		summary += " · " + localmodel.FormatBytes(host.AvailableDiskBytes) + " model storage free"
	}
	advice := make([]string, 0, 2)
	if host.MemoryError != "" {
		advice = append(advice, "Memory inspection: "+host.MemoryError)
	}
	if host.DiskError != "" {
		advice = append(advice, "Storage inspection: "+host.DiskError)
	}
	return summary, advice
}

func localModelRequirement(eligibility localmodel.Eligibility) string {
	return "needs " + localmodel.FormatBytes(eligibility.RequiredMemoryBytes) + " RAM / " + localmodel.FormatBytes(eligibility.RequiredDiskBytes) + " disk"
}

func localModelBlockedReason(model localmodel.Model) string {
	eligibility := localModelEligibility(model)
	return strings.TrimSpace(eligibility.Reason)
}

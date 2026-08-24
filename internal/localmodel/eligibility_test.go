package localmodel

import (
	"strings"
	"testing"
)

func TestAssessBlocksUnsupportedHostAndResourceShortfalls(t *testing.T) {
	model, found := Resolve("qwen2.5-coder-7b")
	if !found {
		t.Fatal("catalog model was not found")
	}
	tests := []struct {
		name string
		host Host
		want string
	}{
		{
			name: "unsupported OS",
			host: Host{OS: "plan9", Architecture: "amd64", TotalMemoryBytes: 64 << 30, AvailableMemoryBytes: 64 << 30, AvailableMemoryKnown: true, AvailableDiskBytes: 64 << 30, AvailableDiskKnown: true},
			want: "not a supported",
		},
		{
			name: "unsupported architecture",
			host: Host{OS: "linux", Architecture: "386", TotalMemoryBytes: 64 << 30, AvailableMemoryBytes: 64 << 30, AvailableMemoryKnown: true, AvailableDiskBytes: 64 << 30, AvailableDiskKnown: true},
			want: "architecture",
		},
		{
			name: "insufficient physical RAM",
			host: Host{OS: "linux", Architecture: "amd64", TotalMemoryBytes: 8 << 30, AvailableMemoryBytes: 8 << 30, AvailableMemoryKnown: true, AvailableDiskBytes: 64 << 30, AvailableDiskKnown: true},
			want: "total RAM",
		},
		{
			name: "insufficient available RAM",
			host: Host{OS: "linux", Architecture: "amd64", TotalMemoryBytes: 64 << 30, AvailableMemoryBytes: 8 << 30, AvailableMemoryKnown: true, AvailableDiskBytes: 64 << 30, AvailableDiskKnown: true},
			want: "currently available RAM",
		},
		{
			name: "no available RAM",
			host: Host{OS: "linux", Architecture: "amd64", TotalMemoryBytes: 64 << 30, AvailableMemoryKnown: true, AvailableDiskBytes: 64 << 30, AvailableDiskKnown: true},
			want: "currently available RAM",
		},
		{
			name: "insufficient disk",
			host: Host{OS: "linux", Architecture: "amd64", TotalMemoryBytes: 64 << 30, AvailableMemoryBytes: 64 << 30, AvailableMemoryKnown: true, AvailableDiskBytes: 1 << 30, AvailableDiskKnown: true},
			want: "free in the Ollama model filesystem",
		},
		{
			name: "no free disk",
			host: Host{OS: "linux", Architecture: "amd64", TotalMemoryBytes: 64 << 30, AvailableMemoryBytes: 64 << 30, AvailableMemoryKnown: true, AvailableDiskKnown: true},
			want: "free in the Ollama model filesystem",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Assess(model, test.host)
			if result.Allowed || !strings.Contains(result.Reason, test.want) {
				t.Fatalf("assessment = %#v, want blocked reason containing %q", result, test.want)
			}
		})
	}
}

func TestAssessAllowsAdequatelyResourcedHostAndExplainsGuardrail(t *testing.T) {
	model, found := Resolve("qwen3-coder-30b")
	if !found {
		t.Fatal("catalog model was not found")
	}
	result := Assess(model, Host{
		OS:                   "linux",
		Architecture:         "amd64",
		TotalMemoryBytes:     64 << 30,
		AvailableMemoryBytes: 64 << 30,
		AvailableMemoryKnown: true,
		AvailableDiskBytes:   64 << 30,
		AvailableDiskKnown:   true,
	})
	if !result.Allowed || result.RequiredMemoryBytes != 38_000_000_000 || result.RequiredDiskBytes != 22_800_000_000 {
		t.Fatalf("assessment = %#v", result)
	}
	if len(result.Advice) == 0 || !strings.Contains(result.Advice[0], "guardrail") {
		t.Fatalf("assessment advice = %#v", result.Advice)
	}
}

func TestFormatBytes(t *testing.T) {
	if got := FormatBytes(4_700_000_000); got != "4.4 GiB" {
		t.Fatalf("FormatBytes = %q", got)
	}
}

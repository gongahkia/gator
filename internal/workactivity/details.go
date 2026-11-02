package workactivity

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

const maxDetailBytes = 32 * 1024
const maxDetails = 12

// Detail is a finite public projection of observed native tool results. It has
// no free-form diagnostic, command argv, tool arguments, URL path or output.
type Detail struct {
	Kind        string `json:"kind"`
	Path        string `json:"path,omitempty"`
	Domain      string `json:"domain,omitempty"`
	EvidenceID  string `json:"evidence_id,omitempty"`
	CandidateID string `json:"candidate_id,omitempty"`
	Status      string `json:"status,omitempty"`
	ExitCode    *int   `json:"exit_code,omitempty"`
	TimedOut    bool   `json:"timed_out,omitempty"`
}

func displayPath(value string) bool {
	return value != "" && value != "." && len(value) <= 512 && utf8.ValidString(value) && strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) < 0 && path.Clean(value) == value && !strings.HasPrefix(value, "/") && value != ".." && !strings.HasPrefix(value, "../") && !strings.ContainsAny(value, "\\:\x00\r\n\t")
}
func displayID(value string) bool {
	if len(value) > 128 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func displayDomain(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if len(host) == 0 || len(host) > 253 {
		return ""
	}
	for _, r := range host {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-') {
			return ""
		}
	}
	return host
}
func (d Detail) Valid() bool {
	if !displayID(d.EvidenceID) || !displayID(d.CandidateID) {
		return false
	}
	switch d.Kind {
	case "searched", "fetched":
		return d.Path == "" && d.Domain != "" && displayDomain("https://"+d.Domain) == d.Domain && d.CandidateID == "" && d.ExitCode == nil && d.Status == "" && !d.TimedOut
	case "read_file", "read_code", "changed_file":
		return displayPath(d.Path) && d.Domain == "" && d.ExitCode == nil && d.Status == "" && !d.TimedOut
	case "command":
		return d.ExitCode != nil && d.Path == "" && d.Domain == "" && d.EvidenceID == "" && d.CandidateID == "" && d.Status == ""
	case "candidate":
		return d.CandidateID != "" && d.Path == "" && d.Domain == "" && d.EvidenceID == "" && d.ExitCode == nil && (d.Status == "verified" || d.Status == "failed" || d.Status == "rejected") && !d.TimedOut
	}
	return false
}
func toolDetails(name, content string) ([]Detail, bool) {
	if len(content) > 128*1024 {
		return nil, true
	}
	var envelope struct {
		OK     *bool           `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(content), &envelope) != nil {
		return nil, false
	}
	data := []byte(content)
	if envelope.OK != nil {
		if !*envelope.OK {
			return nil, false
		}
		data = envelope.Result
	}
	var value struct {
		Path             string   `json:"path"`
		Applied          bool     `json:"applied"`
		ChangedPaths     []string `json:"changed_paths"`
		PathsUnavailable bool     `json:"paths_unavailable"`
		Truncated        bool     `json:"truncated"`
		Results          []struct {
			URL string `json:"url"`
		} `json:"results"`
		Evidence struct {
			ID      string `json:"id"`
			Locator string `json:"locator"`
		} `json:"evidence"`
		Candidate struct {
			ID           string   `json:"id"`
			Status       string   `json:"status"`
			ChangedPaths []string `json:"changed_paths"`
		} `json:"candidate"`
		ExitCode *int `json:"exit_code"`
		TimedOut bool `json:"timed_out"`
	}
	if json.Unmarshal(data, &value) != nil {
		return nil, false
	}
	details := []Detail{}
	partial := value.Truncated || value.PathsUnavailable
	add := func(d Detail) {
		if !d.Valid() {
			return
		}
		for _, prior := range details {
			a, _ := json.Marshal(prior)
			b, _ := json.Marshal(d)
			if string(a) == string(b) {
				return
			}
		}
		if len(details) >= maxDetails {
			partial = true
			return
		}
		details = append(details, d)
	}
	switch name {
	case "web_search":
		for _, result := range value.Results {
			if domain := displayDomain(result.URL); domain != "" {
				add(Detail{Kind: "searched", Domain: domain})
			}
		}
	case "web_fetch":
		if domain := displayDomain(value.Evidence.Locator); domain != "" {
			add(Detail{Kind: "fetched", Domain: domain, EvidenceID: value.Evidence.ID})
		}
	case "read_file":
		add(Detail{Kind: "read_file", Path: value.Path})
	case "code_read_file":
		add(Detail{Kind: "read_code", Path: value.Path})
	case "apply_patch", "code_apply_patch":
		if value.Applied {
			for _, p := range value.ChangedPaths {
				add(Detail{Kind: "changed_file", Path: p})
			}
		}
	case "code_stage", "integrate_code", "complete_code":
		add(Detail{Kind: "candidate", CandidateID: value.Candidate.ID, Status: value.Candidate.Status})
		for _, p := range value.Candidate.ChangedPaths {
			add(Detail{Kind: "changed_file", Path: p, CandidateID: value.Candidate.ID})
		}
	case "run_command", "code_run_command":
		if value.ExitCode != nil {
			add(Detail{Kind: "command", ExitCode: value.ExitCode, TimedOut: value.TimedOut})
		}
	}
	return details, partial
}

// ValidDetails is shared by the retained sidecar reader. Unknown/new display
// data fails closed without damaging canonical conversation replay.
func (t *Trace) ValidDetails() bool {
	bytes := 0
	for _, entry := range t.Entries {
		if len(entry.Details) > maxDetails {
			return false
		}
		for _, detail := range entry.Details {
			if !detail.Valid() {
				return false
			}
		}
		if len(entry.Details) > 0 {
			data, _ := json.Marshal(entry.Details)
			bytes += len(data)
		}
		if bytes > maxDetailBytes {
			return false
		}
	}
	return true
}

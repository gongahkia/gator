package envelope

import "time"

type EgressFinding struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type EgressUnit struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type EgressManifest struct {
	Units            []EgressUnit             `json:"units"`
	TotalBytes       int                      `json:"total_bytes"`
	Findings         []EgressFinding          `json:"findings,omitempty"`
	ProviderApproval *ProviderApprovalReceipt `json:"provider_approval,omitempty"`
}

type ProviderApprovalReceipt struct {
	SchemaVersion  string    `json:"schema_version"`
	Transport      string    `json:"transport"`
	BaseURL        string    `json:"base_url"`
	ManifestSHA256 string    `json:"manifest_sha256"`
	ApprovedAt     time.Time `json:"approved_at"`
	AutoApproved   bool      `json:"auto_approved"`
}

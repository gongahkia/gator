package envelope

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
	Units      []EgressUnit    `json:"units"`
	TotalBytes int             `json:"total_bytes"`
	Findings   []EgressFinding `json:"findings,omitempty"`
}

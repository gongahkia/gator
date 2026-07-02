package envelope

type RawUnit struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Text      string `json:"text"`
}

type RawContext struct {
	Units      []RawUnit `json:"units"`
	TotalBytes int       `json:"total_bytes"`
}

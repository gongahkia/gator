package envelope

type DigestSpan struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Quote     string `json:"quote"`
}

type DigestItem struct {
	UnitID    string       `json:"unit_id"`
	Path      string       `json:"path"`
	Relevance int          `json:"relevance"`
	Spans     []DigestSpan `json:"spans"`
}

type ContextDigest struct {
	Summary string       `json:"summary"`
	Items   []DigestItem `json:"items"`
}

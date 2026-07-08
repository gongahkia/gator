package envelope

type Patch struct {
	UnifiedDiff string   `json:"unified_diff"`
	Files       []string `json:"files"`
	Note        string   `json:"note,omitempty"`
}

type VerifyResult struct {
	Passed        bool   `json:"passed"`
	ExitCode      int    `json:"exit_code"`
	Command       string `json:"command"`
	FailureDigest string `json:"failure_digest"`
	RawTailBytes  int    `json:"raw_tail_bytes"`
}

type Budget struct {
	MaxTurns                 int    `json:"max_turns"`
	Turn                     int    `json:"turn"`
	BrainInputTokens         int    `json:"brain_input_tokens"`
	BrainOutputTokens        int    `json:"brain_output_tokens"`
	BrainCacheCreationTokens int    `json:"brain_cache_creation_tokens,omitempty"`
	BrainCacheReadTokens     int    `json:"brain_cache_read_tokens,omitempty"`
	BrainTokenSource         string `json:"brain_token_source,omitempty"`
	DroneTokens              int    `json:"drone_tokens"`
	DroneTokenSource         string `json:"drone_token_source,omitempty"`
	MaxBrainTokens           int    `json:"max_brain_tokens"`
}

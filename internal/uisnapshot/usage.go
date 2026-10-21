package uisnapshot

// Usage supplies display data without making a renderer parse terminal prose.
// Limits of zero are uncapped. Policy changes still use reviewed Go actions.
type Usage struct {
	Day               string          `json:"day"`
	ResetAt           string          `json:"reset_at"`
	Paused            bool            `json:"paused"`
	Requests          int             `json:"requests"`
	Tokens            int64           `json:"tokens"`
	ActiveRequests    int             `json:"active_requests"`
	FailedRequests    int             `json:"failed_requests"`
	EstimatedRequests int             `json:"estimated_requests"`
	DailyRequestLimit int             `json:"daily_request_limit"`
	DailyTokenLimit   int64           `json:"daily_token_limit"`
	ConcurrentLimit   int             `json:"concurrent_limit"`
	Categories        []UsageCategory `json:"categories"`
	KnownCostUSD      *float64        `json:"known_cost_usd,omitempty"`
}

type UsageCategory struct {
	Name     string `json:"name"`
	Requests int    `json:"requests"`
}

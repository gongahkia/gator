package workapp

import "time"

// Chat checks are scoped by the reviewed definition's originating chat. A
// supervisor heartbeat gap is never projected as a successful source check.
type BridgeChatCheck struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	State        string `json:"state"`
	Issue        string `json:"issue,omitempty"`
	NextAction   string `json:"next_action"`
	LastSuccess  string `json:"last_success,omitempty"`
	NextDue      string `json:"next_due,omitempty"`
	GapFrom      string `json:"gap_from,omitempty"`
	GapTo        string `json:"gap_to,omitempty"`
	Unsuccessful int    `json:"unsuccessful"`
}

func bridgeCheckTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

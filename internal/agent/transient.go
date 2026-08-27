package agent

import (
	"errors"
	"fmt"
)

// maxTransientAttempts is one initial Complete plus two retries. Failed
// attempts never enter conversation history: only a successful turn is
// appended. This avoids retry debris amplifying later compaction.
const maxTransientAttempts = 3

// TransientError marks a provider turn that may be retried without mutating
// history. Adapters use it for HTTP 429 and 503. Other 4xx/5xx stay fatal.
type TransientError struct {
	Err error
}

func (e TransientError) Error() string {
	if e.Err == nil {
		return "transient provider error"
	}
	return e.Err.Error()
}

func (e TransientError) Unwrap() error {
	return e.Err
}

// IsTransient reports whether err is a history-safe retry.
func IsTransient(err error) bool {
	var transient TransientError
	return errors.As(err, &transient)
}

// HTTPStatusError wraps a provider HTTP error. 429 and 503 are transient;
// every other status is returned unchanged.
func HTTPStatusError(status int, err error) error {
	if err == nil {
		return nil
	}
	if status == 429 || status == 503 {
		return TransientError{Err: err}
	}
	return err
}

func formatTurnError(step int, err error) error {
	return fmt.Errorf("model turn %d: %w", step, err)
}

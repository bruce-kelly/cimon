package github

import (
	"fmt"
	"time"
)

// AuthError indicates the GitHub token is invalid, expired, or lacks permissions.
type AuthError struct {
	StatusCode int
	Message    string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("authentication failed (HTTP %d): %s", e.StatusCode, e.Message)
}

// RateLimitError indicates GitHub rate limiting (HTTP 429 or X-RateLimit-Remaining: 0).
type RateLimitError struct {
	RetryAfter time.Duration
	ResetAt    time.Time
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited; retry after %v (resets at %s)",
		e.RetryAfter.Round(time.Second), e.ResetAt.Format("15:04:05"))
}

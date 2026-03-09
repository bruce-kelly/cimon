package github

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAuthError_Is(t *testing.T) {
	err := &AuthError{StatusCode: 401, Message: "bad token"}
	var target *AuthError
	assert.True(t, errors.As(err, &target))
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestRateLimitError_RetryAfter(t *testing.T) {
	err := &RateLimitError{
		RetryAfter: 60 * time.Second,
		ResetAt:    time.Now().Add(60 * time.Second),
	}
	var target *RateLimitError
	assert.True(t, errors.As(err, &target))
	assert.Contains(t, err.Error(), "rate limited")
}

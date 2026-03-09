package notify

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanNotify(t *testing.T) {
	// Just verify it doesn't panic
	_ = CanNotify()
}

func TestSend_DoesNotPanic(t *testing.T) {
	// Don't actually send — just verify the function handles gracefully
	// On CI/test environments, notify-send won't exist
	_ = Send("test", "test body")
}

func TestCanNotify_ReturnsBool(t *testing.T) {
	result := CanNotify()
	// Result depends on platform — just verify it's a bool
	assert.IsType(t, true, result)
}

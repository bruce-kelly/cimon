package notify

import (
	"context"
	"os/exec"
	"runtime"
	"time"
)

// CanNotify checks if desktop notifications are available.
func CanNotify() bool {
	switch runtime.GOOS {
	case "linux":
		_, err := exec.LookPath("notify-send")
		return err == nil
	case "darwin":
		_, err := exec.LookPath("osascript")
		return err == nil
	default:
		return false
	}
}

// Send sends a desktop notification with a 5-second timeout.
func Send(title, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch runtime.GOOS {
	case "linux":
		return exec.CommandContext(ctx, "notify-send", title, body).Run()
	case "darwin":
		script := `display notification "` + body + `" with title "` + title + `"`
		return exec.CommandContext(ctx, "osascript", "-e", script).Run()
	default:
		return nil
	}
}

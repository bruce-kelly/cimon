package notify

import (
	"os/exec"
	"runtime"
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

// Send sends a desktop notification.
func Send(title, body string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("notify-send", title, body).Run()
	case "darwin":
		script := `display notification "` + body + `" with title "` + title + `"`
		return exec.Command("osascript", "-e", script).Run()
	default:
		return nil
	}
}

package components

import "time"

// Flash shows a timed message.
type Flash struct {
	Message string
	IsError bool
	ShowAt  time.Time
}

func (f *Flash) Show(message string, isError bool) {
	f.Message = message
	f.IsError = isError
	f.ShowAt = time.Now()
}

func (f *Flash) Visible() bool {
	if f.Message == "" {
		return false
	}
	return time.Since(f.ShowAt) < 3*time.Second
}

func (f *Flash) Clear() {
	f.Message = ""
}

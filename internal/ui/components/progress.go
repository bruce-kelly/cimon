package components

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui"
)

// RenderMiniBar renders a filled/empty bar like ███░░░
func RenderMiniBar(filled, total, width int) string {
	if total == 0 {
		return strings.Repeat("░", width)
	}
	f := filled * width / total
	if f > width {
		f = width
	}
	return strings.Repeat("█", f) + strings.Repeat("░", width-f)
}

// JobProgressColor returns "green", "amber", or "red" based on job states.
func JobProgressColor(jobs []models.Job) string {
	for _, j := range jobs {
		if j.Conclusion == "failure" {
			return "red"
		}
	}
	for _, j := range jobs {
		if j.Status == "in_progress" || j.Status == "queued" {
			return "amber"
		}
	}
	return "green"
}

// RenderJobProgress returns a compact job progress string like [3/5]██░░░
func RenderJobProgress(jobs []models.Job) string {
	if len(jobs) == 0 {
		return ""
	}
	done := 0
	for _, j := range jobs {
		if j.Status == "completed" {
			done++
		}
	}
	total := len(jobs)

	colorName := JobProgressColor(jobs)
	var barColor color.Color
	switch colorName {
	case "red":
		barColor = ui.ColorRed
	case "amber":
		barColor = ui.ColorAmber
	default:
		barColor = ui.ColorGreen
	}

	bar := RenderMiniBar(done, total, 5)
	return fmt.Sprintf("[%d/%d]%s",
		done, total,
		lipgloss.NewStyle().Foreground(barColor).Render(bar),
	)
}

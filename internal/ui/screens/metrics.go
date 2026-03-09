package screens

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/db"
	"github.com/bruce-kelly/cimon/internal/ui"
)

// MetricsModel shows historical CI health and agent stats.
type MetricsModel struct {
	RunStats      *db.RunStatsResult
	TaskStats     *db.TaskStatsResult
	Effectiveness *db.EffectivenessResult
	Width         int
	Height        int
}

func NewMetrics() MetricsModel {
	return MetricsModel{}
}

func (m *MetricsModel) Render() string {
	var sb strings.Builder

	headerStyle := lipgloss.NewStyle().
		Foreground(ui.ColorAccent).
		Bold(true)

	statStyle := lipgloss.NewStyle().
		Foreground(ui.ColorFg)

	labelStyle := lipgloss.NewStyle().
		Foreground(ui.ColorMuted)

	// CI Health section
	sb.WriteString(headerStyle.Render("  CI Health (30 days)") + "\n\n")
	if m.RunStats != nil {
		rate := 0.0
		if m.RunStats.Total > 0 {
			rate = float64(m.RunStats.Success) / float64(m.RunStats.Total) * 100
		}
		sb.WriteString(fmt.Sprintf("  %s %s  %s %s  %s %s  %s %.1f%%\n",
			labelStyle.Render("Total:"),
			statStyle.Render(fmt.Sprintf("%d", m.RunStats.Total)),
			labelStyle.Render("Success:"),
			lipgloss.NewStyle().Foreground(ui.ColorGreen).Render(fmt.Sprintf("%d", m.RunStats.Success)),
			labelStyle.Render("Failure:"),
			lipgloss.NewStyle().Foreground(ui.ColorRed).Render(fmt.Sprintf("%d", m.RunStats.Failure)),
			labelStyle.Render("Rate:"),
			rate,
		))
	} else {
		sb.WriteString(labelStyle.Render("  No data") + "\n")
	}

	sb.WriteString("\n")

	// Agent Tasks section
	sb.WriteString(headerStyle.Render("  Agent Tasks (30 days)") + "\n\n")
	if m.TaskStats != nil {
		sb.WriteString(fmt.Sprintf("  %s %s  %s %s  %s %s\n",
			labelStyle.Render("Total:"),
			statStyle.Render(fmt.Sprintf("%d", m.TaskStats.Total)),
			labelStyle.Render("Completed:"),
			lipgloss.NewStyle().Foreground(ui.ColorGreen).Render(fmt.Sprintf("%d", m.TaskStats.Completed)),
			labelStyle.Render("Failed:"),
			lipgloss.NewStyle().Foreground(ui.ColorRed).Render(fmt.Sprintf("%d", m.TaskStats.Failed)),
		))
	} else {
		sb.WriteString(labelStyle.Render("  No data") + "\n")
	}

	sb.WriteString("\n")

	// Agent Effectiveness section
	sb.WriteString(headerStyle.Render("  Agent Effectiveness (30 days)") + "\n\n")
	if m.Effectiveness != nil {
		prRate := 0.0
		if m.Effectiveness.Dispatched > 0 {
			prRate = float64(m.Effectiveness.CreatedPR) / float64(m.Effectiveness.Dispatched) * 100
		}
		sb.WriteString(fmt.Sprintf("  %s %s  %s %s  %s %.1f%%  %s %.1f%%\n",
			labelStyle.Render("Dispatched:"),
			statStyle.Render(fmt.Sprintf("%d", m.Effectiveness.Dispatched)),
			labelStyle.Render("Created PRs:"),
			lipgloss.NewStyle().Foreground(ui.ColorGreen).Render(fmt.Sprintf("%d", m.Effectiveness.CreatedPR)),
			labelStyle.Render("PR Rate:"),
			prRate,
			labelStyle.Render("Failure Rate:"),
			m.Effectiveness.FailureRate*100,
		))
	} else {
		sb.WriteString(labelStyle.Render("  No data") + "\n")
	}

	return sb.String()
}

package screens

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/agents"
	"github.com/bruce-kelly/cimon/internal/review"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// FocusArea identifies which dashboard panel has focus.
type FocusArea int

const (
	FocusPipeline FocusArea = iota
	FocusReview
	FocusRoster
)

// DashboardModel is the dashboard screen.
type DashboardModel struct {
	Pipeline      *components.PipelineView
	ReviewItems   []review.ReviewItem
	AgentProfiles []agents.AgentProfile
	Dispatched    []*agents.DispatchedAgent
	Focus         FocusArea
	ReviewSel     components.Selector
	RosterSel     components.Selector
	ShowRoster    bool
	Width         int
	Height        int
}

func NewDashboard() DashboardModel {
	return DashboardModel{
		Pipeline: components.NewPipelineView(),
	}
}

func (d *DashboardModel) SetSize(w, h int) {
	d.Width = w
	d.Height = h
}

func (d *DashboardModel) CycleFocus() {
	d.Focus = (d.Focus + 1) % 3
	// Skip roster panel if no agent workflows configured
	if d.Focus == FocusRoster && !d.ShowRoster {
		d.Focus = FocusPipeline
	}
}

func (d *DashboardModel) Render() string {
	if d.Width == 0 {
		return ""
	}

	// Three-panel layout: pipeline (left), review queue (center), agent roster (right)
	panelWidth := d.Width / 3
	if panelWidth < 20 {
		panelWidth = d.Width // single column fallback
	}

	// Panel headers
	headerStyle := lipgloss.NewStyle().
		Foreground(ui.ColorAccent).
		Bold(true)

	// Pipeline panel
	pipeHeader := headerStyle.Render("Pipeline")
	if d.Focus == FocusPipeline {
		pipeHeader = headerStyle.Render("[Pipeline]")
	}
	pipeContent := d.Pipeline.Render(panelWidth)

	// Review queue panel
	reviewHeader := headerStyle.Render("Review Queue")
	if d.Focus == FocusReview {
		reviewHeader = headerStyle.Render("[Review Queue]")
	}
	reviewContent := d.renderReviewQueue(panelWidth)

	// Agent roster panel
	rosterHeader := headerStyle.Render("Agent Roster")
	if d.Focus == FocusRoster {
		rosterHeader = headerStyle.Render("[Agent Roster]")
	}
	rosterContent := d.renderAgentRoster(panelWidth)

	if panelWidth == d.Width {
		// Single column
		return strings.Join([]string{
			pipeHeader, pipeContent, "",
			reviewHeader, reviewContent, "",
			rosterHeader, rosterContent,
		}, "\n")
	}

	// Three columns
	leftPanel := lipgloss.NewStyle().Width(panelWidth).Render(pipeHeader + "\n" + pipeContent)
	centerPanel := lipgloss.NewStyle().Width(panelWidth).Render(reviewHeader + "\n" + reviewContent)
	rightPanel := lipgloss.NewStyle().Width(panelWidth).Render(rosterHeader + "\n" + rosterContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, centerPanel, rightPanel)
}

func (d *DashboardModel) renderReviewQueue(width int) string {
	if len(d.ReviewItems) == 0 {
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  No items for review")
	}

	d.ReviewSel.SetCount(len(d.ReviewItems))
	var sb strings.Builder
	for i, item := range d.ReviewItems {
		selected := d.Focus == FocusReview && i == d.ReviewSel.Index()
		sb.WriteString(d.renderReviewItem(item, selected, width))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (d *DashboardModel) renderReviewItem(item review.ReviewItem, selected bool, width int) string {
	pr := item.PR

	// Escalation color
	escColor := ui.ColorGreen
	switch item.Escalation {
	case review.EscalationAmber:
		escColor = ui.ColorAmber
	case review.EscalationRed:
		escColor = ui.ColorRed
	}

	// Status indicator
	ciDot := ui.StatusDot(pr.CIStatus)
	ciColor := ui.StatusColor(pr.CIStatus)

	// Agent badge
	agentBadge := ""
	if pr.IsAgent {
		agentBadge = lipgloss.NewStyle().Foreground(ui.ColorPurple).Render(" [agent]")
	}

	line := fmt.Sprintf(" %s #%d %s%s",
		lipgloss.NewStyle().Foreground(ciColor).Render(ciDot),
		pr.Number,
		lipgloss.NewStyle().Foreground(escColor).Render(pr.Title),
		agentBadge,
	)

	if selected {
		return lipgloss.NewStyle().Background(ui.ColorSelection).Width(width).Render(line)
	}
	return line
}

func (d *DashboardModel) renderAgentRoster(width int) string {
	var sb strings.Builder

	// Workflow agent profiles
	for i, profile := range d.AgentProfiles {
		selected := d.Focus == FocusRoster && i == d.RosterSel.Index()
		sparkline := components.RenderSparkline(profile.History)

		outcomeColor := ui.ColorGreen
		if profile.LastOutcome == agents.BucketAlert {
			outcomeColor = ui.ColorRed
		}

		line := fmt.Sprintf(" %s %s  %s  %.0f%%",
			sparkline,
			profile.WorkflowFile,
			lipgloss.NewStyle().Foreground(outcomeColor).Render(profile.LastOutcome.String()),
			profile.SuccessRate*100,
		)

		if selected {
			sb.WriteString(lipgloss.NewStyle().Background(ui.ColorSelection).Width(width).Render(line))
		} else {
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}

	// Dispatched agents
	if len(d.Dispatched) > 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  ─ dispatched ─") + "\n")
		for _, agent := range d.Dispatched {
			statusColor := ui.ColorAccent
			if agent.Status == "completed" {
				statusColor = ui.ColorGreen
			} else if agent.Status == "failed" || agent.Status == "killed" {
				statusColor = ui.ColorRed
			}

			prLink := ""
			if agent.PRNumber != nil {
				prLink = fmt.Sprintf(" → PR #%d", *agent.PRNumber)
			}

			line := fmt.Sprintf("  %s %s%s",
				lipgloss.NewStyle().Foreground(statusColor).Render(agent.Status),
				agent.Task,
				lipgloss.NewStyle().Foreground(ui.ColorBlue).Render(prLink),
			)
			sb.WriteString(line + "\n")
		}
	}

	if sb.Len() == 0 {
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  No agent activity")
	}
	return sb.String()
}

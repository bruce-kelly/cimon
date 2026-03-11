package views

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// PRDetailView renders the drill-down into a single pull request.
type PRDetailView struct {
	PR       models.PullRequest
	RepoName string
	Files    []DiffFile
	RawDiff  string
	Cursor   components.Selector
}

// NewPRDetailView creates a PR detail view.
func NewPRDetailView(pr models.PullRequest, repoName string) *PRDetailView {
	return &PRDetailView{
		PR:       pr,
		RepoName: repoName,
	}
}

// SetFiles populates the file list and resets cursor.
func (pv *PRDetailView) SetFiles(files []DiffFile) {
	pv.Files = files
	pv.Cursor.SetCount(len(files))
}

// SelectedFile returns the file at the cursor, or nil if no files.
func (pv *PRDetailView) SelectedFile() *DiffFile {
	if len(pv.Files) == 0 {
		return nil
	}
	return &pv.Files[pv.Cursor.Index()]
}

// Render draws the PR detail view.
func (pv *PRDetailView) Render(width, height int) string {
	var lines []string
	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)

	// PR header
	num := lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true).Render(fmt.Sprintf("#%d", pv.PR.Number))
	lines = append(lines, fmt.Sprintf("  %s  %s", num, pv.PR.Title))

	// Meta line: author, age, size, agent badge
	age := detailFormatAge(time.Since(pv.PR.CreatedAt).Hours())
	size := fmt.Sprintf("+%d -%d (%d lines)", pv.PR.Additions, pv.PR.Deletions, pv.PR.Size())

	meta := fmt.Sprintf("  %s · %s · %s", pv.PR.Author, age, size)
	if pv.PR.IsAgent {
		meta += "   " + lipgloss.NewStyle().Foreground(ui.ColorPurple).Render("agent")
	}
	if pv.PR.Draft {
		meta += "   " + muted.Render("draft")
	}
	lines = append(lines, muted.Render(meta))

	// CI + review status
	ciStr := prCIStatus(pv.PR)
	reviewStr := prReviewStatus(pv.PR)
	lines = append(lines, fmt.Sprintf("  %s  %s", ciStr, reviewStr))
	lines = append(lines, "")

	// Files section
	if len(pv.Files) == 0 {
		lines = append(lines, muted.Render("  Loading diff..."))
	} else {
		filesHeader := muted.Render(fmt.Sprintf("  Files Changed (%d)", len(pv.Files)))
		lines = append(lines, filesHeader)
		for i, f := range pv.Files {
			selected := pv.Cursor.Index() == i
			lines = append(lines, pv.renderFileLine(f, selected, width))
		}
	}

	return strings.Join(lines, "\n")
}

func (pv *PRDetailView) renderFileLine(f DiffFile, selected bool, width int) string {
	adds := lipgloss.NewStyle().Foreground(ui.ColorGreen).Render(fmt.Sprintf("+%d", f.Additions))
	dels := lipgloss.NewStyle().Foreground(ui.ColorRed).Render(fmt.Sprintf("-%d", f.Deletions))

	line := fmt.Sprintf("  %s  %s %s", f.Path, adds, dels)
	if selected {
		style := lipgloss.NewStyle().Background(ui.ColorSelection)
		line = style.Render(padRight(line, width))
	}
	return line
}

func prCIStatus(pr models.PullRequest) string {
	switch pr.CIStatus {
	case "success":
		return lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("CI✓")
	case "failure":
		return lipgloss.NewStyle().Foreground(ui.ColorRed).Render("CI✗")
	default:
		return lipgloss.NewStyle().Foreground(ui.ColorAmber).Render("CI⧗")
	}
}

func prReviewStatus(pr models.PullRequest) string {
	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)
	switch pr.ReviewState {
	case "approved":
		return muted.Render("review: ") + lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("approved ✔")
	case "changes_requested":
		return muted.Render("review: ") + lipgloss.NewStyle().Foreground(ui.ColorRed).Render("changes requested")
	case "pending":
		return muted.Render("review: pending")
	default:
		return muted.Render("review: none")
	}
}

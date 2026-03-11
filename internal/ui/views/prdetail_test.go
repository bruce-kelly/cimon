package views

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPR() models.PullRequest {
	return models.PullRequest{
		Number:      142,
		Title:       "Fix authentication timeout handling",
		Author:      "bfk",
		Repo:        "owner/repo",
		HTMLURL:     "https://github.com/owner/repo/pull/142",
		State:       "open",
		CIStatus:    "success",
		ReviewState: "approved",
		IsAgent:     true,
		AgentSource: "body",
		Additions:   47,
		Deletions:   12,
		CreatedAt:   time.Now().Add(-48 * time.Hour),
		UpdatedAt:   time.Now().Add(-1 * time.Hour),
	}
}

func TestPRDetailView_Render(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")
	out := pv.Render(80, 30)

	assert.Contains(t, out, "#142")
	assert.Contains(t, out, "Fix authentication timeout handling")
	assert.Contains(t, out, "bfk")
	assert.Contains(t, out, "+47")
	assert.Contains(t, out, "-12")
	assert.Contains(t, out, "agent")
	assert.Contains(t, out, "approved")
}

func TestPRDetailView_CIStatusRendered(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")
	out := pv.Render(80, 30)
	assert.Contains(t, out, "CI")
}

func TestPRDetailView_NoFilesBeforeDiff(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")
	out := pv.Render(80, 30)
	assert.Contains(t, out, "Loading diff")
	assert.Equal(t, 0, pv.Cursor.Count())
}

func TestPRDetailView_SetFiles(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")

	files := []DiffFile{
		{Path: "internal/auth/client.go", Additions: 31, Deletions: 8, Offset: 0},
		{Path: "internal/auth/client_test.go", Additions: 12, Deletions: 0, Offset: 20},
	}
	pv.SetFiles(files)
	assert.Equal(t, 2, pv.Cursor.Count())

	out := pv.Render(80, 30)
	assert.Contains(t, out, "Files Changed (2)")
	assert.Contains(t, out, "client.go")
	assert.Contains(t, out, "+31")
	assert.Contains(t, out, "-8")
}

func TestPRDetailView_CursorNavigatesFiles(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")
	pv.SetFiles([]DiffFile{
		{Path: "a.go", Additions: 1, Deletions: 0, Offset: 0},
		{Path: "b.go", Additions: 2, Deletions: 1, Offset: 10},
	})

	assert.Equal(t, 0, pv.Cursor.Index())

	pv.Cursor.Next()
	file := pv.SelectedFile()
	require.NotNil(t, file)
	assert.Equal(t, "b.go", file.Path)
	assert.Equal(t, 10, file.Offset)
}

func TestPRDetailView_SelectedFileNil(t *testing.T) {
	pr := testPR()
	pv := NewPRDetailView(pr, "owner/repo")
	assert.Nil(t, pv.SelectedFile())
}

func TestPRDetailView_NonAgentPR(t *testing.T) {
	pr := testPR()
	pr.IsAgent = false
	pv := NewPRDetailView(pr, "owner/repo")
	out := pv.Render(80, 30)
	assert.NotContains(t, out, "agent")
}

func TestPRDetailView_DraftPR(t *testing.T) {
	pr := testPR()
	pr.Draft = true
	pv := NewPRDetailView(pr, "owner/repo")
	out := pv.Render(80, 30)
	assert.Contains(t, out, "draft")
}

package views

import "strings"

// DiffFile represents a single file in a parsed diff.
type DiffFile struct {
	Path      string
	Additions int
	Deletions int
	Offset    int // line offset into raw diff for log pane scroll
}

// ParseDiffFiles extracts file entries from a unified diff string.
func ParseDiffFiles(raw string) []DiffFile {
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	var files []DiffFile
	var current *DiffFile

	for i, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				files = append(files, *current)
			}
			current = &DiffFile{
				Path:   parseDiffPath(line),
				Offset: i,
			}
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			current.Additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			current.Deletions++
		}
	}
	if current != nil {
		files = append(files, *current)
	}
	return files
}

// parseDiffPath extracts the file path from "diff --git a/path b/path".
// Uses LastIndex to handle paths containing " b/" (e.g., a/internal/b/foo.go).
func parseDiffPath(line string) string {
	idx := strings.LastIndex(line, " b/")
	if idx >= 0 {
		return line[idx+3:]
	}
	parts := strings.SplitN(line, " a/", 2)
	if len(parts) == 2 {
		path := parts[1]
		if i := strings.Index(path, " "); i >= 0 {
			path = path[:i]
		}
		return path
	}
	return line
}

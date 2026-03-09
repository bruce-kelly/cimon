package components

import "strings"

// FilterBar provides case-insensitive multi-term filtering.
type FilterBar struct {
	Query  string
	Active bool
}

// Matches returns true if the text matches all space-separated filter terms.
func (f *FilterBar) Matches(text string) bool {
	if f.Query == "" {
		return true
	}
	lower := strings.ToLower(text)
	terms := strings.Fields(strings.ToLower(f.Query))
	for _, term := range terms {
		if !strings.Contains(lower, term) {
			return false
		}
	}
	return true
}

// HandleKey processes a key for the filter bar.
// Returns true if the key was consumed.
func (f *FilterBar) HandleKey(keyStr string) bool {
	if !f.Active {
		return false
	}
	switch keyStr {
	case "esc":
		f.Active = false
		f.Query = ""
		return true
	case "enter":
		f.Active = false
		return true
	case "backspace":
		if len(f.Query) > 0 {
			f.Query = f.Query[:len(f.Query)-1]
		}
		return true
	default:
		if len(keyStr) == 1 {
			f.Query += keyStr
			return true
		}
		return false
	}
}

func (f *FilterBar) Clear() {
	f.Query = ""
	f.Active = false
}

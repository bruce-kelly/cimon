package components

import "strings"

var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// RenderSparkline creates a unicode bar chart from values.
func RenderSparkline(values []float64) string {
	if len(values) == 0 {
		return ""
	}
	max := 0.0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	if max == 0 {
		return strings.Repeat(string(sparkChars[0]), len(values))
	}
	var buf strings.Builder
	for _, v := range values {
		idx := int((v / max) * float64(len(sparkChars)-1))
		if idx >= len(sparkChars) {
			idx = len(sparkChars) - 1
		}
		buf.WriteRune(sparkChars[idx])
	}
	return buf.String()
}

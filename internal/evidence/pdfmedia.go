package evidence

import (
	"fmt"
	"regexp"
)

// ParseMediaBox extracts the MediaBox dimensions from PDF bytes.
// Returns width, height in PDF points.
func ParseMediaBox(pdfBytes []byte) (float64, float64, error) {
	// MediaBox format: /MediaBox [0 0 612 792]
	re := regexp.MustCompile(`/MediaBox\s*\[\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)\s*\]`)
	matches := re.FindSubmatch(pdfBytes)
	if len(matches) < 5 {
		return 0, 0, nil // not found, caller can use defaults
	}
	// matches[1] = x0, matches[2] = y0, matches[3] = x1, matches[4] = y1
	// Width = x1 - x0, Height = y1 - y0
	x0 := parseFloatSafe(string(matches[1]))
	y0 := parseFloatSafe(string(matches[2]))
	x1 := parseFloatSafe(string(matches[3]))
	y1 := parseFloatSafe(string(matches[4]))
	return x1 - x0, y1 - y0, nil
}

func parseFloatSafe(s string) float64 {
	var f float64
	_, _ = fmt.Sscanf(s, "%f", &f)
	return f
}
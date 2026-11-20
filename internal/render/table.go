package render

import (
	"io"
	"strings"
)

// Table writes rows in columns two spaces apart, as text/tabwriter does, but
// measures each cell without its colors, so colored cells line up. The last
// cell of a row is not padded.
func Table(w io.Writer, rows [][]string) error {
	widths := columnWidths(rows)
	var b strings.Builder
	for _, row := range rows {
		for i, cell := range row {
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-Width(cell)+2))
			}
		}
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func columnWidths(rows [][]string) []int {
	var widths []int
	for _, row := range rows {
		for i, cell := range row[:max(0, len(row)-1)] {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], Width(cell))
		}
	}
	return widths
}

// columnWidth is how wide column i of rows is, its header included.
func columnWidth(rows [][]string, i int) int {
	w := 0
	for _, row := range rows {
		if i < len(row) {
			w = max(w, Width(row[i]))
		}
	}
	return w
}

// cellWidths is how wide each column of rows is, the last included.
func cellWidths(rows [][]string) []int {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], Width(cell))
		}
	}
	return widths
}

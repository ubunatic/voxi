package listing

import (
	"fmt"
	"io"
	"strings"
)

// Column describes one fixed-width table column for WriteTable. Width <= 0
// means "unpadded" -- meant for a single trailing free-text column (e.g. a
// TRANSCRIPT column) that should not be padded out with trailing spaces.
type Column struct {
	Header string
	Width  int
	Right  bool // right-align within Width (numeric columns); default left-align
}

// WriteTable writes a header row followed by one row per entry in rows to
// w. Each row must supply one cell per column (a short row is padded with
// empty cells; extra cells beyond len(cols) are ignored). Columns are
// joined with two spaces, the separator convention `chunks list`
// established before this package existed.
func WriteTable(w io.Writer, cols []Column, rows [][]string) {
	fmt.Fprintln(w, formatRow(cols, headerCells(cols)))
	for _, r := range rows {
		fmt.Fprintln(w, formatRow(cols, r))
	}
}

func headerCells(cols []Column) []string {
	cells := make([]string, len(cols))
	for i, c := range cols {
		cells[i] = c.Header
	}
	return cells
}

func formatRow(cols []Column, cells []string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		v := ""
		if i < len(cells) {
			v = cells[i]
		}
		switch {
		case c.Width <= 0:
			parts[i] = v
		case c.Right:
			parts[i] = fmt.Sprintf("%*s", c.Width, v)
		default:
			parts[i] = fmt.Sprintf("%-*s", c.Width, v)
		}
	}
	return strings.Join(parts, "  ")
}

// FormatSparklineCell brackets a fixed-width Braille sparkline (or an empty
// placeholder when no audio was available to compute one), pads it to
// width, and colorizes it if useColor is true. Padding happens before
// colorizing -- see ColorizeSparkline's doc comment for why the order
// matters.
func FormatSparklineCell(sparkline string, width int, useColor bool) string {
	cell := fmt.Sprintf("%-*s", width, "["+sparkline+"]")
	if useColor {
		cell = ColorizeSparkline(cell)
	}
	return cell
}

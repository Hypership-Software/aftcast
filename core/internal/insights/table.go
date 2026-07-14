package insights

import (
	"fmt"
	"strings"

	"github.com/Hypership-Software/atlas/internal/ui"

	"github.com/charmbracelet/x/ansi"
)

const (
	colGap        = 2
	cursorPointer = "▸ "
	cursorBlank   = "  "
	maxTableRows  = 15
)

// tableColumn is one column of the session list: a header title, its cells (one
// per row, possibly ANSI-styled), and a floor width. floor == 0 marks a fixed
// column that never shrinks; a positive floor lets the column give up width down
// to that floor when the table would otherwise overflow the terminal.
type tableColumn struct {
	title string
	cells []string
	floor int
}

// columnWidths sizes each column to its natural (ANSI-aware) width — the widest
// of its title or any cell — then, only if the row would overflow maxWidth,
// shrinks the flexible columns (those with a floor) toward their floors, widest
// first, so the fixed leading columns are never clipped. maxWidth <= 0 renders
// every column at its natural width: the wide-terminal case, where nothing
// should truncate. Measuring with ansi.StringWidth is what fixes the clipping a
// styled cell suffered under bubbles/table, whose runewidth.Truncate counted the
// cell's escape bytes as display width.
func columnWidths(cols []tableColumn, maxWidth int) []int {
	w := make([]int, len(cols))
	if len(cols) == 0 {
		return w
	}
	total := colGap * (len(cols) - 1)
	for i, c := range cols {
		w[i] = ansi.StringWidth(c.title)
		for _, cell := range c.cells {
			if cw := ansi.StringWidth(cell); cw > w[i] {
				w[i] = cw
			}
		}
		total += w[i]
	}
	deficit := total - maxWidth
	if maxWidth <= 0 || deficit <= 0 {
		return w
	}
	// Shrink one column-unit at a time from the currently widest flexible column,
	// so the biggest offender gives up space before an already-tight one — the
	// columns converge on a balanced fit instead of gutting a single column.
	for deficit > 0 {
		widest, room := -1, 0
		for i, c := range cols {
			if c.floor > 0 && w[i]-c.floor > room {
				widest, room = i, w[i]-c.floor
			}
		}
		if widest < 0 {
			break // nothing left to give; let the row overflow rather than clip fixed columns
		}
		w[widest]--
		deficit--
	}
	return w
}

// padCell left-aligns a possibly-styled cell to width w. A cell wider than w is
// truncated with an ellipsis — ANSI-aware on both the measure and the cut, so a
// cell's colour codes are never counted as width nor severed mid-sequence.
func padCell(s string, w int) string {
	if w <= 0 {
		return ""
	}
	vw := ansi.StringWidth(s)
	if vw > w {
		return ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-vw)
}

// windowBounds returns the [start,end) slice of rows to render so that a list
// taller than maxTableRows scrolls to keep the cursor in view, biased to center
// the cursor once scrolling begins.
func windowBounds(cursor, rows, max int) (int, int) {
	if rows <= max {
		return 0, rows
	}
	start := cursor - max/2
	if start < 0 {
		start = 0
	}
	end := start + max
	if end > rows {
		end = rows
		start = end - max
	}
	return start, end
}

// renderSessionTable lays out the columns as a width-responsive, ANSI-aware table
// with a selection pointer. Column widths are computed over every row (stable as
// the cursor scrolls), but only the visible window is rendered.
func renderSessionTable(cols []tableColumn, cursor, width int) string {
	rows := 0
	if len(cols) > 0 {
		rows = len(cols[0].cells)
	}
	widths := columnWidths(cols, width-ansi.StringWidth(cursorPointer))

	var b strings.Builder
	b.WriteString(cursorBlank)
	b.WriteString(ui.Bold(joinCells(cols, widths, -1)))

	start, end := windowBounds(cursor, rows, maxTableRows)
	if start > 0 {
		b.WriteString("\n" + cursorBlank + ui.Hint(scrollNote(start, "above")))
	}
	for r := start; r < end; r++ {
		b.WriteByte('\n')
		if r == cursor {
			b.WriteString(ui.Warn(cursorPointer))
		} else {
			b.WriteString(cursorBlank)
		}
		b.WriteString(joinCells(cols, widths, r))
	}
	if end < rows {
		b.WriteString("\n" + cursorBlank + ui.Hint(scrollNote(rows-end, "below")))
	}
	return b.String()
}

// joinCells renders one line: each column's cell padded to its width, joined by
// the column gap. row == -1 renders the header titles.
func joinCells(cols []tableColumn, widths []int, row int) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		cell := c.title
		if row >= 0 {
			cell = c.cells[row]
		}
		parts[i] = padCell(cell, widths[i])
	}
	return strings.TrimRight(strings.Join(parts, strings.Repeat(" ", colGap)), " ")
}

func scrollNote(n int, dir string) string {
	word := "sessions"
	if n == 1 {
		word = "session"
	}
	return fmt.Sprintf("… %d more %s %s", n, word, dir)
}

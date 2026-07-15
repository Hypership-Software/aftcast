package insights

import (
	"strings"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/ui"

	"github.com/charmbracelet/x/ansi"
)

// A styled cell must be measured and padded by its VISIBLE width, not its byte
// length — this is the exact defect that made bubbles/table clip "✓ clean" to
// "✓ cle…": runewidth counted the colour escape bytes as display columns.
func TestPadCellIsANSIAware(t *testing.T) {
	styled := ui.OK("✓ clean") // carries colour escape codes
	got := padCell(styled, 10)
	if w := ansi.StringWidth(got); w != 10 {
		t.Fatalf("padded visible width = %d, want 10", w)
	}
	if !strings.Contains(ansi.Strip(got), "✓ clean") {
		t.Fatalf("padding clipped the visible content: %q", ansi.Strip(got))
	}
}

func TestPadCellTruncatesWithEllipsis(t *testing.T) {
	got := padCell("a very long cell value", 8)
	if w := ansi.StringWidth(got); w != 8 {
		t.Fatalf("truncated width = %d, want 8", w)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncation missing ellipsis: %q", got)
	}
}

// On a wide terminal every column renders at its natural width — nothing
// truncates. This is the wide-monitor complaint: fixed columns clipped content
// even with room to spare.
func TestColumnWidthsNaturalWhenWide(t *testing.T) {
	cols := []tableColumn{
		{title: "When", cells: []string{"4h ago"}},
		{title: "Flags", cells: []string{"⚠ untrusted input ⚑ 1 flagged ★ 6 skills"}, floor: 16},
	}
	w := columnWidths(cols, 200)
	if w[0] != 6 { // "4h ago" wider than "When"
		t.Errorf("When width = %d, want 6", w[0])
	}
	flagsNatural := ansi.StringWidth(cols[1].cells[0])
	if w[1] != flagsNatural {
		t.Errorf("Flags width = %d, want natural %d (no truncation on a wide terminal)", w[1], flagsNatural)
	}
}

// When the terminal is too narrow, only the flexible (floored) columns give up
// width; the fixed columns keep their natural width.
func TestColumnWidthsShrinkFlexibleOnly(t *testing.T) {
	cols := []tableColumn{
		{title: "When", cells: []string{"4h ago"}}, // fixed (floor 0)
		{title: "Work", cells: []string{"648 calls · 83 files"}, floor: 10},
		{title: "Flags", cells: []string{"⚠ untrusted input ⚑ 1 flagged ★ 6 skills"}, floor: 16},
	}
	w := columnWidths(cols, 40)
	if w[0] != 6 {
		t.Errorf("fixed When column shrank to %d, want 6", w[0])
	}
	if w[1] < 10 || w[2] < 16 {
		t.Errorf("flexible columns dropped below floor: work=%d flags=%d", w[1], w[2])
	}
	total := w[0] + w[1] + w[2] + colGap*2
	if total > 40 {
		t.Errorf("columns still overflow: total=%d, want <=40", total)
	}
}

func TestRenderSessionTablePointsAtCursor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	cols := []tableColumn{
		{title: "When", cells: []string{"1h ago", "2h ago", "3h ago"}},
	}
	out := renderSessionTable(cols, 1, 120, maxTableRows)
	lines := strings.Split(out, "\n")
	// header + 3 rows
	if len(lines) != 4 {
		t.Fatalf("want 4 lines (header + 3 rows), got %d:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[2], cursorPointer) {
		t.Errorf("cursor row (2h ago) missing pointer: %q", lines[2])
	}
	if strings.HasPrefix(lines[1], cursorPointer) || strings.HasPrefix(lines[3], cursorPointer) {
		t.Errorf("non-cursor rows should not carry the pointer:\n%s", out)
	}
}

func TestRenderSessionTableWindowsLongList(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	cells := make([]string, 30)
	for i := range cells {
		cells[i] = "row"
	}
	cols := []tableColumn{{title: "When", cells: cells}}
	out := renderSessionTable(cols, 29, 120, maxTableRows) // cursor at the last row
	if !strings.Contains(out, "more sessions above") {
		t.Errorf("windowed list missing scroll-up note:\n%s", out)
	}
	if strings.Contains(out, "more sessions below") {
		t.Errorf("cursor at end should have nothing below:\n%s", out)
	}
	rowCount := strings.Count(out, "row")
	if rowCount > maxTableRows {
		t.Errorf("rendered %d rows, want <= %d", rowCount, maxTableRows)
	}
}

func TestRenderSessionTableHonoursExplicitRowLimit(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	cells := []string{"row1", "row2", "row3", "row4", "row5"}
	out := renderSessionTable([]tableColumn{{title: "When", cells: cells}}, 2, 120, 3)
	visible := 0
	for _, cell := range cells {
		if strings.Contains(out, cell) {
			visible++
		}
	}
	if visible != 3 {
		t.Fatalf("rendered %d data rows, want 3:\n%s", visible, out)
	}
	if !strings.Contains(out, "more sessions above") || !strings.Contains(out, "more sessions below") {
		t.Fatalf("limited table missing scroll notes:\n%s", out)
	}
}

func TestRenderSessionTableHonoursZeroAndOneRowLimits(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	cells := []string{"row1", "row2", "row3"}
	for _, tt := range []struct {
		limit int
		want  int
	}{{0, 0}, {1, 1}} {
		out := renderSessionTable([]tableColumn{{title: "When", cells: cells}}, 1, 120, tt.limit)
		visible := 0
		for _, cell := range cells {
			if strings.Contains(out, cell) {
				visible++
			}
		}
		if visible != tt.want {
			t.Fatalf("limit %d rendered %d rows, want %d:\n%s", tt.limit, visible, tt.want, out)
		}
	}
}

func TestZeroRowStatusCountsAroundSelectedSession(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	cols := []tableColumn{{title: "When", cells: []string{"first", "middle", "last"}}}
	tests := []struct {
		name   string
		cursor int
		want   string
	}{
		{name: "first", cursor: 0, want: "selected 1 of 3 · 0 above · 2 below"},
		{name: "middle", cursor: 1, want: "selected 2 of 3 · 1 above · 1 below"},
		{name: "last", cursor: 2, want: "selected 3 of 3 · 2 above · 0 below"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderCompactSessionTable(cols, tt.cursor, 80, 0, 2)
			plain := ansi.Strip(out)
			if !strings.Contains(plain, tt.want) {
				t.Fatalf("zero-row status missing exact position %q:\n%s", tt.want, plain)
			}
			if !strings.Contains(plain, "2 empty sessions hidden") {
				t.Fatalf("zero-row status omitted hidden count:\n%s", plain)
			}
			status := strings.TrimSpace(strings.Split(plain, "\n")[1])
			if width := ansi.StringWidth(status); width > 80 {
				t.Fatalf("zero-row status width = %d, want <= 80: %q", width, status)
			}
		})
	}
}

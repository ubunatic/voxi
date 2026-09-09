package listing

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteTableAlignsFixedWidthColumnsAndLeavesLastColumnUnpadded(t *testing.T) {
	cols := []Column{
		{Header: "NAME", Width: 8},
		{Header: "RMS", Width: 5, Right: true},
		{Header: "TRANSCRIPT", Width: 0},
	}
	rows := [][]string{
		{"a", "12", "hi there"},
		{"bee", "842", "bye"},
	}

	var buf bytes.Buffer
	WriteTable(&buf, cols, rows)
	out := buf.String()

	if !strings.Contains(out, "NAME") || !strings.Contains(out, "RMS") || !strings.Contains(out, "TRANSCRIPT") {
		t.Fatalf("expected all column headers in output, got:\n%s", out)
	}
	if !strings.Contains(out, "a       ") { // left-aligned, padded to width 8
		t.Fatalf("expected left-aligned NAME cell, got:\n%s", out)
	}
	if !strings.Contains(out, "   12") { // right-aligned, padded to width 5
		t.Fatalf("expected right-aligned RMS cell, got:\n%s", out)
	}
	if !strings.Contains(out, "hi there\n") {
		t.Fatalf("expected unpadded trailing TRANSCRIPT cell, got:\n%s", out)
	}
}

func TestFormatSparklineCellPadsBeforeColorizing(t *testing.T) {
	cell := FormatSparklineCell("⣶⣶⣶⣶⣶⣀⣀⣀⣀⣀", 12, false)
	if cell != "[⣶⣶⣶⣶⣶⣀⣀⣀⣀⣀]" {
		t.Fatalf("expected exact bracketed sparkline with no color, got %q", cell)
	}

	colored := FormatSparklineCell("⣶⣶⣶⣶⣶⣀⣀⣀⣀⣀", 12, true)
	if !strings.Contains(colored, "\x1b[") {
		t.Fatalf("expected ANSI codes when useColor is true, got %q", colored)
	}
}

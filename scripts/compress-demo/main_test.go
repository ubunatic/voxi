package main

import "testing"

func TestSplitSigned(t *testing.T) {
	cases := []struct {
		in             float64
		wantTrim, want float64
	}{
		{0, 0, 0},
		{2.5, 0, 2.5},
		{-2.5, 2.5, 0},
	}
	for _, c := range cases {
		trim, pad := splitSigned(c.in)
		if trim != c.wantTrim || pad != c.want {
			t.Errorf("splitSigned(%v) = (%v, %v), want (%v, %v)", c.in, trim, pad, c.wantTrim, c.want)
		}
	}
}

func TestFmtSecs(t *testing.T) {
	if got := fmtSecs(2); got != "2.000" {
		t.Errorf("fmtSecs(2) = %q, want %q", got, "2.000")
	}
	if got := fmtSecs(2.5); got != "2.500" {
		t.Errorf("fmtSecs(2.5) = %q, want %q", got, "2.500")
	}
}

func TestParseSeconds(t *testing.T) {
	if got := parseSeconds("START", []string{"a", "b", "c"}, 5); got != 0 {
		t.Errorf("parseSeconds with missing index = %v, want 0", got)
	}
	if got := parseSeconds("START", []string{"a", "b", "c", "-3.5"}, 3); got != -3.5 {
		t.Errorf("parseSeconds(-3.5) = %v, want -3.5", got)
	}
}

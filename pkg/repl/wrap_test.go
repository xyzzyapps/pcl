package repl

import "testing"

func TestWrapDisplayWordBreak(t *testing.T) {
	lines := WrapDisplay("hello world from pcl", 10)
	if len(lines) < 2 {
		t.Fatalf("expected wrap, got %#v", lines)
	}
	for _, ln := range lines {
		if visibleWidth(ln) > 10 {
			t.Fatalf("line too wide %q (%d)", ln, visibleWidth(ln))
		}
	}
}

func TestWrapDisplayIgnoresANSI(t *testing.T) {
	s := "\033[2;3mhello world\033[0m"
	lines := WrapDisplay(s, 20)
	if len(lines) != 1 {
		t.Fatalf("short ansi string should be one line, got %#v", lines)
	}
}

func TestWrapDisplayLongToken(t *testing.T) {
	s := "abcdefghijklmnopqrstuvwxyz"
	lines := WrapDisplay(s, 10)
	if len(lines) < 2 {
		t.Fatalf("expected hard wrap, got %#v", lines)
	}
}

package repl

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// WrapDisplay word-wraps s to width columns, ignoring ANSI CSI sequences
// so escape codes do not count toward width or split across lines.
func WrapDisplay(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	if s == "" {
		return []string{""}
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		lines = append(lines, wrapParagraph(para, width)...)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func wrapParagraph(s string, width int) []string {
	if visibleWidth(s) <= width {
		return []string{s}
	}
	var lines []string
	var buf []byte
	col := 0
	breakAt := -1
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			j := skipANSI(s, i)
			buf = append(buf, s[i:j]...)
			i = j
			continue
		}
		r, sz := utf8.DecodeRuneInString(s[i:])
		rw := 1
		if col+rw > width && col > 0 {
			cut := len(buf)
			if breakAt > 0 {
				cut = breakAt
			}
			lines = append(lines, strings.TrimRightFunc(string(buf[:cut]), unicode.IsSpace))
			rest := buf[cut:]
			for len(rest) > 0 && rest[0] == ' ' {
				rest = rest[1:]
			}
			buf = append([]byte(nil), rest...)
			col = visibleWidth(string(buf))
			breakAt = -1
			continue
		}
		buf = append(buf, s[i:i+sz]...)
		if r == ' ' {
			breakAt = len(buf)
		}
		col += rw
		i += sz
	}
	if len(buf) > 0 {
		lines = append(lines, string(buf))
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func skipANSI(s string, i int) int {
	if i >= len(s) || s[i] != '\x1b' {
		return i
	}
	i++
	if i < len(s) && s[i] == '[' {
		i++
		for i < len(s) {
			c := s[i]
			i++
			if c >= '@' && c <= '~' {
				break
			}
		}
	}
	return i
}

func visibleWidth(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			i = skipANSI(s, i)
			continue
		}
		_, sz := utf8.DecodeRuneInString(s[i:])
		n++
		i += sz
	}
	return n
}

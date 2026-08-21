package core

import (
	"unicode/utf8"
)

// RuneCount returns the number of UTF-8 codepoints in the string.
func RuneCount(s string) int {
	return utf8.RuneCountInString(s)
}

// IsValidUTF8 checks whether byte slice is valid UTF-8.
func IsValidUTF8(b []byte) bool {
	return utf8.Valid(b)
}

// IsValidUTF8String checks whether string is valid UTF-8.
func IsValidUTF8String(s string) bool {
	return utf8.ValidString(s)
}

// RuneSubstr slices a string by rune start and end indices.
// Negative indices count from the end of the string.
func RuneSubstr(s string, start, end int) string {
	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return ""
	}

	if start < 0 {
		start = n + start
	}
	if end < 0 {
		end = n + end
	}

	if start < 0 {
		start = 0
	}
	if start > n {
		return ""
	}
	if end > n {
		end = n
	}
	if end < start {
		return ""
	}

	return string(runes[start:end])
}

// RuneAt returns the rune at given index (supporting negative indexing).
func RuneAt(s string, idx int) (rune, bool) {
	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return 0, false
	}
	if idx < 0 {
		idx = n + idx
	}
	if idx < 0 || idx >= n {
		return 0, false
	}
	return runes[idx], true
}

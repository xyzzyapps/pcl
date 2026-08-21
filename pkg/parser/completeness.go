package parser

// IsCompleteCommand checks whether input buffer has closed all blocks, quotes, and brackets.
// Returns (isComplete, missingDelim).
func IsCompleteCommand(input string) (bool, string) {
	runes := []rune(input)
	n := len(runes)
	if n == 0 {
		return true, ""
	}

	inDoubleQuote := false
	braceDepth := 0
	bracketDepth := 0
	parenDepth := 0

	i := 0
	for i < n {
		ch := runes[i]

		// Handle escapes
		if ch == '\\' {
			i += 2
			continue
		}

		// Handle comments if not in string
		if ch == '#' && !inDoubleQuote && braceDepth == 0 && bracketDepth == 0 {
			// Skip to newline
			for i < n && runes[i] != '\n' {
				i++
			}
			continue
		}

		if inDoubleQuote {
			if ch == '"' {
				inDoubleQuote = false
			}
			i++
			continue
		}

		// Check for Perl quoting prefix: p{, q{, qq{, qx{, qw{
		if (ch == 'p' || ch == 'q') && i+1 < n {
			p1 := runes[i+1]
			if isQuoteDelimiter(p1) {
				// Scan delimited content
				closeDelim := getClosingDelimiter(p1)
				isPaired := (p1 != closeDelim)
				delimDepth := 1
				i += 2 // skip p and open delim
				for i < n {
					c := runes[i]
					if c == '\\' {
						i += 2
						continue
					}
					if isPaired {
						if c == p1 {
							delimDepth++
						} else if c == closeDelim {
							delimDepth--
							if delimDepth == 0 {
								i++
								break
							}
						}
					} else {
						if c == closeDelim {
							i++
							break
						}
					}
					i++
				}
				if delimDepth > 0 {
					return false, string(closeDelim)
				}
				continue
			}
		}

		switch ch {
		case '"':
			inDoubleQuote = true
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		}

		i++
	}

	if inDoubleQuote {
		return false, `"`
	}
	if braceDepth > 0 {
		return false, "}"
	}
	if bracketDepth > 0 {
		return false, "]"
	}
	if parenDepth > 0 {
		return false, ")"
	}

	// Check if ends with trailing pipe '|' or trailing backslash '\'
	trimmed := []rune(input)
	j := len(trimmed) - 1
	for j >= 0 && (trimmed[j] == ' ' || trimmed[j] == '\t' || trimmed[j] == '\r' || trimmed[j] == '\n') {
		j--
	}
	if j >= 0 && (trimmed[j] == '|' || trimmed[j] == '\\') {
		return false, "continuation"
	}

	return true, ""
}

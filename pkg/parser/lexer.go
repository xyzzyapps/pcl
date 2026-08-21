package parser

import (
	"strings"
	"unicode"
)

// Lexer converts PCL source into a sequence of Tokens.
type Lexer struct {
	source []rune
	pos    int
	length int
	line   int
	col    int
}

func NewLexer(input string) *Lexer {
	runes := []rune(input)
	return &Lexer{
		source: runes,
		pos:    0,
		length: len(runes),
		line:   1,
		col:    1,
	}
}

func (l *Lexer) peek() rune {
	if l.pos >= l.length {
		return 0
	}
	return l.source[l.pos]
}

func (l *Lexer) peekAhead(n int) rune {
	if l.pos+n >= l.length {
		return 0
	}
	return l.source[l.pos+n]
}

func (l *Lexer) advance() rune {
	if l.pos >= l.length {
		return 0
	}
	ch := l.source[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) skipWhitespace() {
	for l.pos < l.length {
		ch := l.peek()
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.advance()
		} else {
			break
		}
	}
}

// NextToken returns the next lexical token.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	if l.pos >= l.length {
		return Token{Type: TokEOF, Line: l.line, Column: l.col}
	}

	startLine := l.line
	startCol := l.col
	ch := l.peek()

	// Newlines and Semicolons are command separators (EOL)
	if ch == '\n' || ch == ';' {
		l.advance()
		return Token{Type: TokEOL, Value: string(ch), Line: startLine, Column: startCol}
	}

	// Comment (#) until end of line
	if ch == '#' {
		var sb strings.Builder
		for l.pos < l.length && l.peek() != '\n' {
			sb.WriteRune(l.advance())
		}
		// Return EOL or recursive next token
		return l.NextToken()
	}

	// Check for Redirections: 2>&1, 2>>, 2>, >>, >, <, |
	if ch == '2' && l.peekAhead(1) == '>' {
		if l.peekAhead(2) == '&' && l.peekAhead(3) == '1' {
			l.advance() // 2
			l.advance() // >
			l.advance() // &
			l.advance() // 1
			return Token{Type: TokRedirectErrOut, Value: "2>&1", Line: startLine, Column: startCol}
		}
		if l.peekAhead(2) == '>' {
			l.advance() // 2
			l.advance() // >
			l.advance() // >
			return Token{Type: TokRedirectErrApp, Value: "2>>", Line: startLine, Column: startCol}
		}
		l.advance() // 2
		l.advance() // >
		return Token{Type: TokRedirectErr, Value: "2>", Line: startLine, Column: startCol}
	}

	if ch == '>' {
		l.advance()
		if l.peek() == '>' {
			l.advance()
			return Token{Type: TokRedirectAppend, Value: ">>", Line: startLine, Column: startCol}
		}
		return Token{Type: TokRedirectOut, Value: ">", Line: startLine, Column: startCol}
	}

	if ch == '<' {
		l.advance()
		return Token{Type: TokRedirectIn, Value: "<", Line: startLine, Column: startCol}
	}

	if ch == '|' {
		l.advance()
		if l.peek() == '>' {
			l.advance()
			return Token{Type: TokPipeTap, Value: "|>", Line: startLine, Column: startCol}
		}
		return Token{Type: TokPipe, Value: "|", Line: startLine, Column: startCol}
	}

	if ch == '=' {
		l.advance()
		return Token{Type: TokAssign, Value: "=", Line: startLine, Column: startCol}
	}

	// Check for Perl-style quoting operators: p{...}, q{...}, qq{...}, qx{...}, qw{...}
	if tok, ok := l.tryScanPerlQuote(startLine, startCol); ok {
		return tok
	}

	// Double Quoted String: "..."
	if ch == '"' {
		return l.scanQuotedString(startLine, startCol)
	}

	// Parenthesized Code Block, Condition, or Expression: (...)
	if ch == '(' {
		return l.scanParens(startLine, startCol)
	}

	// Array / Dict Literal: {...}
	if ch == '{' {
		return l.scanBraces(startLine, startCol)
	}

	// Bracket Command Substitution: [...]
	if ch == '[' {
		return l.scanBrackets(startLine, startCol)
	}

	// Standard Word / Identifier / Number / Variable
	return l.scanWord(startLine, startCol)
}

func (l *Lexer) tryScanPerlQuote(startLine, startCol int) (Token, bool) {
	ch := l.peek()
	p1 := l.peekAhead(1)
	p2 := l.peekAhead(2)

	// p{...} or p(...) or p[...] or p<...> or p/.../ or p!{...}
	if ch == 'p' {
		// p!{...} -> streaming prompt
		if p1 == '!' && isQuoteDelimiter(p2) {
			l.advance() // p
			l.advance() // !
			delim := l.advance()
			content := l.scanDelimited(delim)
			return Token{Type: TokPerlPrompt, Value: "!" + content, Line: startLine, Column: startCol}, true
		}
		if isQuoteDelimiter(p1) {
			l.advance() // p
			delim := l.advance()
			content := l.scanDelimited(delim)
			return Token{Type: TokPerlPrompt, Value: content, Line: startLine, Column: startCol}, true
		}
	}

	// q{...}, qq{...}, qx{...}, qw{...}
	if ch == 'q' {
		if isQuoteDelimiter(p1) {
			l.advance() // q
			delim := l.advance()
			content := l.scanDelimited(delim)
			return Token{Type: TokPerlRaw, Value: content, Line: startLine, Column: startCol}, true
		}
		if p1 == 'q' && isQuoteDelimiter(p2) {
			l.advance() // q
			l.advance() // q
			delim := l.advance()
			content := l.scanDelimited(delim)
			return Token{Type: TokPerlInterp, Value: content, Line: startLine, Column: startCol}, true
		}
		if p1 == 'x' && isQuoteDelimiter(p2) {
			l.advance() // q
			l.advance() // x
			delim := l.advance()
			content := l.scanDelimited(delim)
			return Token{Type: TokPerlExec, Value: content, Line: startLine, Column: startCol}, true
		}
		if p1 == 'w' && isQuoteDelimiter(p2) {
			l.advance() // q
			l.advance() // w
			delim := l.advance()
			content := l.scanDelimited(delim)
			return Token{Type: TokPerlWords, Value: content, Line: startLine, Column: startCol}, true
		}
	}

	return Token{}, false
}

func isQuoteDelimiter(ch rune) bool {
	return ch == '{' || ch == '(' || ch == '[' || ch == '<' || ch == '/' || ch == '|' || ch == '"' || ch == '\''
}

func getClosingDelimiter(open rune) rune {
	switch open {
	case '{':
		return '}'
	case '(':
		return ')'
	case '[':
		return ']'
	case '<':
		return '>'
	default:
		return open
	}
}

func (l *Lexer) scanDelimited(open rune) string {
	close := getClosingDelimiter(open)
	isPaired := (open != close)
	depth := 1
	var sb strings.Builder

	for l.pos < l.length {
		ch := l.advance()
		if ch == '\\' && l.pos < l.length {
			next := l.advance()
			if next == close || next == open || next == '\\' {
				sb.WriteRune(next)
			} else {
				sb.WriteRune('\\')
				sb.WriteRune(next)
			}
			continue
		}

		if isPaired {
			if ch == open {
				depth++
				sb.WriteRune(ch)
			} else if ch == close {
				depth--
				if depth == 0 {
					break
				}
				sb.WriteRune(ch)
			} else {
				sb.WriteRune(ch)
			}
		} else {
			if ch == close {
				break
			}
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

func (l *Lexer) scanQuotedString(startLine, startCol int) Token {
	l.advance() // skip open "
	var sb strings.Builder

	for l.pos < l.length {
		ch := l.peek()
		if ch == '\\' {
			l.advance()
			if l.pos < l.length {
				esc := l.advance()
				switch esc {
				case 'n':
					sb.WriteRune('\n')
				case 'r':
					sb.WriteRune('\r')
				case 't':
					sb.WriteRune('\t')
				case '"':
					sb.WriteRune('"')
				case '\\':
					sb.WriteRune('\\')
				case '$':
					sb.WriteString(`\$`)
				case '[':
					sb.WriteString(`\[`)
				default:
					sb.WriteRune('\\')
					sb.WriteRune(esc)
				}
			}
			continue
		}
		if ch == '"' {
			l.advance()
			break
		}
		sb.WriteRune(l.advance())
	}

	return Token{Type: TokString, Value: sb.String(), Line: startLine, Column: startCol}
}

func (l *Lexer) scanParens(startLine, startCol int) Token {
	l.advance() // skip open (
	depth := 1
	var sb strings.Builder

	for l.pos < l.length {
		ch := l.advance()
		if ch == '\\' && l.pos < l.length {
			next := l.advance()
			sb.WriteRune('\\')
			sb.WriteRune(next)
			continue
		}
		if ch == '(' {
			depth++
			sb.WriteRune(ch)
		} else if ch == ')' {
			depth--
			if depth == 0 {
				break
			}
			sb.WriteRune(ch)
		} else {
			sb.WriteRune(ch)
		}
	}

	return Token{Type: TokParen, Value: sb.String(), Line: startLine, Column: startCol}
}

func (l *Lexer) scanBraces(startLine, startCol int) Token {
	l.advance() // skip open {
	depth := 1
	var sb strings.Builder

	for l.pos < l.length {
		ch := l.advance()
		if ch == '\\' && l.pos < l.length {
			next := l.advance()
			sb.WriteRune('\\')
			sb.WriteRune(next)
			continue
		}
		if ch == '{' {
			depth++
			sb.WriteRune(ch)
		} else if ch == '}' {
			depth--
			if depth == 0 {
				break
			}
			sb.WriteRune(ch)
		} else {
			sb.WriteRune(ch)
		}
	}

	return Token{Type: TokBrace, Value: sb.String(), Line: startLine, Column: startCol}
}

func (l *Lexer) scanBrackets(startLine, startCol int) Token {
	l.advance() // skip open [
	depth := 1
	var sb strings.Builder

	for l.pos < l.length {
		ch := l.advance()
		if ch == '\\' && l.pos < l.length {
			next := l.advance()
			sb.WriteRune('\\')
			sb.WriteRune(next)
			continue
		}
		if ch == '[' {
			depth++
			sb.WriteRune(ch)
		} else if ch == ']' {
			depth--
			if depth == 0 {
				break
			}
			sb.WriteRune(ch)
		} else {
			sb.WriteRune(ch)
		}
	}

	return Token{Type: TokBracket, Value: sb.String(), Line: startLine, Column: startCol}
}

func (l *Lexer) scanWord(startLine, startCol int) Token {
	var sb strings.Builder
	parenDepth := 0
	bracketDepth := 0
	inQuote := false
	var quoteChar rune

	for l.pos < l.length {
		ch := l.peek()

		if inQuote {
			if ch == '\\' && l.pos+1 < l.length {
				sb.WriteRune(l.advance())
				sb.WriteRune(l.advance())
				continue
			}
			if ch == quoteChar {
				inQuote = false
				quoteChar = rune(0)
			}
			sb.WriteRune(l.advance())
			continue
		}

		if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
			sb.WriteRune(l.advance())
			continue
		}

		if ch == '(' {
			parenDepth++
			sb.WriteRune(l.advance())
			continue
		}
		if ch == ')' {
			if parenDepth > 0 {
				parenDepth--
			}
			sb.WriteRune(l.advance())
			continue
		}
		if ch == '[' {
			bracketDepth++
			sb.WriteRune(l.advance())
			continue
		}
		if ch == ']' {
			if bracketDepth > 0 {
				bracketDepth--
			}
			sb.WriteRune(l.advance())
			continue
		}

		if parenDepth == 0 && bracketDepth == 0 {
			if unicode.IsSpace(ch) || ch == ';' || ch == '|' || ch == '<' || ch == '>' || ch == '#' {
				break
			}
			if ch == '=' && sb.Len() == 0 {
				l.advance()
				return Token{Type: TokAssign, Value: "=", Line: startLine, Column: startCol}
			}
		}

		sb.WriteRune(l.advance())
	}

	word := sb.String()
	if word == "with" {
		return Token{Type: TokWith, Value: "with", Line: startLine, Column: startCol}
	}
	if strings.HasPrefix(word, "$") {
		return Token{Type: TokVariable, Value: word, Line: startLine, Column: startCol}
	}

	return Token{Type: TokWord, Value: word, Line: startLine, Column: startCol}
}

// TokenizeAll returns the complete token list for input.
func TokenizeAll(input string) []Token {
	lexer := NewLexer(input)
	var tokens []Token
	for {
		tok := lexer.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TokEOF {
			break
		}
	}
	return tokens
}

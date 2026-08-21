package parser

// TokenType specifies the lexical token classification.
type TokenType int

const (
	TokEOF TokenType = iota
	TokEOL
	TokWord
	TokString          // "..."
	TokParen           // (...) code blocks, conditions, expressions
	TokBrace           // {...} arrays and dicts
	TokBracket         // [...] command substitution
	TokVariable        // $var or ${var}
	TokAssign          // =
	TokPipe            // |
	TokPipeTap         // |>
	TokRedirectIn      // <
	TokRedirectOut     // >
	TokRedirectAppend  // >>
	TokRedirectErr     // 2>
	TokRedirectErrApp  // 2>>
	TokRedirectErrOut  // 2>&1
	TokPerlPrompt      // p{...}
	TokPerlRaw         // q{...}
	TokPerlInterp      // qq{...}
	TokPerlExec        // qx{...}
	TokPerlWords       // qw{...}
	TokWith            // with (for prompt options)
)

func (t TokenType) String() string {
	switch t {
	case TokEOF:
		return "EOF"
	case TokEOL:
		return "EOL"
	case TokWord:
		return "Word"
	case TokString:
		return "String"
	case TokParen:
		return "Paren(())"
	case TokBrace:
		return "Brace({})"
	case TokBracket:
		return "Bracket([])"
	case TokVariable:
		return "Variable"
	case TokAssign:
		return "Assign(=)"
	case TokPipe:
		return "Pipe(|)"
	case TokPipeTap:
		return "PipeTap(|>)"
	case TokRedirectIn:
		return "Redirect(<)"
	case TokRedirectOut:
		return "Redirect(>)"
	case TokRedirectAppend:
		return "Redirect(>>)"
	case TokRedirectErr:
		return "Redirect(2>)"
	case TokRedirectErrApp:
		return "Redirect(2>>)"
	case TokRedirectErrOut:
		return "Redirect(2>&1)"
	case TokPerlPrompt:
		return "PerlPrompt(p{})"
	case TokPerlRaw:
		return "PerlRaw(q{})"
	case TokPerlInterp:
		return "PerlInterp(qq{})"
	case TokPerlExec:
		return "PerlExec(qx{})"
	case TokPerlWords:
		return "PerlWords(qw{})"
	case TokWith:
		return "With"
	default:
		return "Unknown"
	}
}

// Token represents a single lexical token.
type Token struct {
	Type   TokenType
	Value  string
	Line   int
	Column int
}

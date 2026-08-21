package tests

import (
	"testing"
	"pcl/pkg/parser"
)

func TestPerlQuotingTokens(t *testing.T) {
	input := `x = p( explain this code )`
	tokens := parser.TokenizeAll(input)

	if len(tokens) < 3 {
		t.Fatalf("expected at least 3 tokens, got %d", len(tokens))
	}

	if tokens[0].Value != "x" || tokens[1].Type != parser.TokAssign || tokens[2].Type != parser.TokPerlPrompt {
		t.Fatalf("unexpected tokens: %v", tokens)
	}

	if tokens[2].Value != " explain this code " {
		t.Fatalf("unexpected prompt content: '%s'", tokens[2].Value)
	}
}

func TestPerlQuoteTypes(t *testing.T) {
	raw := `q(raw string \n no escape)`
	toks := parser.TokenizeAll(raw)
	if toks[0].Type != parser.TokPerlRaw {
		t.Fatalf("expected TokPerlRaw, got %s", toks[0].Type)
	}

	words := `qw(apple banana cherry)`
	toks = parser.TokenizeAll(words)
	if toks[0].Type != parser.TokPerlWords {
		t.Fatalf("expected TokPerlWords, got %s", toks[0].Type)
	}
}

func TestArrayAndDictLiterals(t *testing.T) {
	arrInput := `{ 10 20 30 }`
	toks := parser.TokenizeAll(arrInput)
	if toks[0].Type != parser.TokBrace {
		t.Fatalf("expected TokBrace for array literal, got %s", toks[0].Type)
	}
}

func TestMultilineCompleteness(t *testing.T) {
	// Incomplete paren block
	complete, missing := parser.IsCompleteCommand("proc greet () ( puts hello")
	if complete || missing != ")" {
		t.Fatalf("expected incomplete ')', got %v, %s", complete, missing)
	}

	// Complete paren block
	complete, _ = parser.IsCompleteCommand("proc greet () ( puts hello )")
	if !complete {
		t.Fatalf("expected complete block")
	}

	// Incomplete Perl prompt
	complete, missing = parser.IsCompleteCommand("x = p( explain this")
	if complete || missing != ")" {
		t.Fatalf("expected incomplete prompt ')', got %v, %s", complete, missing)
	}

	// Complete Perl prompt
	complete, _ = parser.IsCompleteCommand("x = p( explain this )")
	if !complete {
		t.Fatalf("expected complete prompt")
	}
}

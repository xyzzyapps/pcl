package parser

import (
	"fmt"
	"strings"
	"unicode"
)

// AccessOpType defines types of member or subscript operations.
type AccessOpType int

const (
	OpField AccessOpType = iota // .field
	OpIndex                     // [key] or [index]
	OpCall                      // .method(args...)
)

// AccessOp represents a single link in an access chain.
type AccessOp struct {
	Type   AccessOpType
	Name   string   // field or method name
	Index  string   // index key or expression
	Args   []string // arguments for method call
}

// AccessChain represents a complete UFCS / member access expression.
type AccessChain struct {
	Root string
	Ops  []AccessOp
}

// IsUFCSOrAccessExpr returns true if the token string looks like an access chain or method call.
func IsUFCSOrAccessExpr(expr string) bool {
	if expr == "" {
		return false
	}
	// Ignore simple bracket command like [puts hello]
	if strings.HasPrefix(expr, "[") && strings.HasSuffix(expr, "]") && !strings.Contains(expr, "].") && !strings.Contains(expr, "][") {
		return false
	}

	// 1. If it starts with $, like $x.response or $x["tools"]
	if strings.HasPrefix(expr, "$") && (strings.Contains(expr, ".") || strings.Contains(expr, "[")) {
		return true
	}

	// 2. If it contains method call parentheses, like msg.upper() or x.exec()
	if strings.Contains(expr, ".") && strings.Contains(expr, "(") && strings.Contains(expr, ")") {
		return true
	}

	// 3. If it contains subscript indexing, like x["tools"] or x[0]
	if strings.Contains(expr, "[") && strings.Contains(expr, "]") {
		return true
	}

	// 4. If it has dot and ends with standard property name
	if strings.HasSuffix(expr, ".exec") || strings.HasSuffix(expr, ".response") || strings.HasSuffix(expr, ".text") || strings.HasSuffix(expr, ".tools") || strings.HasSuffix(expr, ".json") || strings.HasSuffix(expr, ".len") {
		return true
	}

	return false
}

// ParseAccessChain parses expressions like x["tools"][0].exec or $x.response into an AccessChain.
func ParseAccessChain(expr string) (*AccessChain, error) {
	runes := []rune(expr)
	n := len(runes)
	if n == 0 {
		return nil, fmt.Errorf("empty access expression")
	}

	i := 0
	// 1. Parse Root (e.g. $x, x, "hello", etc.)
	var rootSb strings.Builder
	for i < n {
		ch := runes[i]
		if ch == '.' || ch == '[' {
			break
		}
		rootSb.WriteRune(ch)
		i++
	}

	root := rootSb.String()
	if root == "" {
		return nil, fmt.Errorf("invalid access expression: missing root in %s", expr)
	}

	chain := &AccessChain{
		Root: root,
		Ops:  make([]AccessOp, 0),
	}

	// 2. Parse Operations (.field, .method(args), [index])
	for i < n {
		ch := runes[i]

		if ch == '.' {
			i++ // skip .
			// Read method or field name
			var nameSb strings.Builder
			for i < n && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				nameSb.WriteRune(runes[i])
				i++
			}
			name := nameSb.String()
			if name == "" {
				return nil, fmt.Errorf("expected member name after '.' at %d in %s", i, expr)
			}

			// Check if followed by method call ()
			if i < n && runes[i] == '(' {
				i++ // skip (
				args, nextPos, err := parseMethodArgs(runes, i)
				if err != nil {
					return nil, err
				}
				i = nextPos
				chain.Ops = append(chain.Ops, AccessOp{
					Type: OpCall,
					Name: name,
					Args: args,
				})
			} else {
				// Field or zero-arg method property
				chain.Ops = append(chain.Ops, AccessOp{
					Type: OpField,
					Name: name,
				})
			}
			continue
		}

		if ch == '[' {
			i++ // skip [
			var idxSb strings.Builder
			depth := 1
			for i < n {
				c := runes[i]
				if c == '[' {
					depth++
				} else if c == ']' {
					depth--
					if depth == 0 {
						i++ // skip closing ]
						break
					}
				}
				idxSb.WriteRune(c)
				i++
			}
			idxStr := strings.TrimSpace(idxSb.String())
			// Unquote if it's "key" or 'key'
			if (strings.HasPrefix(idxStr, `"`) && strings.HasSuffix(idxStr, `"`)) ||
				(strings.HasPrefix(idxStr, `'`) && strings.HasSuffix(idxStr, `'`)) {
				idxStr = idxStr[1 : len(idxStr)-1]
			}
			chain.Ops = append(chain.Ops, AccessOp{
				Type:  OpIndex,
				Index: idxStr,
			})
			continue
		}

		return nil, fmt.Errorf("unexpected character '%c' in access expression %s", ch, expr)
	}

	return chain, nil
}

func parseMethodArgs(runes []rune, start int) ([]string, int, error) {
	n := len(runes)
	i := start
	var args []string
	var currentArg strings.Builder
	inQuote := false
	quoteChar := rune(0)
	depth := 0

	for i < n {
		ch := runes[i]

		if inQuote {
			if ch == '\\' && i+1 < n {
				currentArg.WriteRune(ch)
				i++
				currentArg.WriteRune(runes[i])
				i++
				continue
			}
			if ch == quoteChar {
				inQuote = false
			}
			currentArg.WriteRune(ch)
			i++
			continue
		}

		if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
			currentArg.WriteRune(ch)
			i++
			continue
		}

		if ch == '(' || ch == '{' || ch == '[' {
			depth++
			currentArg.WriteRune(ch)
			i++
			continue
		}

		if ch == ')' || ch == '}' || ch == ']' {
			if depth > 0 {
				depth--
				currentArg.WriteRune(ch)
				i++
				continue
			}
			if ch == ')' {
				// End of argument list
				trimmed := strings.TrimSpace(currentArg.String())
				if trimmed != "" {
					args = append(args, unquoteString(trimmed))
				}
				i++ // skip closing )
				return args, i, nil
			}
		}

		if ch == ',' && depth == 0 {
			trimmed := strings.TrimSpace(currentArg.String())
			if trimmed != "" {
				args = append(args, unquoteString(trimmed))
			}
			currentArg.Reset()
			i++
			continue
		}

		currentArg.WriteRune(ch)
		i++
	}

	return nil, i, fmt.Errorf("unclosed parenthesis in method call")
}

func unquoteString(s string) string {
	s = strings.TrimSpace(s)
	// Handle escaped quotes \"...\"
	if (strings.HasPrefix(s, `\"`) && strings.HasSuffix(s, `\"`)) ||
		(strings.HasPrefix(s, `\'`) && strings.HasSuffix(s, `\'`)) {
		if len(s) >= 4 {
			s = s[2 : len(s)-2]
		}
	}
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) ||
		(strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		if len(s) >= 2 {
			s = s[1 : len(s)-1]
		}
	}
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\r`, "\r")
	s = strings.ReplaceAll(s, `\"`, "\"")
	s = strings.ReplaceAll(s, `\'`, "'")
	s = strings.ReplaceAll(s, `\\`, "\\")
	return s
}

package interp

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"pcl/pkg/ai"
	"pcl/pkg/core"
	"pcl/pkg/parser"
	"pcl/pkg/services"
)

// EvalWord performs variable ($var) and command ([cmd]) substitutions on a token.
func (in *Interpreter) EvalWord(tok parser.Token) (*core.Value, error) {
	switch tok.Type {
	case parser.TokString:
		// Interpolate $var and [cmd] inside "..."
		expanded, err := in.interpolateString(tok.Value)
		if err != nil {
			return nil, err
		}
		return core.NewString(expanded), nil

	case parser.TokParen:
		// ( ... ) returns the parenthesized expression/block string for control structures and commands.
		return core.NewString(tok.Value), nil

	case parser.TokBrace:
		// { ... } Array and Dict literals
		return in.parseArrayOrDictLiteral(tok.Value)

	case parser.TokBracket:
		// Command substitution [cmd args...]
		return in.Eval(tok.Value)

	case parser.TokVariable:
		varName := strings.TrimPrefix(tok.Value, "$")
		// Check for UFCS/Access on variable: $x["response"] or $x.response
		if parser.IsUFCSOrAccessExpr(tok.Value) {
			chain, err := parser.ParseAccessChain(tok.Value)
			if err == nil {
				return in.EvalUFCSChain(chain)
			}
		}
		// Check if surrounded with ${var}
		if strings.HasPrefix(varName, "{") && strings.HasSuffix(varName, "}") {
			varName = varName[1 : len(varName)-1]
		}
		if val, ok := in.Scope.Get(varName); ok {
			return val, nil
		}
		return core.NewNull(), nil

	case parser.TokPerlPrompt:
		// p{...} or p!{...}
		isStream := strings.HasPrefix(tok.Value, "!")
		promptBody := strings.TrimPrefix(tok.Value, "!")
		expanded, err := in.interpolateString(promptBody)
		if err != nil {
			return nil, err
		}
		return in.evalPrompt(expanded, isStream, "")

	case parser.TokPerlRaw:
		// q{...} raw string
		return core.NewString(tok.Value), nil

	case parser.TokPerlInterp:
		// qq{...} interpolated string
		expanded, err := in.interpolateString(tok.Value)
		if err != nil {
			return nil, err
		}
		return core.NewString(expanded), nil

	case parser.TokPerlExec:
		// qx{...} execute command and capture stdout
		var outBuf bytes.Buffer
		savedOut := in.Services.IO().Stdout()
		in.Services.SetIO(services.NewCustomIOService(in.Services.IO().Stdin(), &outBuf, in.Services.IO().Stderr()))
		_, err := in.Eval(tok.Value)
		in.Services.SetIO(services.NewCustomIOService(in.Services.IO().Stdin(), savedOut, in.Services.IO().Stderr()))
		if err != nil {
			return nil, err
		}
		return core.NewString(strings.TrimRight(outBuf.String(), "\r\n")), nil

	case parser.TokPerlWords:
		// qw{...} word list
		words := strings.Fields(tok.Value)
		items := make([]*core.Value, len(words))
		for i, w := range words {
			items[i] = core.NewString(w)
		}
		return core.NewList(items...), nil

	case parser.TokWord:
		// Check for single quoted string: '...'
		if strings.HasPrefix(tok.Value, "'") && strings.HasSuffix(tok.Value, "'") && len(tok.Value) >= 2 {
			return core.NewString(tok.Value[1 : len(tok.Value)-1]), nil
		}

		// Check for double quoted string: "..."
		if strings.HasPrefix(tok.Value, `"`) && strings.HasSuffix(tok.Value, `"`) && len(tok.Value) >= 2 {
			expanded, err := in.interpolateString(tok.Value[1 : len(tok.Value)-1])
			if err != nil {
				return nil, err
			}
			return core.NewString(expanded), nil
		}

		// Check if it's a UFCS / access expression like x["response"] or x.tools[0].exec
		if parser.IsUFCSOrAccessExpr(tok.Value) {
			chain, err := parser.ParseAccessChain(tok.Value)
			if err == nil {
				return in.EvalUFCSChain(chain)
			}
		}

		// Check for variable prefix
		if strings.HasPrefix(tok.Value, "$") {
			return in.EvalWord(parser.Token{Type: parser.TokVariable, Value: tok.Value})
		}

		return core.NewString(tok.Value), nil

	default:
		return core.NewString(tok.Value), nil
	}
}

func (in *Interpreter) interpolateString(input string) (string, error) {
	runes := []rune(input)
	n := len(runes)
	var sb strings.Builder

	i := 0
	for i < n {
		ch := runes[i]

		// Escapes
		if ch == '\\' && i+1 < n {
			next := runes[i+1]
			if next == '$' || next == '[' || next == '"' || next == '\\' {
				sb.WriteRune(next)
				i += 2
				continue
			}
		}

		// Variable substitution: $var or ${var} or $x.response or $x["key"]
		if ch == '$' && i+1 < n {
			i++ // skip $
			var varExpr strings.Builder
			varExpr.WriteRune('$')
			inQuote := false
			var quoteChar rune
			parenDepth := 0
			bracketDepth := 0

			if i < n && runes[i] == '{' {
				// ${varName} format
				i++ // skip {
				for i < n && runes[i] != '}' {
					varExpr.WriteRune(runes[i])
					i++
				}
				if i < n && runes[i] == '}' {
					i++ // skip }
				}
			} else {
				for i < n {
					c := runes[i]
					if inQuote {
						varExpr.WriteRune(c)
						if c == '\\' && i+1 < n {
							i++
							varExpr.WriteRune(runes[i])
							i++
							continue
						}
						if c == quoteChar {
							inQuote = false
							quoteChar = rune(0)
						}
						i++
						continue
					}
					if (c == '"' || c == '\'') && (parenDepth > 0 || bracketDepth > 0) {
						inQuote = true
						quoteChar = c
						varExpr.WriteRune(c)
						i++
						continue
					}
					if c == '(' {
						parenDepth++
					} else if c == ')' {
						if parenDepth > 0 {
							parenDepth--
						}
					} else if c == '[' {
						bracketDepth++
					} else if c == ']' {
						if bracketDepth > 0 {
							bracketDepth--
						}
					} else if parenDepth == 0 && bracketDepth == 0 {
						if !isIdentRune(c) && c != '.' && c != '_' {
							break
						}
					}
					varExpr.WriteRune(c)
					i++

					if parenDepth == 0 && bracketDepth == 0 && (c == ')' || c == ']') {
						if i < n && (runes[i] == '.' || runes[i] == '[') {
							continue
						}
						break
					}
				}
			}

			exprStr := varExpr.String()
			if parser.IsUFCSOrAccessExpr(exprStr) {
				chain, err := parser.ParseAccessChain(exprStr)
				if err == nil {
					val, err := in.EvalUFCSChain(chain)
					if err == nil {
						sb.WriteString(val.String())
						continue
					}
				}
			}

			varName := strings.TrimPrefix(exprStr, "$")
			if val, ok := in.Scope.Get(varName); ok {
				sb.WriteString(val.String())
			}
			continue
		}

		// Command substitution: [cmd args...]
		if ch == '[' {
			i++ // skip [
			var cmdSb strings.Builder
			depth := 1
			for i < n {
				c := runes[i]
				if c == '[' {
					depth++
				} else if c == ']' {
					depth--
					if depth == 0 {
						i++
						break
					}
				}
				cmdSb.WriteRune(c)
				i++
			}
			val, err := in.Eval(cmdSb.String())
			if err != nil {
				return "", err
			}
			sb.WriteString(val.String())
			continue
		}

		sb.WriteRune(ch)
		i++
	}

	return sb.String(), nil
}

func isIdentRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func (in *Interpreter) evalPrompt(promptText string, stream bool, promptOpt string) (*core.Value, error) {
	opts := ai.DefaultAgentOptions()
	opts.Model = in.Services.Config().Get("model")
	opts.SystemPrompt = in.Services.Config().Get("system_prompt")
	if sw := in.streamDest(stream); sw != nil {
		opts.StreamWriter = sw
	}

	followTools := true
	if promptOpt != "" {
		optVal, optErr := in.parseArrayOrDictLiteral(promptOpt)
		if optErr == nil && optVal.Type() == core.TypeDict {
			if ag, exists := optVal.DictVal["agent"]; exists && !ag.IsTruthy() {
				followTools = false
			}
			if mt, exists := optVal.DictVal["max_turns"]; exists {
				if turns, err := mt.AsInt(); err == nil && turns > 0 {
					opts.MaxTurns = int(turns)
					if turns == 1 {
						followTools = false
					}
				}
			}
		}
	}

	if followTools {
		resp, err := ai.RunReActLoop(in.Ctx, in.Services.AI(), in, promptText, opts)
		if err != nil {
			return nil, fmt.Errorf("agent execution error: %w", err)
		}
		return core.NewResponse(resp), nil
	}

	req := &services.AIRequest{
		Prompt:       promptText,
		SystemPrompt: in.Services.Config().Get("system_prompt"),
		Model:        in.Services.Config().Get("model"),
		Temperature:  0.7,
		Tools:        in.Services.AI().ListTools(),
	}

	if tempStr := in.Services.Config().Get("temperature"); tempStr != "" {
		if t, err := strconv.ParseFloat(tempStr, 64); err == nil {
			req.Temperature = t
		}
	}

	if sw := in.streamDest(stream); sw != nil {
		req.StreamWriter = sw
	}

	resp, err := in.Services.AI().Prompt(in.Ctx, req)
	if err != nil {
		return nil, fmt.Errorf("AI prompt execution error: %w", err)
	}

	return core.NewResponse(resp), nil
}

func (in *Interpreter) streamDest(explicit bool) io.Writer {
	if in.StreamWriter != nil {
		return in.StreamWriter
	}
	if explicit && in.Services != nil && in.Services.IO() != nil {
		return in.Services.IO().Stdout()
	}
	return nil
}

// EvalExpr evaluates simple arithmetic and logical expressions for expr / if / while.
func (in *Interpreter) EvalExpr(exprStr string) (*core.Value, error) {
	exprStr = strings.TrimSpace(exprStr)
	exprStr = strings.TrimPrefix(exprStr, "(")
	exprStr = strings.TrimSuffix(exprStr, ")")
	exprStr = strings.TrimSpace(exprStr)
	// First interpolate variables and command substitutions
	interpolated, err := in.interpolateString(exprStr)
	if err != nil {
		return nil, err
	}

	return evalSimpleExpression(interpolated)
}

func evalSimpleExpression(s string) (*core.Value, error) {
	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return core.NewInt(0), nil
	}

	if len(tokens) == 1 {
		tok := tokens[0]
		if tok == "true" {
			return core.NewBool(true), nil
		}
		if tok == "false" {
			return core.NewBool(false), nil
		}
		if i, err := strconv.ParseInt(tok, 0, 64); err == nil {
			return core.NewInt(i), nil
		}
		if f, err := strconv.ParseFloat(tok, 64); err == nil {
			return core.NewFloat(f), nil
		}
		return core.NewString(tok), nil
	}

	// Binary operations: a op b
	if len(tokens) == 3 {
		a, op, b := tokens[0], tokens[1], tokens[2]
		aInt, aIntErr := strconv.ParseInt(a, 0, 64)
		bInt, bIntErr := strconv.ParseInt(b, 0, 64)

		if aIntErr == nil && bIntErr == nil {
			switch op {
			case "+":
				return core.NewInt(aInt + bInt), nil
			case "-":
				return core.NewInt(aInt - bInt), nil
			case "*":
				return core.NewInt(aInt * bInt), nil
			case "/":
				if bInt == 0 {
					return nil, fmt.Errorf("division by zero")
				}
				return core.NewInt(aInt / bInt), nil
			case "%":
				if bInt == 0 {
					return nil, fmt.Errorf("modulo by zero")
				}
				return core.NewInt(aInt % bInt), nil
			case "==":
				return core.NewBool(aInt == bInt), nil
			case "!=":
				return core.NewBool(aInt != bInt), nil
			case "<":
				return core.NewBool(aInt < bInt), nil
			case "<=":
				return core.NewBool(aInt <= bInt), nil
			case ">":
				return core.NewBool(aInt > bInt), nil
			case ">=":
				return core.NewBool(aInt >= bInt), nil
			case "&&":
				return core.NewBool((aInt != 0) && (bInt != 0)), nil
			case "||":
				return core.NewBool((aInt != 0) || (bInt != 0)), nil
			}
		}

		isATrue := (a == "1" || a == "true") || (aIntErr == nil && aInt != 0)
		isBTrue := (b == "1" || b == "true") || (bIntErr == nil && bInt != 0)

		if op == "&&" {
			return core.NewBool(isATrue && isBTrue), nil
		}
		if op == "||" {
			return core.NewBool(isATrue || isBTrue), nil
		}

		// String comparison
		switch op {
		case "==":
			return core.NewBool(a == b), nil
		case "!=":
			return core.NewBool(a != b), nil
		}
	}

	return core.NewString(s), nil
}

func isArithmeticOrComparisonExpr(s string) bool {
	s = strings.TrimSpace(s)
	// Check for operators
	ops := []string{"==", "!=", "<=", ">=", "&&", "||", " + ", " - ", " * ", " / ", " % ", " < ", " > "}
	for _, op := range ops {
		if strings.Contains(s, op) {
			return true
		}
	}
	return false
}

func (in *Interpreter) parseArrayOrDictLiteral(s string) (*core.Value, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return core.NewList(), nil
	}

	// Check if key: value dictionary
	if strings.Contains(s, ":") {
		// Clean outer braces if present
		inner := strings.TrimPrefix(s, "{")
		inner = strings.TrimSuffix(inner, "}")

		// Split pairs by comma
		pairs := strings.Split(inner, ",")
		dictMap := make(map[string]*core.Value)
		validDict := true

		for _, pair := range pairs {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			colonIdx := strings.Index(pair, ":")
			if colonIdx == -1 {
				validDict = false
				break
			}
			k := strings.TrimSpace(pair[:colonIdx])
			v := strings.TrimSpace(pair[colonIdx+1:])
			k = strings.Trim(k, "\"'\t ")
			v = strings.Trim(v, "\"'\t ")
			dictMap[k] = core.NewString(v)
		}

		if validDict && len(dictMap) > 0 {
			return core.NewDict(dictMap), nil
		}
	}

	// Parse elements as list
	tokens := parser.TokenizeAll(s)
	items := make([]*core.Value, 0)
	for _, tok := range tokens {
		if tok.Type == parser.TokEOF || tok.Type == parser.TokEOL {
			continue
		}
		val, err := in.EvalWord(tok)
		if err != nil {
			items = append(items, core.NewString(tok.Value))
		} else {
			items = append(items, val)
		}
	}

	return core.NewList(items...), nil
}

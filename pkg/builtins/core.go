package builtins

import (
	"errors"
	"fmt"
	"strings"
	"pcl/pkg/core"
	"pcl/pkg/interp"
)

// RegisterCoreBuiltins registers all core Tcl language primitives.
func RegisterCoreBuiltins(in *interp.Interpreter) {
	// set <varName> ?<value>?
	in.RegisterBuiltin("set", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("wrong # args: should be \"set varName ?newValue?\"")
		}
		varName := strings.TrimPrefix(args[0].String(), "$")
		if len(args) == 1 {
			val, ok := in.Scope.Get(varName)
			if !ok {
				return nil, fmt.Errorf("can't read \"%s\": no such variable", varName)
			}
			return val, nil
		}
		val := args[1]
		valStr := val.String()
		if strings.ContainsAny(valStr, "+-*/%<>=") {
			if exprVal, err := in.EvalExpr(valStr); err == nil {
				val = exprVal
			}
		}
		in.Scope.Set(varName, val)
		return val, nil
	})

	// unset <varName...>
	in.RegisterBuiltin("unset", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		for _, arg := range args {
			varName := strings.TrimPrefix(arg.String(), "$")
			in.Scope.Unset(varName)
		}
		return core.NewNull(), nil
	})

	// proc <name> <params> <body>
	in.RegisterBuiltin("proc", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) < 3 {
			return nil, fmt.Errorf("wrong # args: should be \"proc name args body\"")
		}
		name := args[0].String()
		paramStr := strings.Trim(args[1].String(), "()")
		body := args[2].String()

		params := strings.Fields(paramStr)
		in.Procs[name] = &interp.ProcDef{
			Name:   name,
			Params: params,
			Body:   body,
		}
		return core.NewNull(), nil
	})

	// return ?<value>?
	in.RegisterBuiltin("return", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return core.NewNull(), nil
		}
		val := args[0]
		valStr := val.String()
		if strings.ContainsAny(valStr, "+-*/%<>=") {
			if exprVal, err := in.EvalExpr(valStr); err == nil {
				val = exprVal
			}
		}
		return val, nil
	})

	// puts ?-nonewline? ?channelId? <string>
	in.RegisterBuiltin("puts", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("wrong # args: should be \"puts ?-nonewline? string\"")
		}

		noNewline := false
		strIdx := 0
		if args[0].String() == "-nonewline" {
			noNewline = true
			strIdx = 1
		}

		if strIdx >= len(args) {
			return nil, fmt.Errorf("wrong # args: should be \"puts ?-nonewline? string\"")
		}

		text := args[strIdx].String()
		if noNewline {
			in.Services.IO().Print(text)
		} else {
			in.Services.IO().Println(text)
		}

		return core.NewNull(), nil
	})

	// gets ?channelId? ?varName?
	in.RegisterBuiltin("gets", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		line, err := in.Services.IO().ReadLine()
		if err != nil {
			return core.NewString(""), nil
		}
		if len(args) > 0 {
			varName := strings.TrimPrefix(args[0].String(), "$")
			in.Scope.Set(varName, core.NewString(line))
			return core.NewInt(int64(len(line))), nil
		}
		return core.NewString(line), nil
	})

	// if <expr> ?then? <body> ?elseif <expr> ?then? <body>...? ?else? ?<body>?
	in.RegisterBuiltin("if", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("wrong # args: should be \"if expr1 ?then? body1 ... ?else? ?body2?\"")
		}

		i := 0
		for i < len(args) {
			condVal, err := in.EvalExpr(args[i].String())
			if err != nil {
				return nil, err
			}
			i++
			if i < len(args) && args[i].String() == "then" {
				i++
			}
			if i >= len(args) {
				return nil, fmt.Errorf("missing body in if statement")
			}
			body := args[i].String()
			i++

			if condVal.AsBool() {
				return in.Eval(body)
			}

			// Check for elseif or else
			if i < len(args) {
				if args[i].String() == "elseif" {
					i++
					continue
				}
				if args[i].String() == "else" {
					i++
					if i < len(args) {
						return in.Eval(args[i].String())
					}
					break
				}
				// Default trailing body as else
				return in.Eval(args[i].String())
			}
		}

		return core.NewNull(), nil
	})

	// while <test> <body>
	in.RegisterBuiltin("while", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("wrong # args: should be \"while test body\"")
		}
		testExpr := args[0].String()
		body := args[1].String()

		var lastVal *core.Value = core.NewNull()
		for {
			condVal, err := in.EvalExpr(testExpr)
			if err != nil {
				return nil, err
			}
			if !condVal.AsBool() {
				break
			}
			res, err := in.Eval(body)
			if err != nil {
				if errors.Is(err, core.ErrBreak) {
					return lastVal, nil
				}
				if errors.Is(err, core.ErrContinue) {
					continue
				}
				return nil, err
			}
			lastVal = res
		}

		return lastVal, nil
	})

	// break
	in.RegisterBuiltin("break", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		return nil, core.ErrBreak
	})

	// continue
	in.RegisterBuiltin("continue", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		return nil, core.ErrContinue
	})

	// for <start> <test> <next> <body>
	in.RegisterBuiltin("for", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) != 4 {
			return nil, fmt.Errorf("wrong # args: should be \"for start test next body\"")
		}
		start := args[0].String()
		testExpr := args[1].String()
		next := args[2].String()
		body := args[3].String()

		if _, err := in.Eval(start); err != nil {
			return nil, err
		}

		var lastVal *core.Value = core.NewNull()
		for {
			condVal, err := in.EvalExpr(testExpr)
			if err != nil {
				return nil, err
			}
			if !condVal.AsBool() {
				break
			}
			res, err := in.Eval(body)
			if err != nil {
				if errors.Is(err, core.ErrBreak) {
					return lastVal, nil
				}
				if errors.Is(err, core.ErrContinue) {
					if _, nerr := in.Eval(next); nerr != nil {
						return nil, nerr
					}
					continue
				}
				return nil, err
			}
			lastVal = res

			if _, err := in.Eval(next); err != nil {
				return nil, err
			}
		}

		return lastVal, nil
	})

	// foreach <varName> <list> <body>
	in.RegisterBuiltin("foreach", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) != 3 {
			return nil, fmt.Errorf("wrong # args: should be \"foreach varname list body\"")
		}
		varName := strings.TrimPrefix(args[0].String(), "$")
		listVal := args[1]
		body := args[2].String()

		var items []*core.Value
		if listVal.Type() == core.TypeList {
			items = listVal.ListVal
		} else {
			fields := strings.Fields(listVal.String())
			items = make([]*core.Value, len(fields))
			for i, f := range fields {
				items[i] = core.NewString(f)
			}
		}

		var lastVal *core.Value = core.NewNull()
		for _, item := range items {
			in.Scope.Set(varName, item)
			res, err := in.Eval(body)
			if err != nil {
				if errors.Is(err, core.ErrBreak) {
					return lastVal, nil
				}
				if errors.Is(err, core.ErrContinue) {
					continue
				}
				return nil, err
			}
			lastVal = res
		}

		return lastVal, nil
	})

	// expr <arg...>
	in.RegisterBuiltin("expr", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		var sb strings.Builder
		for i, a := range args {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(a.String())
		}
		return in.EvalExpr(sb.String())
	})

	// source <fileName>
	in.RegisterBuiltin("source", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong # args: should be \"source fileName\"")
		}
		filePath := args[0].String()
		data, err := in.Services.FS().ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("could not read file %s: %w", filePath, err)
		}
		return in.Eval(string(data))
	})

	// list ?<arg...>?
	in.RegisterBuiltin("list", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		return core.NewList(args...), nil
	})

	// lindex <list> <index>
	in.RegisterBuiltin("lindex", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("wrong # args: should be \"lindex list index\"")
		}
		return args[0].Index(args[1])
	})

	// llength <list>
	in.RegisterBuiltin("llength", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong # args: should be \"llength list\"")
		}
		if args[0].Type() == core.TypeList {
			return core.NewInt(int64(len(args[0].ListVal))), nil
		}
		fields := strings.Fields(args[0].String())
		return core.NewInt(int64(len(fields))), nil
	})

	// lappend <varName> ?<value...>?
	in.RegisterBuiltin("lappend", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("wrong # args: should be \"lappend varName ?value...?\"")
		}
		varName := strings.TrimPrefix(args[0].String(), "$")
		existing, ok := in.Scope.Get(varName)
		if !ok || existing.Type() != core.TypeList {
			existing = core.NewList()
		}
		existing.ListVal = append(existing.ListVal, args[1:]...)
		in.Scope.Set(varName, existing)
		return existing, nil
	})
}

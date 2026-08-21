package interp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"pcl/pkg/core"
	"pcl/pkg/ffi"
	"pcl/pkg/parser"
	"pcl/pkg/shell"
)

// EvalUFCSChain evaluates a parsed AccessChain on the interpreter runtime.
func (in *Interpreter) EvalUFCSChain(chain *parser.AccessChain) (*core.Value, error) {
	if chain == nil {
		return core.NewNull(), nil
	}

	// 1. Resolve root value
	rootName := chain.Root
	var current *core.Value

	// Check if root is variable reference
	varName := strings.TrimPrefix(rootName, "$")
	if val, ok := in.Scope.Get(varName); ok {
		current = val
	} else if strings.HasPrefix(rootName, `"`) && strings.HasSuffix(rootName, `"`) {
		current = core.NewString(rootName[1 : len(rootName)-1])
	} else {
		// Try interpreting as string literal
		current = core.NewString(rootName)
	}

	// 2. Evaluate chain operations sequentially
	for _, op := range chain.Ops {
		var err error
		current, err = in.evalAccessOp(current, op)
		if err != nil {
			return nil, err
		}
	}

	return current, nil
}

func (in *Interpreter) evalAccessOp(target *core.Value, op parser.AccessOp) (*core.Value, error) {
	if target == nil {
		target = core.NewNull()
	}

	switch op.Type {
	case parser.OpIndex:
		// Index by key or integer
		idxStr := op.Index
		// Check if index is variable reference: $var
		if strings.HasPrefix(idxStr, "$") {
			if v, ok := in.Scope.Get(strings.TrimPrefix(idxStr, "$")); ok {
				idxStr = v.String()
			}
		}
		return target.Index(core.NewString(idxStr))

	case parser.OpField:
		// Field access: .field or zero-argument method call like .exec
		fieldName := op.Name

		// 1. Check built-in properties
		if target.Type() == core.TypeResponse {
			switch strings.ToLower(fieldName) {
			case "response", "text":
				return core.NewString(target.RespVal.Text), nil
			case "reasoning", "thought", "thinking":
				return core.NewString(target.RespVal.Reasoning), nil
			case "tools", "tool_calls":
				items := make([]*core.Value, len(target.RespVal.ToolCalls))
				for i, tc := range target.RespVal.ToolCalls {
					items[i] = core.NewToolCall(tc)
				}
				return core.NewList(items...), nil
			case "usage":
				return target.Index(core.NewString("usage"))
			case "model":
				return core.NewString(target.RespVal.Model), nil
			}
		}

		if target.Type() == core.TypeToolCall {
			switch strings.ToLower(fieldName) {
			case "exec":
				return in.execToolCall(target.ToolVal)
			case "id":
				return core.NewString(target.ToolVal.ID), nil
			case "name":
				return core.NewString(target.ToolVal.Name), nil
			case "arguments", "args":
				return target.Index(core.NewString("arguments"))
			}
		}

		if target.Type() == core.TypeDict {
			if val, ok := target.DictVal[fieldName]; ok {
				return val, nil
			}
		}

		// 2. Check UFCS zero-arg method call (e.g. .len, .upper, .json)
		return in.evalUFCSMethod(target, fieldName, nil)

	case parser.OpCall:
		// Method invocation: .method(arg1, arg2...)
		evaluatedArgs := make([]*core.Value, len(op.Args))
		for i, argStr := range op.Args {
			if strings.HasPrefix(argStr, "$") {
				if v, ok := in.Scope.Get(strings.TrimPrefix(argStr, "$")); ok {
					evaluatedArgs[i] = v
					continue
				}
			}
			evaluatedArgs[i] = core.NewString(argStr)
		}

		return in.evalUFCSMethod(target, op.Name, evaluatedArgs)

	default:
		return nil, fmt.Errorf("unknown access operation type")
	}
}

func (in *Interpreter) evalUFCSMethod(target *core.Value, methodName string, args []*core.Value) (*core.Value, error) {
	name := strings.ToLower(methodName)

	switch name {
	case "len", "length":
		switch target.Type() {
		case core.TypeString:
			return core.NewInt(int64(core.RuneCount(target.StrVal))), nil
		case core.TypeList:
			return core.NewInt(int64(len(target.ListVal))), nil
		case core.TypeDict:
			return core.NewInt(int64(len(target.DictVal))), nil
		default:
			return core.NewInt(int64(core.RuneCount(target.String()))), nil
		}

	case "exec":
		if target.Type() == core.TypeToolCall {
			return in.execToolCall(target.ToolVal)
		}
		// Execute target string as shell/PCL command
		return in.Eval(target.String())

	case "json":
		if target.Type() == core.TypeResponse || target.Type() == core.TypeDict || target.Type() == core.TypeList || target.Type() == core.TypeToolCall {
			b, err := json.MarshalIndent(target.ToNative(), "", "  ")
			if err != nil {
				return nil, err
			}
			return core.NewString(string(b)), nil
		}
		// Parse string as JSON
		jsonStr := target.String()
		var parsed interface{}
		err := json.Unmarshal([]byte(jsonStr), &parsed)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		return core.FromNative(parsed), nil

	case "upper":
		return core.NewString(strings.ToUpper(target.String())), nil

	case "vim", "edit":
		return in.handleVimEdit(target, args)

	case "lower":
		return core.NewString(strings.ToLower(target.String())), nil

	case "trim":
		return core.NewString(strings.TrimSpace(target.String())), nil

	case "split":
		sep := " "
		if len(args) > 0 {
			sep = args[0].String()
		}
		parts := strings.Split(target.String(), sep)
		items := make([]*core.Value, len(parts))
		for i, p := range parts {
			items[i] = core.NewString(p)
		}
		return core.NewList(items...), nil

	case "join":
		if target.Type() != core.TypeList {
			return nil, fmt.Errorf("join requires list target, got %s", target.Type())
		}
		sep := " "
		if len(args) > 0 {
			sep = args[0].String()
		}
		parts := make([]string, len(target.ListVal))
		for i, el := range target.ListVal {
			parts[i] = el.String()
		}
		return core.NewString(strings.Join(parts, sep)), nil

	case "steps":
		if target.Type() == core.TypeResponse {
			return target.Index(core.NewString("steps"))
		}
		return core.NewList(), nil

	case "tools_used":
		if target.Type() == core.TypeResponse {
			return target.Index(core.NewString("tools_used"))
		}
		return core.NewList(), nil

	case "files":
		if target.Type() == core.TypeResponse {
			filePaths := extractAllFilesFromResponse(target.RespVal)
			items := make([]*core.Value, len(filePaths))
			for i, fp := range filePaths {
				items[i] = core.NewString(fp)
			}
			return core.NewList(items...), nil
		}
		if target.Type() == core.TypeToolCall {
			var files []string
			tc := target.ToolVal
			for _, k := range []string{"path", "filename", "file", "filepath", "target"} {
				if v, ok := tc.Arguments[k]; ok {
					files = append(files, fmt.Sprintf("%v", v))
				}
			}
			items := make([]*core.Value, len(files))
			for i, fp := range files {
				items[i] = core.NewString(fp)
			}
			return core.NewList(items...), nil
		}
		if target.Type() == core.TypeList {
			return target, nil
		}
		return core.NewList(), nil

	case "keys":
		if target.Type() == core.TypeResponse {
			return core.NewList(
				core.NewString("response"),
				core.NewString("reasoning"),
				core.NewString("model"),
				core.NewString("tools"),
				core.NewString("steps"),
				core.NewString("tools_used"),
				core.NewString("files"),
				core.NewString("usage"),
			), nil
		}
		if target.Type() == core.TypeToolCall {
			return core.NewList(
				core.NewString("id"),
				core.NewString("name"),
				core.NewString("arguments"),
			), nil
		}
		if target.Type() != core.TypeDict {
			return nil, fmt.Errorf("keys requires dict target, got %s", target.Type())
		}
		keys := make([]*core.Value, 0, len(target.DictVal))
		for k := range target.DictVal {
			keys = append(keys, core.NewString(k))
		}
		return core.NewList(keys...), nil

	case "values":
		if target.Type() != core.TypeDict {
			return nil, fmt.Errorf("values requires dict target, got %s", target.Type())
		}
		vals := make([]*core.Value, 0, len(target.DictVal))
		for _, v := range target.DictVal {
			vals = append(vals, v)
		}
		return core.NewList(vals...), nil

	// Fluent File Test & Path Methods
	case "exists":
		path := target.String()
		_, err := os.Stat(path)
		return core.NewBool(err == nil), nil

	case "is_file", "isfile":
		path := target.String()
		st, err := os.Stat(path)
		return core.NewBool(err == nil && !st.IsDir()), nil

	case "is_dir", "isdir":
		path := target.String()
		st, err := os.Stat(path)
		return core.NewBool(err == nil && st.IsDir()), nil

	case "size":
		path := target.String()
		st, err := os.Stat(path)
		if err != nil {
			return core.NewInt(0), err
		}
		return core.NewInt(st.Size()), nil

	case "base", "basename":
		return core.NewString(filepath.Base(target.String())), nil

	case "ext":
		return core.NewString(filepath.Ext(target.String())), nil

	case "dir", "dirname":
		return core.NewString(filepath.Dir(target.String())), nil

	case "abs":
		absPath, err := filepath.Abs(target.String())
		if err != nil {
			return core.NewString(target.String()), nil
		}
		return core.NewString(absPath), nil

	case "read":
		data, err := os.ReadFile(target.String())
		if err != nil {
			return nil, err
		}
		return core.NewString(string(data)), nil

	case "write":
		content := ""
		if len(args) > 0 {
			content = args[0].String()
		}
		err := os.WriteFile(target.String(), []byte(content), 0644)
		if err != nil {
			return nil, err
		}
		return core.NewBool(true), nil

	case "append":
		content := ""
		if len(args) > 0 {
			content = args[0].String()
		}
		f, err := os.OpenFile(target.String(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		_, err = f.WriteString(content)
		if err != nil {
			return nil, err
		}
		return core.NewBool(true), nil

	case "lines":
		data, err := os.ReadFile(target.String())
		if err != nil {
			return nil, err
		}
		rawLines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		items := make([]*core.Value, len(rawLines))
		for i, l := range rawLines {
			items[i] = core.NewString(l)
		}
		return core.NewList(items...), nil

	// Fluent Regex & String Transformations
	case "matches":
		if len(args) == 0 {
			return nil, fmt.Errorf("matches requires a regex pattern")
		}
		matched, err := regexp.MatchString(args[0].String(), target.String())
		if err != nil {
			return nil, err
		}
		return core.NewBool(matched), nil

	case "find":
		if len(args) == 0 {
			return nil, fmt.Errorf("find requires a regex pattern")
		}
		re, err := regexp.Compile(args[0].String())
		if err != nil {
			return nil, err
		}
		found := re.FindString(target.String())
		return core.NewString(found), nil

	case "find_all", "findall":
		if len(args) == 0 {
			return nil, fmt.Errorf("findall requires a regex pattern")
		}
		re, err := regexp.Compile(args[0].String())
		if err != nil {
			return nil, err
		}
		found := re.FindAllString(target.String(), -1)
		items := make([]*core.Value, len(found))
		for i, s := range found {
			items[i] = core.NewString(s)
		}
		return core.NewList(items...), nil

	case "replace":
		if len(args) < 2 {
			return nil, fmt.Errorf("replace requires pattern and replacement")
		}
		pat := args[0].String()
		repl := args[1].String()
		re, err := regexp.Compile(pat)
		if err == nil {
			return core.NewString(re.ReplaceAllString(target.String(), repl)), nil
		}
		return core.NewString(strings.ReplaceAll(target.String(), pat, repl)), nil
	}

	// Check if bound to a registered Go FFI function (e.g. strings.ToUpper, math.Sin, etc.)
	ffiArgs := append([]*core.Value{target}, args...)
	if fn, ok := ffi.GetRegistry().Lookup(methodName); ok {
		return ffi.CallGoFunc(fn, ffiArgs)
	}

	// Check if user-defined procedure exists: proc methodName {receiver args...}
	if proc, ok := in.Procs[methodName]; ok {
		return in.CallProc(proc, ffiArgs)
	}

	return nil, fmt.Errorf("unknown method or property '%s' on %s", methodName, target.Type())
}

func (in *Interpreter) execToolCall(tc *core.ToolCall) (*core.Value, error) {
	if tc == nil {
		return core.NewNull(), nil
	}

	// Check if ExecFn is directly attached
	if tc.ExecFn != nil {
		return tc.ExecFn(tc.Arguments)
	}

	// Check registered tool in AI service
	if registered, ok := in.Services.AI().GetTool(tc.Name); ok && registered.ExecFn != nil {
		return registered.ExecFn(tc.Arguments)
	}

	// Check if procedure exists in PCL
	if proc, ok := in.Procs[tc.Name]; ok {
		args := make([]*core.Value, len(proc.Params))
		for i, p := range proc.Params {
			if val, exists := tc.Arguments[p]; exists {
				args[i] = core.FromNative(val)
			} else {
				args[i] = core.NewNull()
			}
		}
		return in.CallProc(proc, args)
	}

	// Execute as external shell command with arguments
	argsList := make([]string, 0)
	for _, v := range tc.Arguments {
		argsList = append(argsList, fmt.Sprintf("%v", v))
	}

	res, err := in.Services.Process().Execute(in.Ctx, tc.Name, argsList, in.Services.IO().Stdin(), in.Services.IO().Stdout(), in.Services.IO().Stderr())
	if err != nil {
		return nil, err
	}
	return core.NewInt(int64(res.ExitCode)), nil
}

func (in *Interpreter) handleVimEdit(target *core.Value, args []*core.Value) (*core.Value, error) {
	em := shell.NewEditorManager(in.Services.FS())
	targetPath := ""
	initialContent := ""

	if len(args) > 0 {
		targetPath = args[0].String()
	}

	// 1. If target is a List of files (e.g. $files.vim() or $x.files().vim())
	if target.Type() == core.TypeList {
		paths := make([]string, 0, len(target.ListVal))
		for _, el := range target.ListVal {
			p := el.String()
			if p != "" {
				paths = append(paths, p)
			}
		}
		if len(paths) == 0 {
			return core.NewNull(), nil
		}
		err := em.OpenMultipleInEditor(paths)
		if err != nil {
			return nil, err
		}
		return core.NewList(target.ListVal...), nil
	}

	// 2. If target is a Response and no explicit single path was requested:
	if target.Type() == core.TypeResponse && targetPath == "" {
		allFiles := extractAllFilesFromResponse(target.RespVal)
		if len(allFiles) > 1 {
			// Open all files in Neovim tabs simultaneously
			err := em.OpenMultipleInEditor(allFiles)
			if err != nil {
				return nil, err
			}
			items := make([]*core.Value, len(allFiles))
			for i, f := range allFiles {
				items[i] = core.NewString(f)
			}
			return core.NewList(items...), nil
		} else if len(allFiles) == 1 {
			targetPath = allFiles[0]
		}
	}

	switch target.Type() {
	case core.TypeResponse:
		if targetPath == "" && len(target.RespVal.ToolCalls) > 0 {
			tc := target.RespVal.ToolCalls[0]
			for _, k := range []string{"path", "filename", "file", "filepath", "target"} {
				if v, ok := tc.Arguments[k]; ok {
					targetPath = fmt.Sprintf("%v", v)
					break
				}
			}
		}
		for _, tc := range target.RespVal.ToolCalls {
			for _, k := range []string{"content", "code", "text", "body", "data"} {
				if v, ok := tc.Arguments[k]; ok {
					initialContent = fmt.Sprintf("%v", v)
					break
				}
			}
			if initialContent != "" {
				break
			}
		}
		if initialContent == "" {
			initialContent = target.RespVal.Text
		}

	case core.TypeToolCall:
		tc := target.ToolVal
		if targetPath == "" {
			for _, k := range []string{"path", "filename", "file", "filepath", "target"} {
				if v, ok := tc.Arguments[k]; ok {
					targetPath = fmt.Sprintf("%v", v)
					break
				}
			}
		}
		for _, k := range []string{"content", "code", "text", "body", "data"} {
			if v, ok := tc.Arguments[k]; ok {
				initialContent = fmt.Sprintf("%v", v)
				break
			}
		}

	default:
		initialContent = target.String()
	}

	editedContent, finalPath, err := em.OpenInEditor(targetPath, initialContent)
	if err != nil {
		return nil, err
	}

	// Update tool call arguments if applicable
	if target.Type() == core.TypeToolCall {
		for _, k := range []string{"content", "code", "text", "body", "data"} {
			if _, ok := target.ToolVal.Arguments[k]; ok {
				target.ToolVal.Arguments[k] = editedContent
				break
			}
		}
	} else if target.Type() == core.TypeResponse && len(target.RespVal.ToolCalls) > 0 {
		for _, k := range []string{"content", "code", "text", "body", "data"} {
			if _, ok := target.RespVal.ToolCalls[0].Arguments[k]; ok {
				target.RespVal.ToolCalls[0].Arguments[k] = editedContent
				break
			}
		}
	}

	_ = finalPath
	return core.NewString(editedContent), nil
}

func extractAllFilesFromResponse(resp *core.Response) []string {
	if resp == nil {
		return nil
	}
	seen := make(map[string]bool)
	var files []string

	addPath := func(p string) {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`+"`")
		if p != "" && !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}

	extractFromTool := func(tc *core.ToolCall) {
		if tc == nil {
			return
		}
		for _, k := range []string{"path", "filename", "file", "filepath", "target", "dest", "destination"} {
			if v, ok := tc.Arguments[k]; ok {
				addPath(fmt.Sprintf("%v", v))
			}
		}
	}

	// 1. Check ToolCalls on Response
	for _, tc := range resp.ToolCalls {
		extractFromTool(tc)
	}

	// 2. Check Steps across all ReAct turns
	for _, st := range resp.Steps {
		for _, tc := range st.ToolCalls {
			extractFromTool(tc)
		}
	}

	// 3. Scan text / reasoning for markdown code blocks or file annotations
	fullText := resp.Text + "\n" + resp.Reasoning
	lines := strings.Split(fullText, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			tag := strings.TrimPrefix(trimmed, "```")
			if strings.Contains(tag, ":") {
				parts := strings.SplitN(tag, ":", 2)
				addPath(parts[1])
			} else if strings.Contains(tag, "filepath=") {
				parts := strings.SplitN(tag, "filepath=", 2)
				addPath(parts[1])
			}
		} else if strings.HasPrefix(trimmed, "// file:") || strings.HasPrefix(trimmed, "# file:") {
			parts := strings.SplitN(trimmed, ":", 2)
			addPath(parts[1])
		}
	}

	return files
}

package builtins

import (
	"fmt"
	"pcl/pkg/ai"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/services"
	"pcl/pkg/shell"
	"strconv"
	"strings"
	"time"
)

// RegisterAIBuiltins registers prompt, tool management, and ReAct agent primitives.
func RegisterAIBuiltins(in *interp.Interpreter) {
	// prompt <text> or p <text>
	in.RegisterBuiltin("prompt", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("wrong # args: should be \"prompt text\"")
		}
		var sb strings.Builder
		for i, a := range args {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(a.String())
		}

		req := &services.AIRequest{
			Prompt:       sb.String(),
			SystemPrompt: in.EffectiveSystemPrompt(),
			Model:        in.Services.Config().Get("model"),
			Tools:        in.Services.AI().ListTools(),
		}

		resp, err := in.Services.AI().Prompt(in.Ctx, req)
		if err != nil {
			return nil, err
		}
		out := core.NewResponse(resp)
		in.Scope.Set("_", out)
		return out, nil
	})
	in.RegisterBuiltin("p", in.Builtins["prompt"])

	compactFn := func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		n0 := len(in.Chat)
		if n0 < 6 {
			in.Services.IO().Println("nothing to compact")
			return core.NewNull(), nil
		}
		model := in.Services.Config().Get("model")
		msgs, err := ai.CompactMessages(in.Ctx, in.Services.AI(), in.Chat, model)
		if err != nil {
			return nil, err
		}
		in.Chat = msgs
		in.Services.IO().Printf("compacted %d → %d messages\n", n0, len(in.Chat))
		return core.NewInt(int64(len(in.Chat))), nil
	}
	in.RegisterBuiltin(".compact", compactFn)
	in.RegisterBuiltin("compact", compactFn)

	// agent <goal> (autonomous ReAct feedback loop grounded in environment feedback)
	in.RegisterBuiltin("agent", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("wrong # args: should be \"agent goal\"")
		}
		goalParts := make([]string, len(args))
		for i, a := range args {
			goalParts[i] = a.String()
		}
		goal := strings.Join(goalParts, " ")

		opts := ai.DefaultAgentOptions()
		opts.Model = in.Services.Config().Get("model")
		opts.SystemPrompt = in.EffectiveSystemPrompt()
		opts.Chat = &in.Chat

		resp, err := ai.RunReActLoop(in.Ctx, in.Services.AI(), in, goal, opts)
		if err != nil {
			return nil, err
		}
		out := core.NewResponse(resp)
		in.Scope.Set("_", out)
		return out, nil
	})

	// tool <name> <params> <body>
	in.RegisterBuiltin("tool", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) < 3 {
			return nil, fmt.Errorf("wrong # args: should be \"tool name params body\"")
		}
		name := args[0].String()
		paramStr := strings.Trim(args[1].String(), "()")
		body := args[2].String()
		paramsList := strings.Fields(paramStr)
		paramSchema := make(map[string]interface{}, len(paramsList))
		for _, p := range paramsList {
			paramSchema[p] = "string"
		}

		// Register proc in interpreter
		in.Procs[name] = &interp.ProcDef{
			Name:   name,
			Params: paramsList,
			Body:   body,
		}

		// Also register in AI service as callable tool
		in.Services.AI().RegisterTool(name, fmt.Sprintf("User defined tool: %s", name), paramSchema, func(argMap map[string]interface{}) (*core.Value, error) {
			procArgs := make([]*core.Value, len(paramsList))
			for i, p := range paramsList {
				if val, exists := argMap[p]; exists {
					procArgs[i] = core.FromNative(val)
				} else {
					procArgs[i] = core.NewNull()
				}
			}
			return in.CallProc(in.Procs[name], procArgs)
		})

		return core.NewNull(), nil
	})

	// Register default ground-truth environment tools
	registerDefaultEnvironmentTools(in)
	registerSkillBuiltins(in)
	registerMCPBuiltin(in)

	// ai_config ?<key>? ?<value>?
	in.RegisterBuiltin("ai_config", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			all := in.Services.Config().GetAll()
			for k, v := range all {
				in.Services.IO().Printf("%s = %s\n", k, v)
			}
			return core.NewNull(), nil
		}

		if len(args) == 1 {
			k := args[0].String()
			v := in.Services.Config().Get(k)
			return core.NewString(v), nil
		}

		k := args[0].String()
		v := args[1].String()
		in.Services.Config().Set(k, v)
		return core.NewString(v), nil
	})

	// tools (list registered tools)
	in.RegisterBuiltin("tools", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		tools := in.Services.AI().ListTools()
		items := make([]*core.Value, len(tools))
		for i, tc := range tools {
			items[i] = core.NewToolCall(tc)
			in.Services.IO().Printf("  - %s\n", tc.Name)
		}
		return core.NewList(items...), nil
	})
}

func registerDefaultEnvironmentTools(in *interp.Interpreter) {
	aiSvc := in.Services.AI()

	aiSvc.RegisterTool("sh", "Run a POSIX shell command (busybox sh on Windows, sh on Unix). Foreground wait grows Fibonacci (1s, 1s, 2s, 3s, …) while output is flowing; a silent slice backgrounds the process and returns a session id. Use sh_output / sh_kill. Optional timeout (ms, max 10m) waits that long even if silent. run_in_background=true starts immediately. Ctrl+C cancels a foreground wait.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"cmd":               map[string]interface{}{"type": "string", "description": "POSIX script to run"},
			"command":           map[string]interface{}{"type": "string", "description": "Alias for cmd"},
			"timeout":           map[string]interface{}{"type": "number", "description": "Optional hard foreground wait in milliseconds (max 600000). Default is Fibonacci slices that background on silence."},
			"timeout_ms":        map[string]interface{}{"type": "number"},
			"block_until_ms":    map[string]interface{}{"type": "number"},
			"run_in_background": map[string]interface{}{"type": "boolean", "description": "Start and return immediately with a session id"},
			"background":        map[string]interface{}{"type": "boolean"},
		},
	}, func(argMap map[string]interface{}) (*core.Value, error) {
		cmdStr := firstString(argMap, "cmd", "command")
		if cmdStr == "" {
			return core.NewString("Error: cmd parameter required"), nil
		}
		opts := shell.AwaitOpts{
			Background: firstBool(argMap, "run_in_background", "background"),
			Timeout:    firstDurationMS(argMap, "timeout", "timeout_ms", "block_until_ms"),
		}
		r := shell.StartAndAwait(in.Ctx, cmdStr, opts)
		return core.NewString(formatShellResult(r)), nil
	})

	aiSvc.RegisterTool("sh_output", "Read captured output from a background shell session started by sh.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "string", "description": "Session id returned by sh (e.g. sh_1)"},
		},
		"required": []string{"id"},
	}, func(argMap map[string]interface{}) (*core.Value, error) {
		id := firstString(argMap, "id")
		if id == "" {
			return core.NewString("Error: id required"), nil
		}
		s, ok := shell.GetSession(id)
		if !ok {
			return core.NewString("Error: no shell session " + id), nil
		}
		out := strings.TrimSpace(s.Output())
		st := "exited"
		if s.Running() {
			st = "running"
		}
		if out == "" {
			out = "(no output yet)"
		}
		return core.NewString(fmt.Sprintf("[%s %s]\n%s", id, st, out)), nil
	})

	aiSvc.RegisterTool("sh_kill", "Stop a background shell session started by sh.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "string", "description": "Session id returned by sh"},
		},
		"required": []string{"id"},
	}, func(argMap map[string]interface{}) (*core.Value, error) {
		id := firstString(argMap, "id")
		if id == "" {
			return core.NewString("Error: id required"), nil
		}
		if err := shell.KillSession(id); err != nil {
			return core.NewString("Error: " + err.Error()), nil
		}
		return core.NewString("killed " + id), nil
	})

	// 2. read_file tool: reads file content
	aiSvc.RegisterTool("read_file", "Read file contents from disk", map[string]interface{}{"path": "string"}, func(argMap map[string]interface{}) (*core.Value, error) {
		path, _ := argMap["path"].(string)
		if path == "" {
			return core.NewString("Error: path parameter required"), nil
		}
		data, err := in.Services.FS().ReadFile(path)
		if err != nil {
			return core.NewString(fmt.Sprintf("Error reading '%s': %v", path, err)), nil
		}
		return core.NewString(string(data)), nil
	})

	// 3. write_file tool: writes content to file
	aiSvc.RegisterTool("write_file", "Write content to a file on disk", map[string]interface{}{"path": "string", "content": "string"}, func(argMap map[string]interface{}) (*core.Value, error) {
		path, _ := argMap["path"].(string)
		content, _ := argMap["content"].(string)
		if path == "" {
			return core.NewString("Error: path parameter required"), nil
		}
		err := in.Services.FS().WriteFile(path, []byte(content), 0644)
		if err != nil {
			return core.NewString(fmt.Sprintf("Error writing '%s': %v", path, err)), nil
		}
		return core.NewString(fmt.Sprintf("Successfully wrote %d bytes to '%s'", len(content), path)), nil
	})
}

func formatShellResult(r shell.SessionResult) string {
	body := strings.TrimSpace(r.Combined())
	if r.Running {
		var sb strings.Builder
		if body != "" {
			sb.WriteString(body)
			sb.WriteByte('\n')
		}
		sb.WriteString(fmt.Sprintf("Command still running in background (id %s pid %d).\n", r.ID, r.Pid))
		sb.WriteString("Use sh_output with that id to read new output, or sh_kill to stop it.")
		return sb.String()
	}
	if r.Err != nil {
		if body != "" {
			return fmt.Sprintf("%s\nError: %v", body, r.Err)
		}
		return fmt.Sprintf("Error: %v", r.Err)
	}
	if body == "" {
		return "(command completed with exit code 0)"
	}
	return body
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func firstBool(m map[string]interface{}, keys ...string) bool {
	for _, k := range keys {
		switch v := m[k].(type) {
		case bool:
			return v
		case string:
			b, err := strconv.ParseBool(v)
			if err == nil {
				return b
			}
		}
	}
	return false
}

func firstDurationMS(m map[string]interface{}, keys ...string) time.Duration {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		var ms float64
		switch t := v.(type) {
		case float64:
			ms = t
		case int:
			ms = float64(t)
		case int64:
			ms = float64(t)
		case string:
			n, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
			if err != nil {
				continue
			}
			ms = n
		default:
			continue
		}
		if ms < 0 {
			return -1
		}
		if ms == 0 {
			continue
		}
		return time.Duration(ms) * time.Millisecond
	}
	return 0
}

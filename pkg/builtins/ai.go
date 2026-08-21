package builtins

import (
	"fmt"
	"strings"
	"pcl/pkg/ai"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/services"
	"pcl/pkg/shell"
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
			SystemPrompt: in.Services.Config().Get("system_prompt"),
			Model:        in.Services.Config().Get("model"),
			Tools:        in.Services.AI().ListTools(),
		}

		resp, err := in.Services.AI().Prompt(in.Ctx, req)
		if err != nil {
			return nil, err
		}
		return core.NewResponse(resp), nil
	})
	in.RegisterBuiltin("p", in.Builtins["prompt"])

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
		opts.SystemPrompt = in.Services.Config().Get("system_prompt")

		resp, err := ai.RunReActLoop(in.Ctx, in.Services.AI(), in, goal, opts)
		if err != nil {
			return nil, err
		}
		return core.NewResponse(resp), nil
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

	// 1. sh: POSIX shell (busybox sh on Windows, sh on Unix)
	aiSvc.RegisterTool("sh", "Run a POSIX shell command (busybox sh on Windows, sh on Unix). Pass the script in cmd.", map[string]interface{}{"cmd": "string"}, func(argMap map[string]interface{}) (*core.Value, error) {
		cmdStr, _ := argMap["cmd"].(string)
		if cmdStr == "" {
			return core.NewString("Error: cmd parameter required"), nil
		}

		out, errOut, err := shell.RunPOSIX(in.Ctx, cmdStr)
		output := strings.TrimSpace(out)
		errOut = strings.TrimSpace(errOut)

		if err != nil {
			if errOut != "" {
				return core.NewString(fmt.Sprintf("%s\nError: %v", errOut, err)), nil
			}
			return core.NewString(fmt.Sprintf("Error: %v", err)), nil
		}

		if output == "" && errOut != "" {
			return core.NewString(errOut), nil
		}
		if output == "" {
			return core.NewString("(command completed with exit code 0)"), nil
		}
		return core.NewString(output), nil
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

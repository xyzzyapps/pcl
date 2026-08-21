package builtins

import (
	"fmt"
	"strings"
	"pcl/pkg/core"
	"pcl/pkg/interp"
)

func registerMCPBuiltin(in *interp.Interpreter) {
	in.RegisterBuiltin("mcp", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if in.MCP == nil {
			return nil, fmt.Errorf("mcp: not available")
		}
		sub := "list"
		rest := args
		if len(args) > 0 {
			sub = strings.ToLower(args[0].String())
			rest = args[1:]
		}
		io := in.Services.IO()
		switch sub {
		case "list", "ls", "status":
			svcs := in.MCP.List()
			if len(svcs) == 0 {
				io.Println("no mcp servers")
				return core.NewList(), nil
			}
			items := make([]*core.Value, 0, len(svcs))
			for _, s := range svcs {
				cmd := strings.Join(s.Command, " ")
				line := fmt.Sprintf("%s  %s  %s", s.Name, s.Status, cmd)
				if s.Err != "" {
					line += "  (" + s.Err + ")"
				}
				io.Println(line)
				if len(s.Tools) > 0 {
					io.Printf("    tools: %s\n", strings.Join(s.Tools, ", "))
				}
				items = append(items, core.NewString(s.Name))
			}
			return core.NewList(items...), nil
		case "tools":
			tools := in.MCP.Tools()
			if len(tools) == 0 {
				io.Println("no mcp tools")
				return core.NewList(), nil
			}
			items := make([]*core.Value, 0, len(tools))
			for _, t := range tools {
				io.Println(t)
				items = append(items, core.NewString(t))
			}
			return core.NewList(items...), nil
		case "add":
			if len(rest) < 2 {
				return nil, fmt.Errorf("wrong # args: should be \"mcp add name command ?args...?\"")
			}
			name := rest[0].String()
			argv := make([]string, 0, len(rest)-1)
			for _, a := range rest[1:] {
				s := a.String()
				if s == "--" {
					continue
				}
				argv = append(argv, s)
			}
			if err := in.MCP.Add(in.Ctx, name, argv, in.Services.AI()); err != nil {
				return nil, err
			}
			n := 0
			for _, s := range in.MCP.List() {
				if s.Name == name {
					n = len(s.Tools)
					break
				}
			}
			io.Printf("mcp %s connected (%d tools)\n", name, n)
			return core.NewString(name), nil
		case "remove", "rm", "stop":
			if len(rest) != 1 {
				return nil, fmt.Errorf("wrong # args: should be \"mcp remove name\"")
			}
			name := rest[0].String()
			if err := in.MCP.Remove(name, in.Services.AI()); err != nil {
				return nil, err
			}
			io.Printf("mcp %s stopped\n", name)
			return core.NewNull(), nil
		default:
			return nil, fmt.Errorf("mcp: unknown subcommand %q (list, add, remove, tools)", sub)
		}
	})
}


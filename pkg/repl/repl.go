package repl

import (
	"io"
	"strings"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/parser"
	"pcl/pkg/services"
	"pcl/pkg/shell"
)

// REPL is the interactive Read-Eval-Print-Loop manager.
type REPL struct {
	in      *interp.Interpreter
	io      services.IOService
	running bool
}

// NewREPL creates a ready-to-run REPL instance with all builtins registered.
func NewREPL(in *interp.Interpreter) *REPL {
	if in == nil {
		in = interp.NewInterpreter(nil, nil)
	}

	// Register all builtin slices
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)
	builtins.RegisterAIBuiltins(in)
	builtins.RegisterFFIBuiltins(in)

	return &REPL{
		in: in,
		io: in.Services.IO(),
	}
}

// Run starts the interactive REPL loop.
func (r *REPL) Run() error {
	InitTerminal()

	prompt := r.in.Services.Config().Get("prompt")
	if prompt == "" {
		prompt = "pcl> "
	}
	multiPrompt := r.in.Services.Config().Get("multiline_prompt")
	if multiPrompt == "" {
		multiPrompt = "...> "
	}

	r.io.Println("PCL - Prompt Command Language (Type 'exit' to quit)")
	r.running = true

	var buffer strings.Builder

	for r.running {
		if buffer.Len() == 0 {
			r.io.Print(prompt)
		} else {
			r.io.Print(multiPrompt)
		}

		line, err := r.io.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if buffer.Len() > 0 {
			buffer.WriteString("\n")
		}
		buffer.WriteString(line)

		currentInput := buffer.String()
		complete, _ := parser.IsCompleteCommand(currentInput)

		// If input is complete, evaluate it!
		if complete {
			cmdStr := strings.TrimSpace(currentInput)
			buffer.Reset()

			if cmdStr == "" {
				continue
			}

			_ = shell.GetHistory().Add(cmdStr)

			val, err := r.in.Eval(cmdStr)
			if err != nil {
				r.io.PrintfError("Error: %v\n", err)
				continue
			}

			// Print non-null and non-empty results if not already output
			if val != nil && val.Type() != core.TypeNull {
				strRes := val.String()
				if strRes != "" {
					r.io.Println(strRes)
				}
			}
		}
	}

	return nil
}

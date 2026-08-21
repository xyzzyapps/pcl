package interp

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"pcl/pkg/core"
	"pcl/pkg/ffi"
	"pcl/pkg/parser"
	"pcl/pkg/services"
)

// BuiltinFunc is the signature for built-in PCL commands.
type BuiltinFunc func(in *Interpreter, args []*core.Value) (*core.Value, error)

// Interpreter is the main runtime execution engine.
type Interpreter struct {
	mu       sync.RWMutex
	Ctx      context.Context
	Scope    *Scope
	Services *services.ServiceLocator
	Builtins map[string]BuiltinFunc
	Procs    map[string]*ProcDef
	Aliases  map[string]string
}

// NewInterpreter constructs a new Interpreter.
func NewInterpreter(ctx context.Context, loc *services.ServiceLocator) *Interpreter {
	if ctx == nil {
		ctx = context.Background()
	}
	if loc == nil {
		loc = services.GetLocator()
	}

	return &Interpreter{
		Ctx:      ctx,
		Scope:    NewScope(nil),
		Services: loc,
		Builtins: make(map[string]BuiltinFunc),
		Procs:    make(map[string]*ProcDef),
		Aliases:  make(map[string]string),
	}
}

// RegisterBuiltin registers a native command into the interpreter.
func (in *Interpreter) RegisterBuiltin(name string, fn BuiltinFunc) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.Builtins[name] = fn
}

// Eval parses and evaluates a PCL script.
func (in *Interpreter) Eval(scriptStr string) (*core.Value, error) {
	script, err := parser.Parse(scriptStr)
	if err != nil {
		return nil, err
	}
	return in.EvalScript(script)
}

// EvalScript executes a parsed ScriptNode.
func (in *Interpreter) EvalScript(script *parser.ScriptNode) (*core.Value, error) {
	if script == nil || len(script.Statements) == 0 {
		return core.NewNull(), nil
	}

	var lastVal *core.Value = core.NewNull()
	for _, stmt := range script.Statements {
		val, err := in.EvalPipeline(stmt)
		if err != nil {
			return nil, err
		}
		lastVal = val
	}
	return lastVal, nil
}

// ExecuteToolCall executes an AI ToolCall against registered procedures, builtins, or FFI bindings.
func (in *Interpreter) ExecuteToolCall(call *core.ToolCall) (*core.Value, error) {
	if call == nil {
		return core.NewNull(), nil
	}

	// 1. If explicit ExecFn is attached
	if call.ExecFn != nil {
		return call.ExecFn(call.Arguments)
	}

	// 2. Check if registered in AI service
	if in.Services != nil && in.Services.AI() != nil {
		if tc, ok := in.Services.AI().GetTool(call.Name); ok && tc.ExecFn != nil {
			return tc.ExecFn(call.Arguments)
		}
	}

	// 2. Convert Arguments map to positional Value list if matching procedure parameters
	args := make([]*core.Value, 0)
	in.mu.RLock()
	proc, ok := in.Procs[call.Name]
	in.mu.RUnlock()

	if ok && proc != nil {
		for _, p := range proc.Params {
			if v, exists := call.Arguments[p]; exists {
				args = append(args, core.FromNative(v))
			} else {
				args = append(args, core.NewNull())
			}
		}
		return in.CallProc(proc, args)
	}

	// 3. Fallback: Dispatch to command / builtin / FFI
	if cmdVal, exists := call.Arguments["cmd"]; exists {
		args = append(args, core.FromNative(cmdVal))
	} else if codeVal, exists := call.Arguments["code"]; exists {
		args = append(args, core.FromNative(codeVal))
	} else {
		for _, v := range call.Arguments {
			args = append(args, core.FromNative(v))
		}
	}
	return in.Dispatch(call.Name, args, nil)
}

// ListTools returns all tools available in the interpreter and AI service.
func (in *Interpreter) ListTools() []*core.ToolCall {
	if in.Services != nil && in.Services.AI() != nil {
		return in.Services.AI().ListTools()
	}
	return nil
}

// EvalPipeline executes a pipeline of commands connected by '|' or staged by '|> $var'.
func (in *Interpreter) EvalPipeline(pipeline *parser.PipelineNode) (*core.Value, error) {
	if pipeline == nil || len(pipeline.Commands) == 0 {
		return core.NewNull(), nil
	}

	// Single command execution
	if len(pipeline.Commands) == 1 {
		res, err := in.EvalCommand(pipeline.Commands[0])
		if err != nil {
			if !core.IsFlow(err) {
				in.Scope.Set("status", core.NewInt(1))
				in.Scope.Set("?", core.NewInt(1))
			}
			return nil, err
		}
		if len(pipeline.TapVars) > 0 && pipeline.TapVars[0] != "" {
			in.Scope.Set(pipeline.TapVars[0], res)
		}
		exit := int64(0)
		if res != nil && res.Type() == core.TypeBool && !res.AsBool() {
			exit = 1
		}
		in.Scope.Set("status", core.NewInt(exit))
		in.Scope.Set("?", core.NewInt(exit))
		return res, nil
	}

	// Multiple commands in pipeline (shell pipeline execution)
	specs := make([]services.CommandSpec, 0, len(pipeline.Commands))
	tapBuffers := make([]*bytes.Buffer, len(pipeline.Commands))

	for i, cmd := range pipeline.Commands {
		evaluatedWords, err := in.evaluateCommandWords(cmd)
		if err != nil {
			return nil, err
		}
		if len(evaluatedWords) == 0 {
			continue
		}

		cmdName := evaluatedWords[0].String()
		args := make([]string, len(evaluatedWords)-1)
		for j := 1; j < len(evaluatedWords); j++ {
			args[j-1] = evaluatedWords[j].String()
		}

		var tapBuf *bytes.Buffer
		if i < len(pipeline.TapVars) && pipeline.TapVars[i] != "" {
			tapBuf = &bytes.Buffer{}
		}
		tapBuffers[i] = tapBuf

		specs = append(specs, services.CommandSpec{
			Name:         cmdName,
			Args:         args,
			Redirections: cmd.Redirections,
			TapBuffer:    tapBuf,
		})
	}

	res, err := in.Services.Process().ExecutePipeline(
		in.Ctx,
		specs,
		in.Services.IO().Stdin(),
		in.Services.IO().Stdout(),
		in.Services.IO().Stderr(),
	)

	exitCode := 0
	if res != nil {
		exitCode = res.ExitCode
	}
	if err != nil {
		exitCode = 1
	}

	in.Scope.Set("status", core.NewInt(int64(exitCode)))
	in.Scope.Set("?", core.NewInt(int64(exitCode)))

	if err != nil {
		return nil, err
	}

	// Bind captured stage outputs to tap variables
	for i, tapBuf := range tapBuffers {
		if tapBuf != nil && i < len(pipeline.TapVars) && pipeline.TapVars[i] != "" {
			varName := pipeline.TapVars[i]
			captured := strings.TrimRight(tapBuf.String(), "\r\n")
			in.Scope.Set(varName, core.NewString(captured))
		}
	}

	return core.NewInt(int64(exitCode)), nil
}

// EvalCommand executes a single command node.
func (in *Interpreter) EvalCommand(cmd *parser.CommandNode) (*core.Value, error) {
	if cmd == nil {
		return core.NewNull(), nil
	}

	// 1. Handle Assignment: x = <expr> or x["key"] = <expr>
	if cmd.IsAssignment {
		var val *core.Value
		var err error
		if cmd.AssignValue != nil && len(cmd.AssignValue.Tokens) == 1 {
			tok := cmd.AssignValue.Tokens[0]
			if tok.Type == parser.TokParen {
				val, err = in.EvalExpr(tok.Value)
			} else {
				val, err = in.EvalWord(tok)
			}
		} else {
			val, err = in.EvalCommand(cmd.AssignValue)
		}
		if err != nil {
			return nil, err
		}

		target := cmd.AssignTarget
		// Check if target is subscript assignment: x["key"] = val or x[0] = val
		if strings.Contains(target, "[") {
			return in.assignSubscript(target, val)
		}

		in.Scope.Set(target, val)
		return val, nil
	}

	// 2. Evaluate words in command
	evaluatedWords, err := in.evaluateCommandWords(cmd)
	if err != nil {
		return nil, err
	}

	if len(evaluatedWords) == 0 {
		return core.NewNull(), nil
	}

	// 3. Check for standalone single-word expression evaluation (e.g. $x.response, p{...}, [cmd])
	if len(evaluatedWords) == 1 && (cmd.Tokens[0].Type == parser.TokPerlPrompt || cmd.Tokens[0].Type == parser.TokVariable || cmd.Tokens[0].Type == parser.TokBracket || parser.IsUFCSOrAccessExpr(cmd.Tokens[0].Value)) {
		return evaluatedWords[0], nil
	}

	cmdName := evaluatedWords[0].String()
	cmdArgs := evaluatedWords[1:]

	// 4. Dispatch Command
	return in.Dispatch(cmdName, cmdArgs, cmd.Redirections)
}

func (in *Interpreter) assignSubscript(target string, val *core.Value) (*core.Value, error) {
	chain, err := parser.ParseAccessChain(target)
	if err != nil {
		return nil, err
	}
	varName := strings.TrimPrefix(chain.Root, "$")
	existing, ok := in.Scope.Get(varName)
	if !ok {
		// Initialize as dict
		existing = core.NewDict(nil)
		in.Scope.Set(varName, existing)
	}

	if existing.Type() == core.TypeDict {
		if len(chain.Ops) > 0 && chain.Ops[0].Type == parser.OpIndex {
			existing.DictVal[chain.Ops[0].Index] = val
			return val, nil
		}
	}

	return nil, fmt.Errorf("cannot assign subscript on %s", existing.Type())
}

func (in *Interpreter) evaluateCommandWords(cmd *parser.CommandNode) ([]*core.Value, error) {
	words := make([]*core.Value, 0, len(cmd.Tokens))
	for _, tok := range cmd.Tokens {
		var val *core.Value
		var err error
		if tok.Type == parser.TokPerlPrompt && cmd.PromptOpt != "" {
			isStream := strings.HasPrefix(tok.Value, "!")
			promptBody := strings.TrimPrefix(tok.Value, "!")
			expanded, expErr := in.interpolateString(promptBody)
			if expErr != nil {
				return nil, expErr
			}
			val, err = in.evalPrompt(expanded, isStream, cmd.PromptOpt)
		} else {
			val, err = in.EvalWord(tok)
		}
		if err != nil {
			return nil, err
		}
		words = append(words, val)
	}
	return words, nil
}

// Dispatch resolves and executes a command name with arguments.
func (in *Interpreter) Dispatch(name string, args []*core.Value, redirections []services.Redirection) (*core.Value, error) {
	// 0. Check Alias
	in.mu.RLock()
	aliasCmd, hasAlias := in.Aliases[name]
	in.mu.RUnlock()
	if hasAlias {
		parts := strings.Fields(aliasCmd)
		if len(parts) > 0 {
			name = parts[0]
			aliasArgs := make([]*core.Value, 0, len(parts)-1+len(args))
			for _, p := range parts[1:] {
				aliasArgs = append(aliasArgs, core.NewString(p))
			}
			args = append(aliasArgs, args...)
		}
	}

	in.mu.RLock()
	builtin, isBuiltin := in.Builtins[name]
	proc, isProc := in.Procs[name]
	in.mu.RUnlock()

	// 1. Check Builtin Command
	if isBuiltin {
		return builtin(in, args)
	}

	// 2. Check User-defined Procedure
	if isProc {
		return in.CallProc(proc, args)
	}

	// 3. Check Go FFI symbol (e.g. math.Sin, strings.ToUpper)
	if ffiFn, ok := ffi.GetRegistry().Lookup(name); ok {
		return ffi.CallGoFunc(ffiFn, args)
	}

	// 4. Fallback: Execute as external binary / shell command
	strArgs := make([]string, len(args))
	for i, a := range args {
		strArgs[i] = a.String()
	}

	specs := []services.CommandSpec{
		{
			Name:         name,
			Args:         strArgs,
			Redirections: redirections,
		},
	}

	res, err := in.Services.Process().ExecutePipeline(
		in.Ctx,
		specs,
		in.Services.IO().Stdin(),
		in.Services.IO().Stdout(),
		in.Services.IO().Stderr(),
	)
	if err != nil {
		return nil, core.NewError(core.ErrCommandNotFound, fmt.Sprintf("command '%s' not found or execution failed: %v", name, err))
	}

	return core.NewInt(int64(res.ExitCode)), nil
}

func (in *Interpreter) CallProc(proc *ProcDef, args []*core.Value) (*core.Value, error) {
	if proc == nil {
		return core.NewNull(), nil
	}

	// Create local frame scope
	localScope := NewScope(in.Scope)
	for i, param := range proc.Params {
		if i < len(args) {
			localScope.Set(param, args[i])
		} else {
			localScope.Set(param, core.NewNull())
		}
	}

	savedScope := in.Scope
	in.Scope = localScope
	defer func() {
		in.Scope = savedScope
	}()

	return in.Eval(proc.Body)
}

package repl

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/parser"
	"pcl/pkg/services"
	"pcl/pkg/shell"

	"github.com/chzyer/readline"
)

// REPL is the interactive Read-Eval-Print-Loop manager.
type REPL struct {
	in      *interp.Interpreter
	io      services.IOService
	running bool
	rl      *readline.Instance

	jobMu   sync.Mutex
	aiMu    sync.Mutex
	outMu   sync.Mutex
	nextJob int64
	jobs    map[int64]*bgJob
	closed  atomic.Bool
}

type bgJob struct {
	id      int64
	cmd     string
	running bool
}

// NewREPL creates a ready-to-run REPL instance with all builtins registered.
func NewREPL(in *interp.Interpreter) *REPL {
	if in == nil {
		in = interp.NewInterpreter(nil, nil)
	}

	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)
	builtins.RegisterAIBuiltins(in)
	builtins.RegisterFFIBuiltins(in)

	r := &REPL{
		in:   in,
		io:   in.Services.IO(),
		jobs: make(map[int64]*bgJob),
	}
	in.RegisterBuiltin("jobs", r.jobsBuiltin)
	return r
}

func (r *REPL) jobsBuiltin(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
	r.jobMu.Lock()
	defer r.jobMu.Unlock()
	if len(r.jobs) == 0 {
		in.Services.IO().Println("no jobs")
		return core.NewNull(), nil
	}
	ids := make([]int64, 0, len(r.jobs))
	for id := range r.jobs {
		ids = append(ids, id)
	}
	for _, id := range ids {
		j := r.jobs[id]
		st := "done"
		if j.running {
			st = "running"
		}
		in.Services.IO().Printf("[%d] %s  %s\n", j.id, st, j.cmd)
	}
	return core.NewNull(), nil
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

	histPath := shell.GetHistory().Path()
	cfg := &readline.Config{
		Prompt:          prompt,
		HistoryFile:     histPath,
		AutoComplete:    &Completer{repl: r},
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		UniqueEditLine:  false,
	}
	rl, err := readline.NewEx(cfg)
	if err != nil {
		return err
	}
	defer func() {
		r.closed.Store(true)
		r.rl = nil
		_ = rl.Close()
	}()
	r.rl = rl

	r.io.Println("PCL — type a command, Tab to complete, p(...) runs in the background while you keep typing")
	r.running = true

	var buffer strings.Builder

	for r.running {
		if buffer.Len() == 0 {
			rl.SetPrompt(prompt)
		} else {
			rl.SetPrompt(multiPrompt)
		}

		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				buffer.Reset()
				continue
			}
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
		if !complete {
			continue
		}

		cmdStr := strings.TrimSpace(currentInput)
		buffer.Reset()
		if cmdStr == "" {
			continue
		}

		_ = shell.GetHistory().Add(cmdStr)
		_ = rl.SaveHistory(cmdStr)

		if looksLikeAICommand(cmdStr) {
			r.startAIJob(cmdStr)
			continue
		}

		r.evalForeground(cmdStr)
	}

	return nil
}

func (r *REPL) evalForeground(cmdStr string) {
	val, err := r.in.Eval(cmdStr)
	if err != nil {
		r.io.PrintfError("Error: %v\n", err)
		return
	}
	if val != nil && val.Type() != core.TypeNull {
		strRes := val.String()
		if strRes != "" {
			r.io.Println(strRes)
		}
	}
}

func (r *REPL) startAIJob(cmdStr string) {
	id := atomic.AddInt64(&r.nextJob, 1)
	j := &bgJob{id: id, cmd: trimJobPreview(cmdStr), running: true}
	r.jobMu.Lock()
	r.jobs[id] = j
	r.jobMu.Unlock()

	sw := &jobWriter{repl: r, id: id}
	go func() {
		r.aiMu.Lock()
		defer r.aiMu.Unlock()
		r.in.StreamWriter = sw
		_, err := r.in.Eval(cmdStr)
		r.in.StreamWriter = nil

		r.jobMu.Lock()
		j.running = false
		r.jobMu.Unlock()

		if err != nil {
			fmt.Fprintf(sw, "error: %v\n", err)
		}
		sw.Flush()
		r.refreshPrompt()
	}()
}

type jobWriter struct {
	repl *REPL
	id   int64
	buf  bytes.Buffer
}

func (r *REPL) refreshPrompt() {
	if r.closed.Load() {
		return
	}
	r.outMu.Lock()
	defer r.outMu.Unlock()
	if r.rl != nil {
		r.rl.Refresh()
	}
}

func (w *jobWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.repl.outMu.Lock()
	defer w.repl.outMu.Unlock()
	_, _ = w.buf.Write(p)
	w.drainLocked(false)
	return len(p), nil
}

func (w *jobWriter) Flush() {
	w.repl.outMu.Lock()
	defer w.repl.outMu.Unlock()
	w.drainLocked(true)
}

func (w *jobWriter) drainLocked(flush bool) {
	for {
		data := w.buf.Bytes()
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		line := string(bytes.TrimRight(data[:i], "\r"))
		w.buf.Next(i + 1)
		w.emitLocked(line)
	}
	if flush && w.buf.Len() > 0 {
		w.emitLocked(w.buf.String())
		w.buf.Reset()
	}
}

func (w *jobWriter) emitLocked(line string) {
	if w.repl.closed.Load() {
		return
	}
	// Erase the live prompt, print the log line, then redraw pcl> at the bottom.
	if rl := w.repl.rl; rl != nil {
		rl.Clean()
		_, _ = fmt.Fprintf(rl.Stdout(), "%s\n", line)
		rl.Refresh()
		return
	}
	fmt.Fprintln(os.Stderr, line)
}

func looksLikeAICommand(cmd string) bool {
	s := strings.TrimSpace(cmd)
	if i := strings.Index(s, "="); i > 0 {
		left := strings.TrimSpace(s[:i])
		if isIdent(left) {
			s = strings.TrimSpace(s[i+1:])
		}
	}
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "[") {
		end := strings.TrimSpace(s[1:])
		return looksLikeAICommand(end)
	}
	if strings.HasPrefix(s, "prompt ") || strings.HasPrefix(s, "agent ") {
		return true
	}
	if len(s) >= 2 && (s[0] == 'p' || s[0] == 'q') {
		rest := s[1:]
		if strings.HasPrefix(rest, "q") || strings.HasPrefix(rest, "x") || strings.HasPrefix(rest, "w") {
			rest = rest[1:]
		}
		if strings.HasPrefix(rest, "!") {
			rest = rest[1:]
		}
		if rest != "" && strings.ContainsRune("({[/!<", rune(rest[0])) {
			return s[0] == 'p'
		}
	}
	return false
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
			return false
		}
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

func trimJobPreview(cmd string) string {
	cmd = strings.ReplaceAll(cmd, "\n", " ")
	if len(cmd) > 80 {
		return cmd[:80] + "…"
	}
	return cmd
}

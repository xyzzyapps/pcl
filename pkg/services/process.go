package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// RedirectionType specifies redirection mode.
type RedirectionType int

const (
	RedirectNone RedirectionType = iota
	RedirectInput         // < file
	RedirectOutput        // > file
	RedirectAppend        // >> file
	RedirectError         // 2> file
	RedirectErrorAppend   // 2>> file
	RedirectErrorToStdout // 2>&1
)

// Redirection defines a single file descriptor redirection.
type Redirection struct {
	Type   RedirectionType
	Target string
}

// CommandSpec defines a single command in a pipeline.
type CommandSpec struct {
	Name         string
	Args         []string
	Redirections []Redirection
	TapBuffer    *bytes.Buffer // In-memory tap destination buffer
}

// ProcessResult captures exit code and captured output.
type ProcessResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// ProcessService defines process and pipeline execution capabilities.
type ProcessService interface {
	Execute(ctx context.Context, cmdName string, args []string, stdin io.Reader, stdout, stderr io.Writer) (*ProcessResult, error)
	ExecutePipeline(ctx context.Context, commands []CommandSpec, stdin io.Reader, stdout, stderr io.Writer) (*ProcessResult, error)
}

// DefaultProcessService implements process execution on OS.
type DefaultProcessService struct {
	fs FSService
}

func NewDefaultProcessService(fs FSService) *DefaultProcessService {
	if fs == nil {
		fs = NewDefaultFSService()
	}
	return &DefaultProcessService{fs: fs}
}

func (p *DefaultProcessService) Execute(ctx context.Context, cmdName string, args []string, stdin io.Reader, stdout, stderr io.Writer) (*ProcessResult, error) {
	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return &ProcessResult{ExitCode: 1}, err
		}
	}
	return &ProcessResult{ExitCode: exitCode}, nil
}

func (p *DefaultProcessService) ExecutePipeline(ctx context.Context, commands []CommandSpec, defaultStdin io.Reader, defaultStdout, defaultStderr io.Writer) (*ProcessResult, error) {
	if len(commands) == 0 {
		return &ProcessResult{ExitCode: 0}, nil
	}

	// Single command optimization
	if len(commands) == 1 {
		return p.executeSingle(ctx, commands[0], defaultStdin, defaultStdout, defaultStderr)
	}

	// Multiple commands in pipeline
	var cmds []*exec.Cmd
	var pipesToClose []io.Closer
	defer func() {
		for _, pipe := range pipesToClose {
			pipe.Close()
		}
	}()

	var lastStdout io.Reader = defaultStdin

	for i, spec := range commands {
		isFirst := (i == 0)
		isLast := (i == len(commands)-1)

		cmdPath, args, err := p.resolveExec(spec.Name, spec.Args)
		if err != nil {
			return &ProcessResult{ExitCode: 127}, fmt.Errorf("command not found: %s", spec.Name)
		}

		cmd := exec.CommandContext(ctx, cmdPath, args...)
		cmds = append(cmds, cmd)

		// Set stdin
		if isFirst {
			cmd.Stdin = lastStdout
		} else {
			cmd.Stdin = lastStdout
		}

		// Apply input redirection if specified
		for _, redir := range spec.Redirections {
			if redir.Type == RedirectInput {
				f, err := os.Open(redir.Target)
				if err != nil {
					return &ProcessResult{ExitCode: 1}, fmt.Errorf("cannot open input file %s: %w", redir.Target, err)
				}
				pipesToClose = append(pipesToClose, f)
				cmd.Stdin = f
			}
		}

		// Set stdout
		if isLast {
			if spec.TapBuffer != nil {
				cmd.Stdout = io.MultiWriter(defaultStdout, spec.TapBuffer)
			} else {
				cmd.Stdout = defaultStdout
			}
		} else {
			pr, pw := io.Pipe()
			if spec.TapBuffer != nil {
				cmd.Stdout = io.MultiWriter(pw, spec.TapBuffer)
			} else {
				cmd.Stdout = pw
			}
			pipesToClose = append(pipesToClose, pw)
			lastStdout = pr
		}

		// Set stderr
		cmd.Stderr = defaultStderr

		// Apply output and error redirections
		for _, redir := range spec.Redirections {
			switch redir.Type {
			case RedirectOutput:
				f, err := os.OpenFile(redir.Target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
				if err != nil {
					return &ProcessResult{ExitCode: 1}, err
				}
				pipesToClose = append(pipesToClose, f)
				cmd.Stdout = f
			case RedirectAppend:
				f, err := os.OpenFile(redir.Target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
				if err != nil {
					return &ProcessResult{ExitCode: 1}, err
				}
				pipesToClose = append(pipesToClose, f)
				cmd.Stdout = f
			case RedirectError:
				f, err := os.OpenFile(redir.Target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
				if err != nil {
					return &ProcessResult{ExitCode: 1}, err
				}
				pipesToClose = append(pipesToClose, f)
				cmd.Stderr = f
			case RedirectErrorAppend:
				f, err := os.OpenFile(redir.Target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
				if err != nil {
					return &ProcessResult{ExitCode: 1}, err
				}
				pipesToClose = append(pipesToClose, f)
				cmd.Stderr = f
			case RedirectErrorToStdout:
				cmd.Stderr = cmd.Stdout
			}
		}
	}

	// Start all commands
	for _, cmd := range cmds {
		if err := cmd.Start(); err != nil {
			return &ProcessResult{ExitCode: 1}, err
		}
	}

	// Wait for last command
	var lastExitCode int
	for i, cmd := range cmds {
		err := cmd.Wait()
		if i == len(cmds)-1 {
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					lastExitCode = exitErr.ExitCode()
				} else {
					lastExitCode = 1
				}
			} else {
				lastExitCode = 0
			}
		}
	}

	return &ProcessResult{ExitCode: lastExitCode}, nil
}

func (p *DefaultProcessService) executeSingle(ctx context.Context, spec CommandSpec, defaultStdin io.Reader, defaultStdout, defaultStderr io.Writer) (*ProcessResult, error) {
	cmdPath, args, err := p.resolveExec(spec.Name, spec.Args)
	if err != nil {
		return &ProcessResult{ExitCode: 127}, fmt.Errorf("command not found: %s", spec.Name)
	}

	cmd := exec.CommandContext(ctx, cmdPath, args...)
	cmd.Stdin = defaultStdin
	cmd.Stdout = defaultStdout
	cmd.Stderr = defaultStderr

	var filesToClose []*os.File
	defer func() {
		for _, f := range filesToClose {
			f.Close()
		}
	}()

	for _, redir := range spec.Redirections {
		switch redir.Type {
		case RedirectInput:
			f, err := os.Open(redir.Target)
			if err != nil {
				return &ProcessResult{ExitCode: 1}, err
			}
			filesToClose = append(filesToClose, f)
			cmd.Stdin = f
		case RedirectOutput:
			f, err := os.OpenFile(redir.Target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
			if err != nil {
				return &ProcessResult{ExitCode: 1}, err
			}
			filesToClose = append(filesToClose, f)
			cmd.Stdout = f
		case RedirectAppend:
			f, err := os.OpenFile(redir.Target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
			if err != nil {
				return &ProcessResult{ExitCode: 1}, err
			}
			filesToClose = append(filesToClose, f)
			cmd.Stdout = f
		case RedirectError:
			f, err := os.OpenFile(redir.Target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
			if err != nil {
				return &ProcessResult{ExitCode: 1}, err
			}
			filesToClose = append(filesToClose, f)
			cmd.Stderr = f
		case RedirectErrorAppend:
			f, err := os.OpenFile(redir.Target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
			if err != nil {
				return &ProcessResult{ExitCode: 1}, err
			}
			filesToClose = append(filesToClose, f)
			cmd.Stderr = f
		case RedirectErrorToStdout:
			cmd.Stderr = cmd.Stdout
		}
	}

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return &ProcessResult{ExitCode: 1}, err
		}
	}

	return &ProcessResult{ExitCode: exitCode}, nil
}

func (p *DefaultProcessService) resolveExec(name string, args []string) (string, []string, error) {
	if strings.EqualFold(name, "sh") {
		if runtime.GOOS == "windows" {
			for _, bb := range []string{"busybox", "busybox.exe", "busybox64.exe"} {
				if pth, err := p.fs.LookPath(bb); err == nil {
					return pth, append([]string{"sh"}, args...), nil
				}
			}
			return "", nil, fmt.Errorf("busybox not found on PATH (needed for sh on Windows)")
		}
		pth, err := p.fs.LookPath("sh")
		if err != nil {
			return "", nil, err
		}
		return pth, args, nil
	}
	pth, err := p.resolveCommand(name)
	return pth, args, err
}

func (p *DefaultProcessService) resolveCommand(name string) (string, error) {
	// Builtin shell commands on Windows like dir, echo, cls when executed externally
	if runtime.GOOS == "windows" {
		lower := strings.ToLower(name)
		if lower == "dir" || lower == "cls" || lower == "copy" || lower == "del" || lower == "type" {
			// Wrap with cmd /c
			return "cmd.exe", nil
		}
	}
	return p.fs.LookPath(name)
}

package shell

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"pcl/pkg/core"
	"pcl/pkg/services"
)

// Executor manages external process discovery and dispatch.
type Executor struct {
	fs   services.FSService
	proc services.ProcessService
}

func NewExecutor(fs services.FSService, proc services.ProcessService) *Executor {
	if fs == nil {
		fs = services.NewDefaultFSService()
	}
	if proc == nil {
		proc = services.NewDefaultProcessService(fs)
	}
	return &Executor{fs: fs, proc: proc}
}

// FindExecutable searches for command in PATH with executable extensions.
func (e *Executor) FindExecutable(name string) (string, error) {
	return e.fs.LookPath(name)
}

// Execute runs a command with arguments and default standard I/O.
func (e *Executor) Execute(ctx context.Context, name string, args []string, in services.IOService) (int, error) {
	res, err := e.proc.Execute(ctx, name, args, in.Stdin(), in.Stdout(), in.Stderr())
	if err != nil {
		return 1, err
	}
	return res.ExitCode, nil
}

// ExpandPath expands environment variables and ~ in paths.
func ExpandPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return os.ExpandEnv(path)
}

// ProcessResultToValue converts execution result to Value.
func ProcessResultToValue(res *services.ProcessResult) *core.Value {
	if res == nil {
		return core.NewInt(0)
	}
	return core.NewInt(int64(res.ExitCode))
}

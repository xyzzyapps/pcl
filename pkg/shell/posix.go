package shell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sync"
	"pcl/pkg/services"
)

// FindPOSIXShell returns the executable and extra argv prefix for POSIX sh.
// Windows: busybox + ["sh"]  (busybox sh -c ...)
// Unix:    sh + nil          (sh -c ...)
func FindPOSIXShell() (bin string, prefix []string, err error) {
	fs := services.NewDefaultFSService()
	if runtime.GOOS == "windows" {
		for _, name := range []string{"busybox", "busybox.exe", "busybox64.exe"} {
			p, lookErr := fs.LookPath(name)
			if lookErr == nil {
				return p, []string{"sh"}, nil
			}
		}
		return "", nil, fmt.Errorf("busybox not found on PATH (install BusyBox and keep busybox.exe on PATH to run sh on Windows)")
	}
	p, lookErr := fs.LookPath("sh")
	if lookErr != nil {
		return "", nil, fmt.Errorf("sh not found on PATH")
	}
	return p, nil, nil
}

// CommandContext builds `sh -c script` (busybox sh -c on Windows).
func CommandContext(ctx context.Context, script string) (*exec.Cmd, error) {
	bin, prefix, err := FindPOSIXShell()
	if err != nil {
		return nil, err
	}
	args := append(append([]string{}, prefix...), "-c", script)
	return exec.CommandContext(ctx, bin, args...), nil
}

// POSIXResult is the outcome of a shell script run.
type POSIXResult struct {
	Stdout string
	Stderr string
	Err    error
	Pid    int
}

// RunPOSIX runs a shell script and captures stdout/stderr.
// It waits until the process exits or ctx is cancelled (Ctrl+C in the REPL).
func RunPOSIX(ctx context.Context, script string) (stdout, stderr string, err error) {
	r := RunPOSIXWait(ctx, script, nil)
	return r.Stdout, r.Stderr, r.Err
}

type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// RunPOSIXWait runs a script until it exits or ctx is cancelled.
// Stdin is the null device so the process cannot steal the REPL TTY.
func RunPOSIXWait(ctx context.Context, script string, live io.Writer) POSIXResult {
	if ctx == nil {
		ctx = context.Background()
	}

	bin, prefix, err := FindPOSIXShell()
	if err != nil {
		return POSIXResult{Err: err}
	}
	args := append(append([]string{}, prefix...), "-c", script)
	cmd := exec.Command(bin, args...)
	applySysProcAttr(cmd)

	var outBuf, errBuf safeBuf
	if live != nil {
		cmd.Stdout = io.MultiWriter(&outBuf, live)
		cmd.Stderr = io.MultiWriter(&errBuf, live)
	} else {
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
	}

	if err := cmd.Start(); err != nil {
		return POSIXResult{Err: err}
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case waitErr := <-done:
		return POSIXResult{
			Stdout: outBuf.String(),
			Stderr: errBuf.String(),
			Err:    waitErr,
			Pid:    pid,
		}
	case <-ctx.Done():
		killCmd(cmd)
		<-done
		return POSIXResult{
			Stdout: outBuf.String(),
			Stderr: errBuf.String(),
			Err:    ctx.Err(),
			Pid:    pid,
		}
	}
}

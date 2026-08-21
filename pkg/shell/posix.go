package shell

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
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

// RunPOSIX runs a shell script string and captures stdout/stderr.
func RunPOSIX(ctx context.Context, script string) (stdout, stderr string, err error) {
	cmd, err := CommandContext(ctx, script)
	if err != nil {
		return "", "", err
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

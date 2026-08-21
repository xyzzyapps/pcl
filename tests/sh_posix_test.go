package tests

import (
	"context"
	"strings"
	"testing"
	"pcl/pkg/shell"
)

func TestRunPOSIXEcho(t *testing.T) {
	if _, _, err := shell.FindPOSIXShell(); err != nil {
		t.Skip(err.Error())
	}
	out, errOut, err := shell.RunPOSIX(context.Background(), "echo pcl_posix_ok")
	if err != nil {
		t.Fatalf("RunPOSIX: %v (%s)", err, errOut)
	}
	if !strings.Contains(out, "pcl_posix_ok") {
		t.Fatalf("unexpected output %q", out)
	}
}

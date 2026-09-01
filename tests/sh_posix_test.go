package tests

import (
	"context"
	"pcl/pkg/shell"
	"strings"
	"testing"
	"time"
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

func TestStartAndAwaitFibBackgroundsSilent(t *testing.T) {
	if _, _, err := shell.FindPOSIXShell(); err != nil {
		t.Skip(err.Error())
	}
	start := time.Now()
	r := shell.StartAndAwait(context.Background(), "sleep 20", shell.AwaitOpts{})
	elapsed := time.Since(start)
	if elapsed > 4*time.Second {
		t.Fatalf("default fib wait blocked too long (%s)", elapsed)
	}
	if !r.Running {
		t.Fatalf("expected backgrounded silent process, err=%v", r.Err)
	}
	if err := shell.KillSession(r.ID); err != nil {
		t.Fatal(err)
	}
}

func TestStartAndAwaitBackgroundsOnTimeout(t *testing.T) {
	if _, _, err := shell.FindPOSIXShell(); err != nil {
		t.Skip(err.Error())
	}
	start := time.Now()
	r := shell.StartAndAwait(context.Background(), "sleep 20", shell.AwaitOpts{Timeout: 200 * time.Millisecond})
	if time.Since(start) > 3*time.Second {
		t.Fatalf("foreground wait blocked too long")
	}
	if !r.Running {
		t.Fatalf("expected backgrounded process, err=%v", r.Err)
	}
	if r.ID == "" || r.Pid == 0 {
		t.Fatalf("expected session id and pid, got %+v", r)
	}
	s, ok := shell.GetSession(r.ID)
	if !ok || !s.Running() {
		t.Fatal("session not tracked")
	}
	if err := shell.KillSession(r.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRunPOSIXCancelStopsProcess(t *testing.T) {
	if _, _, err := shell.FindPOSIXShell(); err != nil {
		t.Skip(err.Error())
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan shell.POSIXResult, 1)
	go func() {
		done <- shell.RunPOSIXWait(ctx, "sleep 30", nil)
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case r := <-done:
		if r.Err == nil {
			t.Fatal("expected error after cancel")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not stop the process")
	}
}

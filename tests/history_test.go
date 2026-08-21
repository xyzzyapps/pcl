package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/services"
	"pcl/pkg/shell"
)

func TestHistoryPersistence(t *testing.T) {
	tempDir := t.TempDir()
	histFile := filepath.Join(tempDir, ".pcl_test_history")

	hm := shell.NewHistoryManager(histFile)
	_ = hm.Add("set a 10")
	_ = hm.Add("puts $a")

	entries := hm.List()
	if len(entries) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(entries))
	}

	// Verify file was written
	data, err := os.ReadFile(histFile)
	if err != nil || len(data) == 0 {
		t.Fatalf("failed reading history file: %v", err)
	}

	// Test history builtin
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)

	val, err := in.Eval("history")
	if err != nil || val == nil || val.Type() != core.TypeList {
		t.Fatalf("history command failed: %v", err)
	}
}

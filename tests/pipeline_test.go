package tests

import (
	"context"
	"os"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestRedirectionAndPipeline(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)

	tmpFile := "test_redirect_output.tmp"
	defer os.Remove(tmpFile)

	// Builtin echo + redirection (portable; do not use cmd.exe)
	_, err := in.Eval("echo pcl_redirection_test > " + tmpFile)
	if err != nil {
		t.Fatalf("redirection failed: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed reading output file: %v", err)
	}

	if len(data) == 0 {
		t.Fatalf("output file is empty")
	}
}

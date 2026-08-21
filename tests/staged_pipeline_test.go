package tests

import (
	"context"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestStagedPipelineAndExitCode(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)

	// Test staged tap destination
	script := `
		echo "hello staged world" |> $stage1
	`
	_, err := in.Eval(script)
	if err != nil {
		t.Fatalf("staged pipeline failed: %v", err)
	}

	val, ok := in.Scope.Get("stage1")
	if !ok || val.String() != "hello staged world" {
		t.Fatalf("expected stage1='hello staged world', got '%v'", val)
	}

	// Verify $status and $?
	statusVal, ok := in.Scope.Get("status")
	if !ok || statusVal.String() != "0" {
		t.Fatalf("expected $status=0, got '%v'", statusVal)
	}

	qVal, ok := in.Scope.Get("?")
	if !ok || qVal.String() != "0" {
		t.Fatalf("expected $?=0, got '%v'", qVal)
	}
}

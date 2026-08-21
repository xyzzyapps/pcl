package tests

import (
	"context"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestCrossPlatformGlobbing(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)

	script := `
		files = [glob "*.go"]
		count = $files.len()
	`
	_, err := in.Eval(script)
	if err != nil {
		t.Fatalf("glob command failed: %v", err)
	}

	countVal, ok := in.Scope.Get("count")
	if !ok || countVal.String() == "0" {
		t.Fatalf("expected at least 1 Go file in directory, got '%v'", countVal)
	}
}

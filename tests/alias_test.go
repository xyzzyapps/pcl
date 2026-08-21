package tests

import (
	"context"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestCommandAliases(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)

	// Define alias
	_, err := in.Eval(`alias greet="echo hello"`)
	if err != nil {
		t.Fatalf("alias definition failed: %v", err)
	}

	// Execute alias
	val, err := in.Eval(`greet "world"`)
	if err != nil || val.String() != "hello world" {
		t.Fatalf("expected 'hello world', got %v", val)
	}

	// List aliases
	aliasList, err := in.Eval(`alias`)
	if err != nil || aliasList == nil {
		t.Fatalf("alias list failed: %v", err)
	}

	// Unalias
	_, err = in.Eval(`unalias greet`)
	if err != nil {
		t.Fatalf("unalias failed: %v", err)
	}
}

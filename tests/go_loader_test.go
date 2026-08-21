package tests

import (
	"context"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/ffi"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestInspectAndLoadGoFile(t *testing.T) {
	funcs, pkgName, err := ffi.InspectGoFile("../custom_funcs.go")
	if err != nil {
		// Try local path
		funcs, pkgName, err = ffi.InspectGoFile("custom_funcs.go")
	}
	if err != nil {
		t.Fatalf("failed inspecting Go file: %v", err)
	}

	if pkgName != "custom" {
		t.Fatalf("expected package 'custom', got '%s'", pkgName)
	}

	if len(funcs) < 3 {
		t.Fatalf("expected at least 3 exported functions, got %d", len(funcs))
	}

	foundTax := false
	for _, fn := range funcs {
		if fn.Name == "CalculateTax" {
			foundTax = true
			break
		}
	}
	if !foundTax {
		t.Fatalf("CalculateTax function not found")
	}
}

func TestLoadGoScriptCommand(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterFFIBuiltins(in)

	script := `
		load_go "../custom_funcs.go"
	`
	_, err := in.Eval(script)
	if err != nil {
		// Try local path
		_, err = in.Eval(`load_go "custom_funcs.go"`)
	}
	if err != nil {
		t.Fatalf("load_go execution failed: %v", err)
	}

	// Verify symbol is registered
	if _, ok := ffi.GetRegistry().Lookup("CalculateTax"); !ok {
		t.Fatalf("CalculateTax symbol not registered in FFI registry")
	}
}

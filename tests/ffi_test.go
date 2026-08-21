package tests

import (
	"context"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/ffi"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestFFIReflectCall(t *testing.T) {
	reg := ffi.GetRegistry()
	fn, ok := reg.Lookup("strings.ToUpper")
	if !ok {
		t.Fatalf("strings.ToUpper not found in registry")
	}

	res, err := ffi.CallGoFunc(fn, []*core.Value{core.NewString("hello world")})
	if err != nil {
		t.Fatalf("FFI call failed: %v", err)
	}

	if res.String() != "HELLO WORLD" {
		t.Fatalf("expected 'HELLO WORLD', got '%s'", res.String())
	}
}

func TestFFIMathCall(t *testing.T) {
	reg := ffi.GetRegistry()
	fn, ok := reg.Lookup("math.Sqrt")
	if !ok {
		t.Fatalf("math.Sqrt not found in registry")
	}

	res, err := ffi.CallGoFunc(fn, []*core.Value{core.NewFloat(16.0)})
	if err != nil {
		t.Fatalf("FFI math call failed: %v", err)
	}

	f, err := res.AsFloat()
	if err != nil || f != 4.0 {
		t.Fatalf("expected 4.0, got %v", f)
	}
}

func TestFFIScriptBinding(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterFFIBuiltins(in)

	script := `
		ffi::bind upper strings.ToUpper
		set msg "tcl prompt"
		set upperMsg [upper $msg]
	`
	_, err := in.Eval(script)
	if err != nil {
		t.Fatalf("FFI script execution failed: %v", err)
	}

	val, ok := in.Scope.Get("upperMsg")
	if !ok || val.String() != "TCL PROMPT" {
		t.Fatalf("expected 'TCL PROMPT', got '%v'", val)
	}
}

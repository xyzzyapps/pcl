package tests

import (
	"context"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestInterpreterControlFlow(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)

	script := `
		set sum 0
		set i 1
		while ($i <= 5) (
			set sum ($sum + $i)
			set i ($i + 1)
		)
	`
	_, err := in.Eval(script)
	if err != nil {
		t.Fatalf("while loop failed: %v", err)
	}

	sumVal, ok := in.Scope.Get("sum")
	if !ok || sumVal.String() != "15" {
		t.Fatalf("expected sum=15, got %v", sumVal)
	}
}

func TestBreakAndContinue(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)

	script := `
		set n 0
		set i 0
		while ($i < 10) (
			set i ($i + 1)
			if ($i == 3) ( continue )
			if ($i == 6) ( break )
			set n ($n + $i)
		)
	`
	_, err := in.Eval(script)
	if err != nil {
		t.Fatalf("break/continue loop failed: %v", err)
	}
	// i=1,2 skip 3, i=4,5 then break at 6 → n = 1+2+4+5 = 12
	nVal, ok := in.Scope.Get("n")
	if !ok || nVal.String() != "12" {
		t.Fatalf("expected n=12, got %v", nVal)
	}
}

func TestBreakOutsideLoop(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	if _, err := in.Eval("break"); err == nil {
		t.Fatal("expected error for break outside loop")
	}
}

func TestProcedureScoping(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)

	script := `
		proc add (a b) (
			return ($a + $b)
		)
		set res [add 10 25]
	`
	_, err := in.Eval(script)
	if err != nil {
		t.Fatalf("proc execution failed: %v", err)
	}

	resVal, ok := in.Scope.Get("res")
	if !ok || resVal.String() != "35" {
		t.Fatalf("expected res=35, got %v", resVal)
	}
}

func TestArrayAndDictLiteralsExecution(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)

	script := `
		set items { apple banana cherry }
		set first $items[0]
	`
	_, err := in.Eval(script)
	if err != nil {
		t.Fatalf("array literal eval failed: %v", err)
	}

	firstVal, ok := in.Scope.Get("first")
	if !ok || firstVal.String() != "apple" {
		t.Fatalf("expected first=apple, got %v", firstVal)
	}
}

package tests

import (
	"context"
	"testing"
	"pcl/pkg/ai"
	"pcl/pkg/interp"
	"pcl/pkg/repl"
	"pcl/pkg/services"
)

func TestREPLJobsBuiltin(t *testing.T) {
	loc := services.NewServiceLocator()
	loc.SetAI(ai.NewMockAIClient())
	in := interp.NewInterpreter(context.Background(), loc)
	_ = repl.NewREPL(in)
	if _, ok := in.Builtins["jobs"]; !ok {
		t.Fatal("jobs builtin not registered")
	}
	names := in.CommandNames()
	found := false
	for _, n := range names {
		if n == "cd" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("CommandNames missing cd: %v", names)
	}
}


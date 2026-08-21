package tests

import (
	"context"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestFluentRegex(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)

	in.Scope.Set("email", core.NewString("user@example.com"))

	// matches
	matched, err := in.Eval(`$email.matches("@example\\.com$")`)
	if err != nil || !matched.IsTruthy() {
		t.Fatalf("expected matches=true, got %v", matched)
	}

	// replace
	replaced, err := in.Eval(`$email.replace("example", "google")`)
	if err != nil || replaced.String() != "user@google.com" {
		t.Fatalf("expected replace='user@google.com', got %v", replaced)
	}

	// find
	found, err := in.Eval(`$email.find("[a-z]+")`)
	if err != nil || found.String() != "user" {
		t.Fatalf("expected find='user', got %v", found)
	}
}

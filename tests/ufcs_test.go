package tests

import (
	"context"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/parser"
	"pcl/pkg/services"
)

func TestParseAccessChain(t *testing.T) {
	expr := `x["tools"][0].exec`
	chain, err := parser.ParseAccessChain(expr)
	if err != nil {
		t.Fatalf("failed to parse access chain: %v", err)
	}

	if chain.Root != "x" {
		t.Fatalf("expected root 'x', got '%s'", chain.Root)
	}

	if len(chain.Ops) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(chain.Ops))
	}

	if chain.Ops[0].Type != parser.OpIndex || chain.Ops[0].Index != "tools" {
		t.Fatalf("op 0 mismatch: %v", chain.Ops[0])
	}
	if chain.Ops[1].Type != parser.OpIndex || chain.Ops[1].Index != "0" {
		t.Fatalf("op 1 mismatch: %v", chain.Ops[1])
	}
	if chain.Ops[2].Type != parser.OpField || chain.Ops[2].Name != "exec" {
		t.Fatalf("op 2 mismatch: %v", chain.Ops[2])
	}
}

func TestUFCSMethodChaining(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)

	script := `
		set msg "hello world"
		set upperMsg $msg.upper()
		set wordCount $msg.split(" ").len()
	`
	_, err := in.Eval(script)
	if err != nil {
		t.Fatalf("UFCS eval failed: %v", err)
	}

	upperVal, _ := in.Scope.Get("upperMsg")
	if upperVal.String() != "HELLO WORLD" {
		t.Fatalf("expected 'HELLO WORLD', got '%s'", upperVal.String())
	}

	countVal, _ := in.Scope.Get("wordCount")
	if countVal.String() != "2" {
		t.Fatalf("expected '2', got '%s'", countVal.String())
	}
}

func TestResponseFieldAccess(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)

	resp := &core.Response{
		Text:      "Hello from AI",
		Reasoning: "Analyzing prompt intent step by step",
		Model:     "deepseek-coder",
		ToolCalls: []*core.ToolCall{
			{
				ID:   "call_001",
				Name: "test_tool",
				Arguments: map[string]interface{}{"param": "val"},
				ExecFn: func(args map[string]interface{}) (*core.Value, error) {
					return core.NewString("tool executed successfully"), nil
				},
			},
		},
	}

	in.Scope.Set("x", core.NewResponse(resp))

	// Test x["response"]
	val1, err := in.Eval(`x["response"]`)
	if err != nil || val1.String() != "Hello from AI" {
		t.Fatalf("expected 'Hello from AI', got '%v'", val1)
	}

	// Test x.response
	val2, err := in.Eval(`$x.response`)
	if err != nil || val2.String() != "Hello from AI" {
		t.Fatalf("expected 'Hello from AI', got '%v'", val2)
	}

	// Test x.reasoning
	reasonVal, err := in.Eval(`$x.reasoning`)
	if err != nil || reasonVal.String() != "Analyzing prompt intent step by step" {
		t.Fatalf("expected 'Analyzing prompt intent step by step', got '%v'", reasonVal)
	}

	// Test x["thought"]
	thoughtVal, err := in.Eval(`x["thought"]`)
	if err != nil || thoughtVal.String() != "Analyzing prompt intent step by step" {
		t.Fatalf("expected 'Analyzing prompt intent step by step', got '%v'", thoughtVal)
	}

	// Test x["tools"][0].exec
	val3, err := in.Eval(`x["tools"][0].exec`)
	if err != nil || val3.String() != "tool executed successfully" {
		t.Fatalf("expected 'tool executed successfully', got '%v'", val3)
	}

	// Test x.keys()
	keysVal, err := in.Eval(`$x.keys().join(",")`)
	if err != nil || keysVal.String() != "response,reasoning,model,tools,steps,tools_used,files,usage" {
		t.Fatalf("expected keys 'response,reasoning,model,tools,steps,tools_used,files,usage', got '%v'", keysVal)
	}

	// Test x.json()
	jsonVal, err := in.Eval(`$x.json()`)
	if err != nil || jsonVal.String() == "" {
		t.Fatalf("expected valid JSON string from $x.json(), got '%v'", jsonVal)
	}
}

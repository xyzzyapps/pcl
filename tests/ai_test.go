package tests

import (
	"context"
	"encoding/json"
	"testing"
	"pcl/pkg/ai"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestExtractReasoning(t *testing.T) {
	r, text := ai.ExtractReasoning("hello", "chain of thought")
	if r != "chain of thought" || text != "hello" {
		t.Fatalf("reasoning_content: got %q / %q", r, text)
	}
	r, text = ai.ExtractReasoning("<think>secret</think>\nanswer", "")
	if r != "secret" || text != "answer" {
		t.Fatalf("think tags: got %q / %q", r, text)
	}
}

func TestFunctionParametersSchema(t *testing.T) {
	var schema map[string]interface{}

	if err := json.Unmarshal(ai.FunctionParametersSchema(nil), &schema); err != nil {
		t.Fatal(err)
	}
	if schema["type"] != "object" {
		t.Fatalf("nil params: expected type object, got %v", schema["type"])
	}
	if schema["properties"] == nil {
		t.Fatal("nil params: expected properties object")
	}

	raw := ai.FunctionParametersSchema(map[string]interface{}{"path": "string"})
	schema = map[string]interface{}{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["type"] != "object" {
		t.Fatalf("flat params: expected type object, got %v", schema["type"])
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("flat params: properties not an object: %v", schema["properties"])
	}
	path, ok := props["path"].(map[string]interface{})
	if !ok || path["type"] != "string" {
		t.Fatalf("flat params: path schema = %v", props["path"])
	}

	already := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"cmd": map[string]interface{}{"type": "string"},
		},
	}
	raw = ai.FunctionParametersSchema(already)
	schema = map[string]interface{}{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["type"] != "object" {
		t.Fatalf("passthrough: expected type object, got %v", schema["type"])
	}
}

func TestAIPromptAndResponseObject(t *testing.T) {
	loc := services.GetLocator()
	mockAI := ai.NewMockAIClient()
	loc.SetAI(mockAI)

	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterAIBuiltins(in)

	// Register a tool
	mockAI.RegisterTool("weather", "get current weather", nil, func(args map[string]interface{}) (*core.Value, error) {
		return core.NewString("Sunny 22C"), nil
	})

	// Execute prompt assignment
	script := `
		x = p( check weather in Paris )
	`
	_, err := in.Eval(script)
	if err != nil {
		t.Fatalf("prompt execution failed: %v", err)
	}

	xVal, ok := in.Scope.Get("x")
	if !ok || xVal.Type() != core.TypeResponse {
		t.Fatalf("expected response object x, got %v", xVal)
	}

	// Verify x["response"]
	respText, err := in.Eval(`x["response"]`)
	if err != nil || respText.String() == "" {
		t.Fatalf("x['response'] failed: %v", err)
	}

	if xVal.RespVal == nil || len(xVal.RespVal.ToolCalls) == 0 {
		t.Fatalf("expected tool calls on prompt response, steps=%v tools=%v", xVal.RespVal.Steps, xVal.RespVal.ToolCalls)
	}

	// Verify x["tools"][0].exec
	toolRes, err := in.Eval(`x["tools"][0].exec`)
	if err != nil || toolRes.String() != "Sunny 22C" {
		t.Fatalf("tool execution failed: expected 'Sunny 22C', got '%v' (err: %v)", toolRes, err)
	}
}

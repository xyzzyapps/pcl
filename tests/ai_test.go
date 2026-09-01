package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"pcl/pkg/ai"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/services"
	"strings"
	"testing"
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
	r, text = ai.ExtractReasoning("<think>a</think>\nmid\n<think>b</think>\nend", "")
	if r != "a\nb" || text != "mid\n\nend" && text != "mid\nend" {
		if r != "a\nb" {
			t.Fatalf("multi think reasoning: got %q", r)
		}
		if !strings.Contains(text, "mid") || !strings.Contains(text, "end") {
			t.Fatalf("multi think text: got %q", text)
		}
	}
	r, text = ai.ExtractReasoning("<think>same</think>shown", "same")
	if r != "same" || text != "shown" {
		t.Fatalf("dedupe: got %q / %q", r, text)
	}
	r, text = ai.ExtractReasoning("<think>unclosed remainder", "")
	if r != "unclosed remainder" || text != "" {
		t.Fatalf("unclosed: got %q / %q", r, text)
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

	last, ok := in.Scope.Get("_")
	if !ok || last.Type() != core.TypeResponse {
		t.Fatalf("expected _ to hold the prompt result, got %v", last)
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

func TestBarePromptStoresUnderscore(t *testing.T) {
	loc := services.NewServiceLocator()
	mockAI := ai.NewMockAIClient()
	loc.SetAI(mockAI)
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterAIBuiltins(in)

	_, err := in.Eval(`p( hello there )`)
	if err != nil {
		t.Fatalf("bare p() failed: %v", err)
	}
	last, ok := in.Scope.Get("_")
	if !ok || last == nil || last.Type() != core.TypeResponse {
		t.Fatalf("expected $_ to be a response after p(), got %v", last)
	}
	text, err := in.Eval(`$_.response`)
	if err != nil || text.String() == "" {
		t.Fatalf("$_.response failed: %v %v", text, err)
	}
}

func TestIsContextOverflow(t *testing.T) {
	if !ai.IsContextOverflow(fmt.Errorf("context_length_exceeded")) {
		t.Fatal("expected overflow detect")
	}
	if ai.IsContextOverflow(fmt.Errorf("connection refused")) {
		t.Fatal("did not expect overflow")
	}
}

func TestCompactMessagesShrinksHistory(t *testing.T) {
	mock := ai.NewMockAIClient()
	var msgs []*services.AIMessage
	msgs = append(msgs, &services.AIMessage{Role: "system", Content: "sys"})
	for i := 0; i < 12; i++ {
		msgs = append(msgs, &services.AIMessage{Role: "user", Content: "u"})
		msgs = append(msgs, &services.AIMessage{Role: "assistant", Content: "a"})
	}
	n0 := len(msgs)
	out, err := ai.CompactMessages(context.Background(), mock, msgs, "mock")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) >= n0 {
		t.Fatalf("expected shrink %d -> %d", n0, len(out))
	}
	if out[0].Role != "system" {
		t.Fatal("system prompt should remain first")
	}
}

func TestPromptCommandsShareSessionChat(t *testing.T) {
	loc := services.NewServiceLocator()
	mockAI := ai.NewMockAIClient()
	loc.SetAI(mockAI)
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterAIBuiltins(in)

	if _, err := in.Eval(`p( hello there )`); err != nil {
		t.Fatal(err)
	}
	n := len(in.Chat)
	if n < 3 {
		t.Fatalf("expected system+user+assistant in chat, got %d", n)
	}
	if _, err := in.Eval(`p( what did I just say )`); err != nil {
		t.Fatal(err)
	}
	if len(in.Chat) <= n {
		t.Fatalf("expected chat to grow across p() calls, %d -> %d", n, len(in.Chat))
	}
	users := 0
	for _, m := range in.Chat {
		if m.Role == "user" {
			users++
		}
	}
	if users < 2 {
		t.Fatalf("expected both prompts in session chat, users=%d", users)
	}
}

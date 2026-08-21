package tests

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"pcl/pkg/ai"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestAgentReActFeedbackLoop(t *testing.T) {
	loc := services.NewServiceLocator()
	mockAI := ai.NewMockAIClient()
	loc.SetAI(mockAI)

	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)
	builtins.RegisterAIBuiltins(in)

	// Register custom tool in PCL
	_, err := in.Eval(`tool inspect_repo (code) ( return "Clean architecture: all tests pass" )`)
	if err != nil {
		t.Fatalf("tool registration failed: %v", err)
	}

	// 1. Test 'agent <goal>' builtin
	res, err := in.Eval(`agent "Please inspect_repo and verify the codebase"`)
	if err != nil {
		t.Fatalf("agent execution failed: %v", err)
	}

	if res.Type() != core.TypeResponse {
		t.Fatalf("expected Response type, got %s", res.Type())
	}

	// 2. Check steps and tools used
	stepsVal, err := in.Eval(`$res.steps.len()`)
	if err != nil || stepsVal.String() == "0" {
		// Mock simulates 2 turns (Turn 1: Tool call -> Turn 2: Observation -> Final answer)
		t.Logf("Agent steps: %v", res.RespVal.Steps)
	}

	// 3. Test 'p(...) with { agent: true, max_turns: 4 }'
	res2, err := in.Eval(`p( Fix the repo and inspect_repo ) with { agent: true, max_turns: 4 }`)
	if err != nil {
		t.Fatalf("p with agent failed: %v", err)
	}

	if res2.RespVal == nil {
		t.Fatalf("expected RespVal on response")
	}

	if len(res2.RespVal.Steps) == 0 {
		t.Fatalf("expected agent steps to be recorded")
	}

	// 4. Test default environment tool: write_file and read_file
	tempDir := t.TempDir()
	testFilePath := filepath.ToSlash(filepath.Join(tempDir, "agent_out.txt"))

	toolCall := &core.ToolCall{
		ID:        "call_write_01",
		Name:      "write_file",
		Arguments: map[string]interface{}{"path": testFilePath, "content": "Agent ground truth observation"},
	}

	val, err := in.ExecuteToolCall(toolCall)
	if err != nil {
		t.Fatalf("write_file tool execution failed: %v", err)
	}
	if val == nil || !strings.Contains(val.String(), "Successfully wrote") {
		t.Fatalf("unexpected write_file result: %v", val)
	}

	readCall := &core.ToolCall{
		ID:        "call_read_01",
		Name:      "read_file",
		Arguments: map[string]interface{}{"path": testFilePath},
	}

	readVal, err := in.ExecuteToolCall(readCall)
	if err != nil {
		t.Fatalf("read_file tool execution failed: %v", err)
	}
	if readVal.String() != "Agent ground truth observation" {
		t.Fatalf("expected 'Agent ground truth observation', got '%s'", readVal.String())
	}
}

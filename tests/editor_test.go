package tests

import (
	"context"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/services"
	"pcl/pkg/shell"
)

func TestEditorResolution(t *testing.T) {
	fs := services.NewDefaultFSService()
	ed, err := shell.FindEditor(fs)
	if err != nil || ed == "" {
		t.Fatalf("failed finding editor: %v", err)
	}
}

func TestToolCallVimArgumentsExtraction(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)

	tc := &core.ToolCall{
		ID:   "call_write_01",
		Name: "write_file",
		Arguments: map[string]interface{}{
			"path":    "test_output.txt",
			"content": "Sample file content",
		},
	}

	resp := &core.Response{
		Text:      "Writing file",
		ToolCalls: []*core.ToolCall{tc},
	}

	respVal := core.NewResponse(resp)
	in.Scope.Set("x", respVal)

	// Verify x.tools[0].name and arguments access
	val, err := in.Eval(`$x.tools[0].name`)
	if err != nil || val.String() != "write_file" {
		t.Fatalf("expected tool name 'write_file', got '%v'", val)
	}

	pathVal, err := in.Eval(`$x.tools[0].args["path"]`)
	if err != nil || pathVal.String() != "test_output.txt" {
		t.Fatalf("expected path 'test_output.txt', got '%v'", pathVal)
	}
}

func TestMultiFileExtractionAndEditor(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)

	resp := &core.Response{
		Text: "Generated microservice:\n```go:main.go\npackage main\n```\n```go:server.go\npackage main\n```",
		Steps: []*core.AgentStep{
			{
				Turn: 1,
				ToolCalls: []*core.ToolCall{
					{
						ID:        "tc_01",
						Name:      "write_file",
						Arguments: map[string]interface{}{"path": "config.go", "content": "package main"},
					},
				},
			},
			{
				Turn: 2,
				ToolCalls: []*core.ToolCall{
					{
						ID:        "tc_02",
						Name:      "write_file",
						Arguments: map[string]interface{}{"path": "routes.go", "content": "package main"},
					},
				},
			},
		},
	}

	in.Scope.Set("res", core.NewResponse(resp))

	// 1. Test $res.files()
	filesVal, err := in.Eval(`$res.files()`)
	if err != nil {
		t.Fatalf("failed evaluating $res.files(): %v", err)
	}

	if filesVal.Type() != core.TypeList {
		t.Fatalf("expected TypeList, got %s", filesVal.Type())
	}

	countVal, err := in.Eval(`$res.files().len()`)
	if err != nil || countVal.String() != "4" {
		t.Fatalf("expected 4 files (config.go, routes.go, main.go, server.go), got %s", countVal)
	}

	// 2. Test $res.files().join(", ")
	joinVal, err := in.Eval(`$res.files().join(", ")`)
	if err != nil || joinVal.String() != "config.go, routes.go, main.go, server.go" {
		t.Fatalf("expected joined files, got %s", joinVal)
	}
}


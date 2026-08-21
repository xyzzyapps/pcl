package mcpctl

import (
	"context"
	"testing"
	"pcl/pkg/ai"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAttachInMemoryServer(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "echo", Version: "v0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping", Description: "pong"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "pong"}},
		}, nil, nil
	})
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "pcl-test", Version: "v0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	mock := ai.NewMockAIClient()
	m := NewManager()
	if err := m.AttachSession("echo", session, mock); err != nil {
		t.Fatal(err)
	}
	tools := m.Tools()
	if len(tools) != 1 || tools[0] != "echo_ping" {
		t.Fatalf("tools=%v", tools)
	}
	tc, ok := mock.GetTool("echo_ping")
	if !ok {
		t.Fatal("echo_ping not registered")
	}
	val, err := tc.ExecFn(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if val.String() != "pong" {
		t.Fatalf("got %q", val.String())
	}
	if err := m.Remove("echo", mock); err != nil {
		t.Fatal(err)
	}
	if _, ok := mock.GetTool("echo_ping"); ok {
		t.Fatal("tool should be unregistered")
	}
}

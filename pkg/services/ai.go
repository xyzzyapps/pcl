package services

import (
	"context"
	"io"
	"pcl/pkg/core"
)

// AIMessage represents a single message in a multi-turn conversation.
type AIMessage struct {
	Role       string           `json:"role"` // "system", "user", "assistant", "tool"
	Content    string           `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []*core.ToolCall `json:"tool_calls,omitempty"`
}

// AIRequest contains prompt parameters and registered tools.
type AIRequest struct {
	Prompt       string
	SystemPrompt string
	Model        string
	Temperature  float64
	MaxTokens    int
	Tools        []*core.ToolCall
	StreamWriter io.Writer
}

// AIMultiTurnRequest contains multi-turn conversation messages and tools.
type AIMultiTurnRequest struct {
	Messages     []*AIMessage
	Model        string
	Temperature  float64
	MaxTokens    int
	Tools        []*core.ToolCall
	StreamWriter io.Writer
}

// AIService defines the abstraction for AI model communication.
type AIService interface {
	Prompt(ctx context.Context, req *AIRequest) (*core.Response, error)
	PromptMessages(ctx context.Context, req *AIMultiTurnRequest) (*core.Response, error)
	RegisterTool(name string, description string, params map[string]interface{}, fn func(args map[string]interface{}) (*core.Value, error))
	UnregisterTool(name string)
	GetTool(name string) (*core.ToolCall, bool)
	ListTools() []*core.ToolCall
}

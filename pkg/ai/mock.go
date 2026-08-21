package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"pcl/pkg/core"
	"pcl/pkg/services"
)

// MockAIClient provides deterministic AI responses for testing and offline use.
type MockAIClient struct {
	mu            sync.RWMutex
	tools         map[string]*core.ToolCall
	defaultModel  string
	customHandler func(req *services.AIRequest) (*core.Response, error)
}

func NewMockAIClient() *MockAIClient {
	m := &MockAIClient{
		tools:        make(map[string]*core.ToolCall),
		defaultModel: "mock-gemini-2.5",
	}
	return m
}

func (m *MockAIClient) SetHandler(fn func(req *services.AIRequest) (*core.Response, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customHandler = fn
}

func (m *MockAIClient) RegisterTool(name string, description string, params map[string]interface{}, fn func(args map[string]interface{}) (*core.Value, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[name] = &core.ToolCall{
		ID:          fmt.Sprintf("call_%s_001", name),
		Name:        name,
		Description: description,
		Arguments:   params,
		ExecFn:      fn,
	}
}

func (m *MockAIClient) UnregisterTool(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tools, name)
}

func (m *MockAIClient) GetTool(name string) (*core.ToolCall, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tc, ok := m.tools[name]
	return tc, ok
}

func (m *MockAIClient) ListTools() []*core.ToolCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*core.ToolCall, 0, len(m.tools))
	for _, tc := range m.tools {
		list = append(list, tc)
	}
	return list
}

func (m *MockAIClient) Prompt(ctx context.Context, req *services.AIRequest) (*core.Response, error) {
	m.mu.RLock()
	handler := m.customHandler
	m.mu.RUnlock()

	if handler != nil {
		return handler(req)
	}

	promptLower := strings.ToLower(req.Prompt)

	// Check if prompt triggers any registered tool
	var toolCalls []*core.ToolCall
	for name, tc := range m.tools {
		if strings.Contains(promptLower, strings.ToLower(name)) || strings.Contains(promptLower, "tool") {
			toolCalls = append(toolCalls, &core.ToolCall{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: map[string]interface{}{"code": "sample code for review", "query": "pcl search query"},
				ExecFn:    tc.ExecFn,
			})
			break
		}
	}

	textResp := fmt.Sprintf("AI Response to: %s", strings.TrimSpace(req.Prompt))
	if len(toolCalls) > 0 {
		textResp = fmt.Sprintf("Invoking tool %s for request: %s", toolCalls[0].Name, req.Prompt)
	}

	if req.StreamWriter != nil {
		req.StreamWriter.Write([]byte(textResp + "\n"))
	}

	return &core.Response{
		Text:      textResp,
		ToolCalls: toolCalls,
		Usage: &core.TokenUsage{
			InputTokens:  core.RuneCount(req.Prompt),
			OutputTokens: core.RuneCount(textResp),
			TotalTokens:  core.RuneCount(req.Prompt) + core.RuneCount(textResp),
		},
		Model: req.Model,
	}, nil
}

// PromptMessages handles multi-turn conversation simulation for ReAct loops.
func (m *MockAIClient) PromptMessages(ctx context.Context, req *services.AIMultiTurnRequest) (*core.Response, error) {
	if len(req.Messages) == 0 {
		return &core.Response{Text: "Empty conversation", Model: req.Model}, nil
	}

	lastMsg := req.Messages[len(req.Messages)-1]

	// 1. If last message is tool result (Observation), produce final answer
	if lastMsg.Role == "tool" {
		ansText := fmt.Sprintf("Goal completed successfully. Observation verified: %s", lastMsg.Content)
		return &core.Response{
			Text:      ansText,
			Reasoning: "Observed execution output; verified task requirements are fully satisfied.",
			Usage: &core.TokenUsage{
				InputTokens:  core.RuneCount(lastMsg.Content),
				OutputTokens: core.RuneCount(ansText),
				TotalTokens:  core.RuneCount(lastMsg.Content) + core.RuneCount(ansText),
			},
			Model: req.Model,
		}, nil
	}

	// 2. If user message (Turn 1), check for tool calls
	var toolCalls []*core.ToolCall
	promptLower := strings.ToLower(lastMsg.Content)

	m.mu.RLock()
	toolsCopy := make(map[string]*core.ToolCall, len(m.tools))
	for k, v := range m.tools {
		toolsCopy[k] = v
	}
	m.mu.RUnlock()

	for name, tc := range toolsCopy {
		if strings.Contains(promptLower, strings.ToLower(name)) || strings.Contains(promptLower, "tool") || strings.Contains(promptLower, "sh") || strings.Contains(promptLower, "fix") || strings.Contains(promptLower, "run") {
			toolCalls = append(toolCalls, &core.ToolCall{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: map[string]interface{}{"code": "sample code for review", "cmd": "echo 'react execution test'"},
				ExecFn:    tc.ExecFn,
			})
			break
		}
	}

	if len(toolCalls) > 0 {
		return &core.Response{
			Text:      fmt.Sprintf("Invoking tool %s for agent goal: %s", toolCalls[0].Name, lastMsg.Content),
			Reasoning: fmt.Sprintf("Planning execution: calling %s to gather environment feedback.", toolCalls[0].Name),
			ToolCalls: toolCalls,
			Usage: &core.TokenUsage{
				InputTokens:  core.RuneCount(lastMsg.Content),
				OutputTokens: 50,
				TotalTokens:  core.RuneCount(lastMsg.Content) + 50,
			},
			Model: req.Model,
		}, nil
	}

	// Standard response without tool call
	ansText := fmt.Sprintf("Agent completed task: %s", lastMsg.Content)
	return &core.Response{
		Text:      ansText,
		Reasoning: "Direct analysis complete without tool invocation.",
		Usage: &core.TokenUsage{
			InputTokens:  core.RuneCount(lastMsg.Content),
			OutputTokens: core.RuneCount(ansText),
			TotalTokens:  core.RuneCount(lastMsg.Content) + core.RuneCount(ansText),
		},
		Model: req.Model,
	}, nil
}

package ai

import (
	"context"
	"fmt"
	"io"
	"strings"
	"pcl/pkg/core"
	"pcl/pkg/services"
)

// ToolExecutor is the interface for executing AI tool calls within the runtime.
type ToolExecutor interface {
	ExecuteToolCall(call *core.ToolCall) (*core.Value, error)
	ListTools() []*core.ToolCall
}

// AgentOptions contains parameters for the autonomous ReAct agent loop.
type AgentOptions struct {
	MaxTurns     int
	SystemPrompt string
	Model        string
	StreamWriter io.Writer
	OnStep       func(step *core.AgentStep)
}

// DefaultAgentOptions provides sensible defaults for agent execution.
func DefaultAgentOptions() AgentOptions {
	return AgentOptions{
		MaxTurns: 10,
	}
}

// RunReActLoop executes the autonomous Reason + Act + Observe loop grounded in environment feedback.
func RunReActLoop(ctx context.Context, aiSvc services.AIService, exec ToolExecutor, goal string, opts AgentOptions) (*core.Response, error) {
	if opts.MaxTurns <= 0 {
		opts.MaxTurns = 10
	}

	systemPrompt := opts.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "You are an autonomous AI coding agent. You can use available tools to inspect the environment, run commands, and execute tests. Always ground your reasoning in concrete tool output. When you have satisfied the goal, output your final answer without requesting further tools."
	}

	messages := []*services.AIMessage{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: goal,
		},
	}

	var allSteps []*core.AgentStep
	var allToolCalls []*core.ToolCall
	var totalUsage core.TokenUsage
	var lastModel string
	var finalResponse *core.Response

	for turn := 1; turn <= opts.MaxTurns; turn++ {
		tools := exec.ListTools()
		req := &services.AIMultiTurnRequest{
			Messages:     messages,
			Tools:        tools,
			Model:        opts.Model,
			StreamWriter: opts.StreamWriter,
		}

		resp, err := aiSvc.PromptMessages(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("agent turn %d error: %w", turn, err)
		}

		if resp.Usage != nil {
			totalUsage.InputTokens += resp.Usage.InputTokens
			totalUsage.OutputTokens += resp.Usage.OutputTokens
			totalUsage.TotalTokens += resp.Usage.TotalTokens
		}
		lastModel = resp.Model

		step := &core.AgentStep{
			Turn:      turn,
			Thought:   resp.Reasoning,
			ToolCalls: resp.ToolCalls,
			Response:  resp.Text,
		}

		if len(resp.ToolCalls) > 0 {
			allToolCalls = append(allToolCalls, resp.ToolCalls...)
		}

		// If no tool calls were requested, the model reached a final answer
		if len(resp.ToolCalls) == 0 {
			allSteps = append(allSteps, step)
			if opts.OnStep != nil {
				opts.OnStep(step)
			}
			finalResponse = resp
			break
		}

		for i, tc := range resp.ToolCalls {
			if tc.ID == "" {
				tc.ID = fmt.Sprintf("call_%s_%d_%d", tc.Name, turn, i)
			}
		}

		messages = append(messages, &services.AIMessage{
			Role:      "assistant",
			Content:   resp.Text,
			ToolCalls: resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			resultVal, execErr := exec.ExecuteToolCall(tc)
			obsStr := ""
			if execErr != nil {
				obsStr = fmt.Sprintf("Error: %v", execErr)
			} else if resultVal != nil {
				obsStr = resultVal.String()
			}

			step.Observations = append(step.Observations, obsStr)

			messages = append(messages, &services.AIMessage{
				Role:       "tool",
				Name:       tc.Name,
				ToolCallID: tc.ID,
				Content:    obsStr,
			})
		}

		allSteps = append(allSteps, step)
		if opts.OnStep != nil {
			opts.OnStep(step)
		}
	}

	if finalResponse == nil {
		// Reached max turns
		finalResponse = &core.Response{
			Text:  fmt.Sprintf("Agent reached max turn limit (%d).", opts.MaxTurns),
			Model: lastModel,
		}
	}

	// Combine all step thoughts into final response reasoning
	var thoughts []string
	for _, st := range allSteps {
		if st.Thought != "" {
			thoughts = append(thoughts, fmt.Sprintf("[Turn %d]: %s", st.Turn, st.Thought))
		}
	}

	finalResponse.Steps = allSteps
	finalResponse.Usage = &totalUsage
	if len(allToolCalls) > 0 && len(finalResponse.ToolCalls) == 0 {
		finalResponse.ToolCalls = allToolCalls
	}
	if len(thoughts) > 0 {
		finalResponse.Reasoning = strings.Join(thoughts, "\n")
	} else if finalResponse.Reasoning == "" {
		// keep last-turn reasoning already on finalResponse
	}

	return finalResponse, nil
}

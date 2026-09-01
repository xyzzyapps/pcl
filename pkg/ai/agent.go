package ai

import (
	"context"
	"fmt"
	"io"
	"pcl/pkg/core"
	"pcl/pkg/services"
	"sort"
	"strings"
	"sync"
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
	Chat         *[]*services.AIMessage
	OnStep       func(step *core.AgentStep)
}

// DefaultAgentOptions provides sensible defaults for agent execution.
func DefaultAgentOptions() AgentOptions {
	return AgentOptions{
		MaxTurns: 50,
	}
}

// RunReActLoop executes the autonomous Reason + Act + Observe loop grounded in environment feedback.
func RunReActLoop(ctx context.Context, aiSvc services.AIService, exec ToolExecutor, goal string, opts AgentOptions) (*core.Response, error) {
	if opts.MaxTurns <= 0 {
		opts.MaxTurns = 50
	}

	systemPrompt := opts.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "You are an autonomous AI coding agent. You can use available tools to inspect the environment, run commands, and execute tests. Always ground your reasoning in concrete tool output. When you have satisfied the goal, output your final answer without requesting further tools."
	}

	messages := seedChat(opts.Chat, systemPrompt, goal)

	var allSteps []*core.AgentStep
	var allToolCalls []*core.ToolCall
	var totalUsage core.TokenUsage
	var lastModel string
	var finalResponse *core.Response

	compacted := false
	for turn := 1; turn <= opts.MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		tools := exec.ListTools()
		req := &services.AIMultiTurnRequest{
			Messages:     messages,
			Tools:        tools,
			Model:        opts.Model,
			StreamWriter: opts.StreamWriter,
		}

		resp, err := aiSvc.PromptMessages(ctx, req)
		if err != nil && IsContextOverflow(err) && !compacted {
			compacted = true
			cm, cErr := CompactMessages(ctx, aiSvc, messages, opts.Model)
			if cErr != nil {
				return nil, fmt.Errorf("agent turn %d error: %w (compact failed: %v)", turn, err, cErr)
			}
			messages = cm
			saveChat(opts.Chat, messages)
			if opts.StreamWriter != nil {
				fmt.Fprintln(opts.StreamWriter, "context window full — compacted session, retrying")
			}
			turn--
			continue
		}
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

		if len(resp.ToolCalls) == 0 {
			messages = append(messages, &services.AIMessage{
				Role:      "assistant",
				Content:   resp.Text,
				Reasoning: resp.Reasoning,
			})
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
			Reasoning: resp.Reasoning,
			ToolCalls: resp.ToolCalls,
		})

		obs := runToolCalls(exec, resp.ToolCalls, opts.StreamWriter)
		step.Observations = obs
		for i, tc := range resp.ToolCalls {
			messages = append(messages, &services.AIMessage{
				Role:       "tool",
				Name:       tc.Name,
				ToolCallID: tc.ID,
				Content:    obs[i],
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

	saveChat(opts.Chat, messages)
	return finalResponse, nil
}

func seedChat(chat *[]*services.AIMessage, systemPrompt, goal string) []*services.AIMessage {
	user := &services.AIMessage{Role: "user", Content: goal}
	if chat == nil || len(*chat) == 0 {
		return []*services.AIMessage{
			{Role: "system", Content: systemPrompt},
			user,
		}
	}
	msgs := append([]*services.AIMessage{}, *chat...)
	if msgs[0] != nil && msgs[0].Role == "system" {
		msgs[0] = &services.AIMessage{Role: "system", Content: systemPrompt}
	} else {
		msgs = append([]*services.AIMessage{{Role: "system", Content: systemPrompt}}, msgs...)
	}
	return append(msgs, user)
}

func saveChat(chat *[]*services.AIMessage, messages []*services.AIMessage) {
	if chat == nil {
		return
	}
	*chat = messages
}

func runToolCalls(exec ToolExecutor, calls []*core.ToolCall, stream io.Writer) []string {
	obs := make([]string, len(calls))
	if len(calls) == 0 {
		return obs
	}
	if len(calls) == 1 {
		obs[0] = runOneTool(exec, calls[0], stream)
		return obs
	}
	var wg sync.WaitGroup
	for i, tc := range calls {
		wg.Add(1)
		go func(i int, tc *core.ToolCall) {
			defer wg.Done()
			obs[i] = runOneTool(exec, tc, stream)
		}(i, tc)
	}
	wg.Wait()
	return obs
}

func runOneTool(exec ToolExecutor, tc *core.ToolCall, stream io.Writer) string {
	if stream != nil {
		traceTool(stream, tc)
	}
	resultVal, execErr := exec.ExecuteToolCall(tc)
	obsStr := ""
	if execErr != nil {
		obsStr = fmt.Sprintf("Error: %v", execErr)
	} else if resultVal != nil {
		obsStr = resultVal.String()
	}
	if stream != nil {
		traceObservation(stream, obsStr)
	}
	return obsStr
}

func compactArgs(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+compactOneLine(fmt.Sprint(args[k]), 0))
	}
	return strings.Join(parts, "  ")
}

func compactOneLine(s string, max int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	rs := []rune(s)
	if max > 0 && len(rs) > max {
		return string(rs[:max]) + "…"
	}
	return s
}

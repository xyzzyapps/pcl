package ai

import (
	"encoding/json"
	"fmt"
	"strings"
	"pcl/pkg/core"

	openai "github.com/sashabaranov/go-openai"
)

// ExtractReasoning pulls model thought from API reasoning_content and <think> tags.
func ExtractReasoning(content, reasoningContent string) (reasoning, text string) {
	text = content
	if strings.Contains(content, "<think>") && strings.Contains(content, "</think>") {
		start := strings.Index(content, "<think>")
		end := strings.Index(content, "</think>")
		if start != -1 && end != -1 && end > start {
			tag := strings.TrimSpace(content[start+len("<think>") : end])
			text = strings.TrimSpace(content[:start] + content[end+len("</think>"):])
			if tag != "" {
				reasoning = tag
			}
		}
	}
	rc := strings.TrimSpace(reasoningContent)
	if rc != "" {
		if reasoning != "" && rc != reasoning {
			reasoning = rc + "\n" + reasoning
		} else {
			reasoning = rc
		}
	}
	return reasoning, text
}

func toolCallsFromOpenAI(c *UniversalAIClient, msg openai.ChatCompletionMessage, extra []*core.ToolCall) []*core.ToolCall {
	var out []*core.ToolCall
	for _, tc := range msg.ToolCalls {
		out = append(out, pclToolCall(c, tc.ID, tc.Function.Name, tc.Function.Arguments, extra))
	}
	if len(out) == 0 && msg.FunctionCall != nil && msg.FunctionCall.Name != "" {
		out = append(out, pclToolCall(c, "call_"+msg.FunctionCall.Name, msg.FunctionCall.Name, msg.FunctionCall.Arguments, extra))
	}
	return out
}

func pclToolCall(c *UniversalAIClient, id, name, argJSON string, extra []*core.ToolCall) *core.ToolCall {
	argMap := map[string]interface{}{}
	argJSON = strings.TrimSpace(argJSON)
	if argJSON != "" && argJSON != "null" {
		_ = json.Unmarshal([]byte(argJSON), &argMap)
	}
	if argMap == nil {
		argMap = map[string]interface{}{}
	}
	if id == "" {
		id = fmt.Sprintf("call_%s", name)
	}
	tc := &core.ToolCall{
		ID:        id,
		Name:      name,
		Arguments: argMap,
	}
	if extra != nil {
		for _, t := range extra {
			if t != nil && t.Name == name && t.ExecFn != nil {
				tc.ExecFn = t.ExecFn
				if tc.Description == "" {
					tc.Description = t.Description
				}
				break
			}
		}
	}
	if tc.ExecFn == nil && c != nil {
		if reg, ok := c.GetTool(name); ok {
			tc.ExecFn = reg.ExecFn
			if tc.Description == "" {
				tc.Description = reg.Description
			}
		}
	}
	return tc
}

func responseFromChoice(c *UniversalAIClient, choice openai.ChatCompletionChoice, usage openai.Usage, model string, extra []*core.ToolCall) *core.Response {
	msg := choice.Message
	reasoning, cleanText := ExtractReasoning(msg.Content, msg.ReasoningContent)
	return &core.Response{
		Text:      cleanText,
		Reasoning: reasoning,
		ToolCalls: toolCallsFromOpenAI(c, msg, extra),
		Usage: &core.TokenUsage{
			InputTokens:  usage.PromptTokens,
			OutputTokens: usage.CompletionTokens,
			TotalTokens:  usage.TotalTokens,
		},
		Model: model,
		Raw:   choice,
	}
}

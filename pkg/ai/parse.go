package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"pcl/pkg/core"

	openai "github.com/sashabaranov/go-openai"
)

var thinkBlockRE = regexp.MustCompile(`(?is)<think(?:ing)?>\s*([\s\S]*?)\s*</think(?:ing)?>`)
var thinkOpenRE = regexp.MustCompile(`(?is)<think(?:ing)?>`)

// ExtractReasoning splits API thought from user-visible text.
// Sources, in order: reasoning_content (DeepSeek / Kimi / OpenCode), then
// every <think> / <thinking> block in content (Qwen and similar). Duplicate
// bodies are kept once. Visible text is content with those tags removed.
func ExtractReasoning(content, reasoningContent string) (reasoning, text string) {
	var blocks []string
	text = thinkBlockRE.ReplaceAllStringFunc(content, func(m string) string {
		sub := thinkBlockRE.FindStringSubmatch(m)
		if len(sub) > 1 {
			if t := strings.TrimSpace(sub[1]); t != "" {
				blocks = append(blocks, t)
			}
		}
		return ""
	})
	if loc := thinkOpenRE.FindStringIndex(text); loc != nil {
		rest := text[loc[1]:]
		if t := strings.TrimSpace(rest); t != "" {
			blocks = append(blocks, t)
		}
		text = text[:loc[0]]
	}
	text = strings.TrimSpace(text)

	rc := strings.TrimSpace(reasoningContent)
	var parts []string
	seen := map[string]bool{}
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		parts = append(parts, s)
	}
	add(rc)
	for _, b := range blocks {
		add(b)
	}
	return strings.Join(parts, "\n"), text
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

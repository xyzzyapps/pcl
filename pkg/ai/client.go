package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"pcl/pkg/core"
	"pcl/pkg/services"

	openai "github.com/sashabaranov/go-openai"
)

// UniversalAIClient uses the go-openai SDK for OpenCode Go and OpenAI-compatible endpoints.
type UniversalAIClient struct {
	mu       sync.RWMutex
	provider string
	apiBase  string
	apiKey   string
	tools    map[string]*core.ToolCall
}

func NewUniversalAIClient(provider, apiBase, apiKey string) *UniversalAIClient {
	if provider == "" {
		provider = "opencode"
	}
	if apiBase == "" && (provider == "opencode" || provider == "openai") {
		apiBase = "https://opencode.ai/zen/go/v1"
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENCODE_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
	}

	return &UniversalAIClient{
		provider: provider,
		apiBase:  apiBase,
		apiKey:   apiKey,
		tools:    make(map[string]*core.ToolCall),
	}
}

// Backward compatibility constructor
func NewGeminiAIClient(apiKey string) *UniversalAIClient {
	return NewUniversalAIClient("opencode", "", apiKey)
}

func (c *UniversalAIClient) RegisterTool(name string, description string, params map[string]interface{}, fn func(args map[string]interface{}) (*core.Value, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tools[name] = &core.ToolCall{
		ID:          fmt.Sprintf("call_%s", name),
		Name:        name,
		Description: description,
		Arguments:   params,
		ExecFn:      fn,
	}
}

func (c *UniversalAIClient) UnregisterTool(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tools, name)
}

func (c *UniversalAIClient) GetTool(name string) (*core.ToolCall, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	tc, ok := c.tools[name]
	return tc, ok
}

func (c *UniversalAIClient) ListTools() []*core.ToolCall {
	c.mu.RLock()
	defer c.mu.RUnlock()
	list := make([]*core.ToolCall, 0, len(c.tools))
	for _, tc := range c.tools {
		list = append(list, tc)
	}
	return list
}

func (c *UniversalAIClient) Prompt(ctx context.Context, req *services.AIRequest) (*core.Response, error) {
	c.mu.RLock()
	apiBase := c.apiBase
	apiKey := c.apiKey
	c.mu.RUnlock()

	// Check runtime config overrides
	cfg := services.GetLocator().Config()
	if b := cfg.Get("api_base"); b != "" {
		apiBase = b
	}
	if k := cfg.Get("api_key"); k != "" && !strings.HasPrefix(k, "$") {
		apiKey = k
	} else if strings.HasPrefix(k, "$") {
		apiKey = os.Getenv(strings.TrimPrefix(k, "$"))
	}

	if apiKey == "" {
		apiKey = os.Getenv("OPENCODE_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
	}

	// Fallback to deterministic mock AI if no API key is configured
	if apiKey == "" {
		mock := NewMockAIClient()
		for _, t := range req.Tools {
			mock.RegisterTool(t.Name, "", t.Arguments, t.ExecFn)
		}
		return mock.Prompt(ctx, req)
	}

	if apiBase == "" {
		apiBase = "https://opencode.ai/zen/go/v1"
	}

	// Configure client via official go-openai SDK
	clientConfig := openai.DefaultConfig(apiKey)
	clientConfig.BaseURL = strings.TrimSuffix(apiBase, "/")
	client := openai.NewClientWithConfig(clientConfig)

	model := req.Model
	if model == "" {
		model = "deepseek-coder"
	}

	// Build messages
	messages := make([]openai.ChatCompletionMessage, 0)
	if req.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: req.SystemPrompt,
		})
	}
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: req.Prompt,
	})

	chatReq := openai.ChatCompletionRequest{
		Model:       model,
		Messages:    messages,
		Temperature: float32(req.Temperature),
	}

	if len(req.Tools) > 0 {
		chatReq.Tools = openaiToolsFromPCL(req.Tools)
		chatReq.ToolChoice = "auto"
	}

	if req.StreamWriter != nil {
		return c.streamCompletion(ctx, client, chatReq, req.StreamWriter, req.Tools)
	}

	resp, err := client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("OpenCode API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return &core.Response{Text: "", Model: model}, nil
	}

	out := responseFromChoice(c, resp.Choices[0], resp.Usage, resp.Model, req.Tools)
	out.Raw = resp
	return out, nil
}

// PromptMessages executes multi-turn conversations for ReAct agent loops.
func (c *UniversalAIClient) PromptMessages(ctx context.Context, req *services.AIMultiTurnRequest) (*core.Response, error) {
	c.mu.RLock()
	apiBase := c.apiBase
	apiKey := c.apiKey
	c.mu.RUnlock()

	cfg := services.GetLocator().Config()
	if b := cfg.Get("api_base"); b != "" {
		apiBase = b
	}
	if k := cfg.Get("api_key"); k != "" && !strings.HasPrefix(k, "$") {
		apiKey = k
	} else if strings.HasPrefix(k, "$") {
		apiKey = os.Getenv(strings.TrimPrefix(k, "$"))
	}

	if apiKey == "" {
		apiKey = os.Getenv("OPENCODE_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
	}

	if apiKey == "" {
		mock := NewMockAIClient()
		for _, t := range req.Tools {
			mock.RegisterTool(t.Name, "", t.Arguments, t.ExecFn)
		}
		return mock.PromptMessages(ctx, req)
	}

	if apiBase == "" {
		apiBase = "https://opencode.ai/zen/go/v1"
	}

	clientConfig := openai.DefaultConfig(apiKey)
	clientConfig.BaseURL = strings.TrimSuffix(apiBase, "/")
	client := openai.NewClientWithConfig(clientConfig)

	model := req.Model
	if model == "" {
		model = "deepseek-coder"
	}

	messages := make([]openai.ChatCompletionMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		msg := openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
			Name:    m.Name,
		}
		if m.ToolCallID != "" {
			msg.ToolCallID = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			var tcList []openai.ToolCall
			for _, tc := range m.ToolCalls {
				argBytes, _ := json.Marshal(tc.Arguments)
				if len(argBytes) == 0 {
					argBytes = []byte("{}")
				}
				id := tc.ID
				if id == "" {
					id = "call_" + tc.Name
				}
				tcList = append(tcList, openai.ToolCall{
					ID:   id,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: string(argBytes),
					},
				})
			}
			msg.ToolCalls = tcList
		}
		messages = append(messages, msg)
	}

	chatReq := openai.ChatCompletionRequest{
		Model:       model,
		Messages:    messages,
		Temperature: float32(req.Temperature),
	}

	if len(req.Tools) > 0 {
		chatReq.Tools = openaiToolsFromPCL(req.Tools)
		chatReq.ToolChoice = "auto"
	}

	if req.StreamWriter != nil {
		return c.streamCompletion(ctx, client, chatReq, req.StreamWriter, req.Tools)
	}

	resp, err := client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("OpenCode API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return &core.Response{Text: "", Model: model}, nil
	}

	out := responseFromChoice(c, resp.Choices[0], resp.Usage, resp.Model, req.Tools)
	out.Raw = resp
	return out, nil
}

func (c *UniversalAIClient) streamCompletion(ctx context.Context, client *openai.Client, chatReq openai.ChatCompletionRequest, w io.Writer, extra []*core.ToolCall) (*core.Response, error) {
	stream, err := client.CreateChatCompletionStream(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("OpenCode stream error: %w", err)
	}
	defer stream.Close()

	var fullContent, fullReasoning strings.Builder
	acc := make(map[int]*openai.ToolCall)
	model := chatReq.Model
	var usage openai.Usage

	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("stream read error: %w", err)
		}
		if response.Model != "" {
			model = response.Model
		}
		if response.Usage != nil {
			usage = *response.Usage
		}
		if len(response.Choices) == 0 {
			continue
		}
		delta := response.Choices[0].Delta
		if delta.ReasoningContent != "" {
			fullReasoning.WriteString(delta.ReasoningContent)
		}
		if delta.Content != "" {
			fullContent.WriteString(delta.Content)
			_, _ = w.Write([]byte(delta.Content))
		}
		for _, tc := range delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			cur := acc[idx]
			if cur == nil {
				cp := tc
				acc[idx] = &cp
				continue
			}
			if tc.ID != "" {
				cur.ID = tc.ID
			}
			if tc.Type != "" {
				cur.Type = tc.Type
			}
			if tc.Function.Name != "" {
				cur.Function.Name = tc.Function.Name
			}
			cur.Function.Arguments += tc.Function.Arguments
		}
	}
	_, _ = w.Write([]byte("\n"))

	msg := openai.ChatCompletionMessage{
		Content:          fullContent.String(),
		ReasoningContent: fullReasoning.String(),
	}
	for i := 0; i < len(acc); i++ {
		if tc, ok := acc[i]; ok {
			msg.ToolCalls = append(msg.ToolCalls, *tc)
		}
	}

	reasoning, text := ExtractReasoning(msg.Content, msg.ReasoningContent)
	return &core.Response{
		Text:      text,
		Reasoning: reasoning,
		ToolCalls: toolCallsFromOpenAI(c, msg, extra),
		Usage: &core.TokenUsage{
			InputTokens:  usage.PromptTokens,
			OutputTokens: usage.CompletionTokens,
			TotalTokens:  usage.TotalTokens,
		},
		Model: model,
	}, nil
}

func openaiToolsFromPCL(tools []*core.ToolCall) []openai.Tool {
	toolsList := make([]openai.Tool, 0, len(tools))
	for _, tc := range tools {
		desc := tc.Description
		if desc == "" {
			desc = fmt.Sprintf("Tool %s", tc.Name)
		}
		toolsList = append(toolsList, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        tc.Name,
				Description: desc,
				Parameters:  FunctionParametersSchema(tc.Arguments),
			},
		})
	}
	return toolsList
}



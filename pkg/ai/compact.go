package ai

import (
	"context"
	"fmt"
	"pcl/pkg/services"
	"strings"
)

// IsContextOverflow reports API errors that mean the prompt no longer fits.
func IsContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	keys := []string{
		"context_length",
		"context window",
		"maximum context",
		"too many tokens",
		"prompt is too long",
		"reduce the length",
		"context_length_exceeded",
		"token limit",
		"max context",
		"please reduce",
	}
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// CompactMessages replaces the middle of a session with a model-written
// summary so later p() turns still fit. System prompt and the newest
// messages are kept verbatim. History is not capped except by this call.
func CompactMessages(ctx context.Context, aiSvc services.AIService, messages []*services.AIMessage, model string) ([]*services.AIMessage, error) {
	if aiSvc == nil {
		return messages, fmt.Errorf("compact: no AI service")
	}
	if len(messages) < 6 {
		return messages, nil
	}

	sys := messages[0]
	keep := 6
	if keep >= len(messages) {
		return messages, nil
	}
	mid := messages[1 : len(messages)-keep]
	tail := messages[len(messages)-keep:]

	var b strings.Builder
	b.WriteString("Compress this coding-agent session so work can continue in a new context window.\n")
	b.WriteString("Keep: goals, decisions, file paths, commands, errors, test status, unfinished work.\n")
	b.WriteString("Drop: duplicate tool dumps, chit-chat, repeated file contents.\n")
	b.WriteString("Write a dense summary only.\n\n")
	for _, m := range mid {
		if m == nil {
			continue
		}
		fmt.Fprintf(&b, "## %s", m.Role)
		if m.Name != "" {
			fmt.Fprintf(&b, " %s", m.Name)
		}
		b.WriteByte('\n')
		body := m.Content
		if m.Reasoning != "" {
			body = m.Reasoning + "\n" + body
		}
		if len(m.ToolCalls) > 0 {
			var names []string
			for _, tc := range m.ToolCalls {
				if tc != nil {
					names = append(names, tc.Name)
				}
			}
			if len(names) > 0 {
				fmt.Fprintf(&b, "tools: %s\n", strings.Join(names, ", "))
			}
		}
		b.WriteString(trimRunes(body, 4000))
		b.WriteString("\n\n")
	}

	resp, err := aiSvc.Prompt(ctx, &services.AIRequest{
		Prompt:       b.String(),
		SystemPrompt: "You compress agent session history for another coding agent. Output only the summary.",
		Model:        model,
	})
	if err != nil {
		return messages, fmt.Errorf("compact: %w", err)
	}
	summary := strings.TrimSpace("")
	if resp != nil {
		summary = strings.TrimSpace(resp.Text)
	}
	if summary == "" {
		summary = "(empty compact summary)"
	}

	out := []*services.AIMessage{
		sys,
		{Role: "user", Content: "[compacted session]\n" + summary},
		{Role: "assistant", Content: "Continuing from the compacted session."},
	}
	out = append(out, tail...)
	return out, nil
}

func trimRunes(s string, max int) string {
	rs := []rune(s)
	if max <= 0 || len(rs) <= max {
		return s
	}
	return string(rs[:max]) + "\n…"
}

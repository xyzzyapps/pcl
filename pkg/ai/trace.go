package ai

import (
	"fmt"
	"io"
	"os"
	"strings"
	"pcl/pkg/core"
)

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	st, err := os.Stderr.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func paint(code, s string) string {
	if !colorEnabled() || s == "" {
		return s
	}
	return code + s + "\033[0m"
}

func traceTool(w io.Writer, tc *core.ToolCall) {
	if w == nil || tc == nil {
		return
	}
	head := toolHeadline(tc)
	dot := paint("\033[1;36m", "●")
	name := paint("\033[1m", head)
	fmt.Fprintf(w, "\n%s %s\n", dot, name)
}

func traceObservation(w io.Writer, obs string) {
	if w == nil {
		return
	}
	if strings.TrimSpace(obs) == "" {
		return
	}
	obs = strings.TrimRight(obs, "\n")
	for _, ln := range strings.Split(obs, "\n") {
		fmt.Fprintf(w, "  %s %s\n", paint("\033[2m", "⎿"), paint("\033[2m", ln))
	}
}

func traceThought(w io.Writer, thought string) {
	if w == nil {
		return
	}
	if thought == "" {
		return
	}
	thought = strings.TrimRight(thought, "\n")
	fmt.Fprintln(w)
	for _, ln := range strings.Split(thought, "\n") {
		fmt.Fprintf(w, "%s\n", paint("\033[2;3m", ln))
	}
}

func traceAnswer(w io.Writer, text string) {
	if w == nil {
		return
	}
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, text)
}

func toolHeadline(tc *core.ToolCall) string {
	if tc.Arguments == nil {
		return tc.Name
	}
	switch tc.Name {
	case "write_file", "read_file":
		if p, ok := tc.Arguments["path"]; ok {
			return tc.Name + "  " + fmt.Sprint(p)
		}
	case "sh":
		if c, ok := tc.Arguments["cmd"]; ok {
			return "sh  " + fmt.Sprint(c)
		}
		if c, ok := tc.Arguments["command"]; ok {
			return "sh  " + fmt.Sprint(c)
		}
	}
	return strings.TrimSpace(tc.Name + "  " + compactArgs(tc.Arguments))
}

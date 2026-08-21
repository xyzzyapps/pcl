package repl

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Completer implements github.com/chzyer/readline.AutoCompleter.
type Completer struct {
	repl *REPL
}

func (c *Completer) Do(line []rune, pos int) ([][]rune, int) {
	if pos > len(line) {
		pos = len(line)
	}
	s := string(line[:pos])
	start := lastWordStart(s)
	prefix := s[start:]
	preLine := strings.TrimSpace(s[:start])

	cands := c.candidates(preLine, prefix)
	preRunes := []rune(prefix)
	var out [][]rune
	seen := map[string]bool{}
	for _, cand := range cands {
		candRunes := []rune(cand)
		if len(candRunes) < len(preRunes) {
			continue
		}
		if !strings.EqualFold(string(candRunes[:len(preRunes)]), prefix) {
			continue
		}
		suf := string(candRunes[len(preRunes):])
		if seen[suf] {
			continue
		}
		seen[suf] = true
		out = append(out, []rune(suf))
	}
	return out, len(preRunes)
}

func (c *Completer) candidates(preLine, prefix string) []string {
	var list []string
	firstWord := preLine == ""
	if strings.HasPrefix(prefix, "$") {
		return c.variables(prefix)
	}
	if firstWord {
		list = append(list, c.commands()...)
	}
	if firstWord || looksLikePathArg(preLine) || strings.ContainsAny(prefix, "/\\.") {
		list = append(list, c.files(prefix)...)
	}
	if !firstWord && strings.HasPrefix(preLine, "ai_config") {
		list = append(list, "provider", "api_base", "api_key", "model", "temperature",
			"system_prompt", "prompt", "multiline_prompt")
	}
	sort.Strings(list)
	return list
}

func (c *Completer) commands() []string {
	var names []string
	if c.repl == nil || c.repl.in == nil {
		return names
	}
	return c.repl.in.CommandNames()
}

func (c *Completer) variables(prefix string) []string {
	if c.repl == nil || c.repl.in == nil {
		return nil
	}
	all := c.repl.in.Scope.GetAll()
	var names []string
	for k := range all {
		names = append(names, "$"+k)
	}
	sort.Strings(names)
	_ = prefix
	return names
}

func (c *Completer) files(prefix string) []string {
	dir := "."
	base := prefix
	if prefix != "" {
		if i := strings.LastIndexAny(prefix, "/\\"); i >= 0 {
			dir = prefix[:i]
			if dir == "" {
				dir = string(prefix[i])
			}
			base = prefix[i+1:]
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	if len(entries) > 400 {
		entries = entries[:400]
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if base != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(base)) {
			continue
		}
		joined := name
		if dir != "." {
			joined = filepath.Join(dir, name)
		}
		if e.IsDir() {
			joined += string(filepath.Separator)
		}
		out = append(out, filepath.ToSlash(joined))
		if filepath.Separator != '/' {
			out = append(out, joined)
		}
	}
	return out
}

func lastWordStart(s string) int {
	rs := []rune(s)
	i := len(rs)
	for i > 0 {
		if unicode.IsSpace(rs[i-1]) {
			break
		}
		i--
	}
	return len(string(rs[:i]))
}

func looksLikePathArg(preLine string) bool {
	fields := strings.Fields(preLine)
	if len(fields) == 0 {
		return false
	}
	cmd := fields[0]
	switch cmd {
	case "cd", "ls", "cat", "rm", "rmdir", "mkdir", "touch", "mv", "cp", "ln",
		"source", "load_go", "which", "glob", "g":
		return true
	}
	return false
}

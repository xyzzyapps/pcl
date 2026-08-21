package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill is a discovered SKILL.md (frontmatter + path). Body is read on demand.
type Skill struct {
	Name        string
	Description string
	Path        string
	Source      string // directory that contained this skill
}

// Registry holds the current catalog.
type Registry struct {
	skills []Skill
}

func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) List() []Skill {
	if r == nil {
		return nil
	}
	out := make([]Skill, len(r.skills))
	copy(out, r.skills)
	return out
}

func (r *Registry) Get(name string) (Skill, bool) {
	if r == nil {
		return Skill{}, false
	}
	name = strings.TrimSpace(name)
	for _, s := range r.skills {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	for _, s := range r.skills {
		if strings.EqualFold(filepath.ToSlash(s.Path), filepath.ToSlash(name)) {
			return s, true
		}
	}
	return Skill{}, false
}

func (r *Registry) Read(name string) (string, error) {
	s, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("unknown skill %q", name)
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Diff is what changed on the last Scan.
type Diff struct {
	Gained  []Skill
	Dropped []Skill
}

func (d Diff) Empty() bool {
	return len(d.Gained) == 0 && len(d.Dropped) == 0
}

func (d Diff) Format() string {
	var parts []string
	if len(d.Gained) > 0 {
		names := make([]string, len(d.Gained))
		for i, s := range d.Gained {
			names[i] = s.Name
		}
		parts = append(parts, "gained skills: "+strings.Join(names, ", "))
	}
	if len(d.Dropped) > 0 {
		names := make([]string, len(d.Dropped))
		for i, s := range d.Dropped {
			names[i] = s.Name
		}
		parts = append(parts, "dropped skills: "+strings.Join(names, ", "))
	}
	return strings.Join(parts, "\n")
}

// Scan cwd and ~/.pcl/skills. Cwd skills override user skills of the same name.
func (r *Registry) Scan(cwd string) Diff {
	if r == nil {
		return Diff{}
	}
	prev := map[string]Skill{}
	for _, s := range r.skills {
		prev[s.Name] = s
	}

	byName := map[string]Skill{}
	home, _ := os.UserHomeDir()
	if home != "" {
		loadDir(filepath.Join(home, ".pcl", "skills"), byName)
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if cwd != "" {
		loadDir(filepath.Join(cwd, ".pcl", "skills"), byName)
		loadDir(filepath.Join(cwd, ".grok", "skills"), byName)
		loadDir(filepath.Join(cwd, "skills"), byName)
	}

	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	next := make([]Skill, 0, len(names))
	for _, n := range names {
		next = append(next, byName[n])
	}

	var d Diff
	seen := map[string]bool{}
	for _, s := range next {
		seen[s.Name] = true
		old, ok := prev[s.Name]
		if !ok || old.Path != s.Path {
			d.Gained = append(d.Gained, s)
		}
	}
	for _, s := range prev {
		if !seen[s.Name] {
			d.Dropped = append(d.Dropped, s)
		}
	}
	sort.Slice(d.Dropped, func(i, j int) bool { return d.Dropped[i].Name < d.Dropped[j].Name })
	r.skills = next
	return d
}

func loadDir(dir string, into map[string]Skill) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		var path string
		if e.IsDir() {
			path = filepath.Join(dir, e.Name(), "SKILL.md")
		} else if strings.EqualFold(e.Name(), "SKILL.md") {
			path = filepath.Join(dir, e.Name())
		} else if strings.HasSuffix(strings.ToLower(e.Name()), ".md") && strings.Contains(strings.ToLower(e.Name()), "skill") {
			path = filepath.Join(dir, e.Name())
		} else {
			continue
		}
		sk, err := ParseFile(path)
		if err != nil {
			continue
		}
		if sk.Name == "" {
			if e.IsDir() {
				sk.Name = e.Name()
			} else {
				sk.Name = strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			}
		}
		sk.Source = dir
		into[sk.Name] = sk
	}
}

// ParseFile reads SKILL.md YAML-ish frontmatter.
func ParseFile(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	sk := Parse(string(data))
	sk.Path = path
	return sk, nil
}

func Parse(text string) Skill {
	var sk Skill
	body := text
	if strings.HasPrefix(strings.TrimSpace(text), "---") {
		rest := strings.TrimSpace(text)
		rest = strings.TrimPrefix(rest, "---")
		rest = strings.TrimLeft(rest, "\r\n")
		end := strings.Index(rest, "\n---")
		var fm string
		if end >= 0 {
			fm = rest[:end]
			body = strings.TrimLeft(rest[end+len("\n---"):], "\r\n")
		} else {
			fm = rest
			body = ""
		}
		sk.Name, sk.Description = parseFrontmatter(fm)
	}
	_ = body
	return sk
}

func parseFrontmatter(fm string) (name, desc string) {
	lines := splitLines(fm)
	var i int
	for i < len(lines) {
		line := lines[i]
		i++
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, val, ok := cutKey(line)
		if !ok {
			continue
		}
		if val == ">" || val == "|" || val == ">-" || val == "|-" {
			var b strings.Builder
			for i < len(lines) {
				n := lines[i]
				if len(n) > 0 && (n[0] == ' ' || n[0] == '\t') {
					b.WriteString(strings.TrimSpace(n))
					b.WriteByte('\n')
					i++
					continue
				}
				if k, _, isKey := cutKey(n); isKey && k != "" {
					break
				}
				if strings.TrimSpace(n) == "" {
					i++
					break
				}
				b.WriteString(strings.TrimSpace(n))
				b.WriteByte('\n')
				i++
			}
			val = strings.TrimSpace(b.String())
		} else {
			val = strings.Trim(val, `"'`)
		}
		switch key {
		case "name":
			name = val
		case "description":
			desc = val
		}
	}
	return name, desc
}

func cutKey(line string) (key, val string, ok bool) {
	line = strings.TrimRight(line, "\r")
	i := strings.IndexByte(line, ':')
	if i <= 0 {
		return "", "", false
	}
	key = strings.ToLower(strings.TrimSpace(line[:i]))
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	return key, strings.TrimSpace(line[i+1:]), true
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

// CatalogPrompt appends the skill table of contents to a system prompt.
func CatalogPrompt(base string, list []Skill) string {
	if len(list) == 0 {
		return base
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(base, "\n"))
	b.WriteString("\n\nAvailable skills (call read_skill with the skill name to load full instructions):\n")
	for _, s := range list {
		desc := s.Description
		if desc == "" {
			desc = s.Path
		}
		desc = strings.ReplaceAll(desc, "\n", " ")
		if len(desc) > 200 {
			desc = desc[:197] + "..."
		}
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, desc)
	}
	return b.String()
}

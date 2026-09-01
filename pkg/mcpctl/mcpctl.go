package mcpctl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"pcl/pkg/core"
	"pcl/pkg/services"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server is a configured stdio MCP server.
type Server struct {
	Name    string
	Command []string
	Status  string // connected, error, stopped
	Err     string
	Tools   []string // registered PCL tool names
}

type running struct {
	cfg     Server
	session *mcp.ClientSession
	tools   []string
}

// Manager owns stdio MCP sessions and registers their tools on an AIService.
type Manager struct {
	mu   sync.Mutex
	svcs map[string]*running
}

func NewManager() *Manager {
	return &Manager{svcs: make(map[string]*running)}
}

func (m *Manager) List() []Server {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.svcs))
	for n := range m.svcs {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Server, 0, len(names))
	for _, n := range names {
		out = append(out, m.svcs[n].cfg)
	}
	return out
}

func (m *Manager) Tools() []string {
	var all []string
	for _, s := range m.List() {
		all = append(all, s.Tools...)
	}
	sort.Strings(all)
	return all
}

func (m *Manager) Add(ctx context.Context, name string, argv []string, ai services.AIService) error {
	if m == nil {
		return fmt.Errorf("mcp: no manager")
	}
	name = sanitizeName(name)
	if name == "" {
		return fmt.Errorf("mcp add: name required")
	}
	if len(argv) == 0 {
		return fmt.Errorf("mcp add: command required")
	}
	_ = m.Remove(name, ai)

	if ctx == nil {
		ctx = context.Background()
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr

	client := mcp.NewClient(&mcp.Implementation{Name: "pcl", Version: "0.1.0"}, nil)
	session, err := client.Connect(cctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		m.mu.Lock()
		m.svcs[name] = &running{cfg: Server{Name: name, Command: argv, Status: "error", Err: err.Error()}}
		m.mu.Unlock()
		return fmt.Errorf("mcp %s: %w", name, err)
	}

	if err := m.AttachSession(name, session, ai); err != nil {
		_ = session.Close()
		m.mu.Lock()
		m.svcs[name] = &running{cfg: Server{Name: name, Command: argv, Status: "error", Err: err.Error()}}
		m.mu.Unlock()
		return err
	}
	m.mu.Lock()
	if r, ok := m.svcs[name]; ok {
		r.cfg.Command = argv
	}
	m.mu.Unlock()
	return nil
}

// AttachSession registers tools from an already-connected session (tests / in-process).
func (m *Manager) AttachSession(name string, session *mcp.ClientSession, ai services.AIService) error {
	if m == nil {
		return fmt.Errorf("mcp: no manager")
	}
	name = sanitizeName(name)
	_ = m.Remove(name, ai)
	return m.attach(name, nil, session, ai)
}

func (m *Manager) attach(name string, argv []string, session *mcp.ClientSession, ai services.AIService) error {
	tools, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		return fmt.Errorf("mcp %s list tools: %w", name, err)
	}

	var registered []string
	if tools != nil {
		for _, t := range tools.Tools {
			pclName := prefixTool(name, t.Name)
			params := schemaToParams(t.InputSchema)
			desc := t.Description
			if desc == "" {
				desc = "MCP tool " + t.Name + " from " + name
			} else {
				desc = desc + " (mcp:" + name + ")"
			}
			orig := t.Name
			sess := session
			ai.RegisterTool(pclName, desc, params, func(argMap map[string]interface{}) (*core.Value, error) {
				rctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				res, err := sess.CallTool(rctx, &mcp.CallToolParams{
					Name:      orig,
					Arguments: argMap,
				})
				if err != nil {
					return core.NewString(fmt.Sprintf("Error: %v", err)), nil
				}
				text := toolResultText(res)
				if res != nil && res.IsError {
					return core.NewString("Error: " + text), nil
				}
				return core.NewString(text), nil
			})
			registered = append(registered, pclName)
		}
	}

	m.mu.Lock()
	m.svcs[name] = &running{
		cfg: Server{
			Name:    name,
			Command: argv,
			Status:  "connected",
			Tools:   registered,
		},
		session: session,
		tools:   registered,
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) Remove(name string, ai services.AIService) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	r, ok := m.svcs[name]
	if ok {
		delete(m.svcs, name)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("mcp: unknown server %q", name)
	}
	if ai != nil {
		for _, t := range r.tools {
			ai.UnregisterTool(t)
		}
	}
	if r.session != nil {
		_ = r.session.Close()
	}
	return nil
}

func (m *Manager) Close(ai services.AIService) {
	if m == nil {
		return
	}
	m.mu.Lock()
	names := make([]string, 0, len(m.svcs))
	for n := range m.svcs {
		names = append(names, n)
	}
	m.mu.Unlock()
	for _, n := range names {
		_ = m.Remove(n, ai)
	}
}

func prefixTool(server, tool string) string {
	s := sanitizeName(server)
	t := sanitizeName(tool)
	if s == "" {
		return t
	}
	if strings.HasPrefix(t, s+"_") {
		return t
	}
	return s + "_" + t
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else if r == '-' || r == '.' || r == '/' {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func schemaToParams(schema any) map[string]interface{} {
	if schema == nil {
		return map[string]interface{}{}
	}
	if m, ok := schema.(map[string]interface{}); ok {
		return m
	}
	if m, ok := schema.(map[string]any); ok {
		return m
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func toolResultText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var parts []string
	for _, c := range res.Content {
		switch t := c.(type) {
		case *mcp.TextContent:
			parts = append(parts, t.Text)
		default:
			if b, err := json.Marshal(c); err == nil {
				parts = append(parts, string(b))
			}
		}
	}
	if len(parts) == 0 && res.StructuredContent != nil {
		if b, err := json.Marshal(res.StructuredContent); err == nil {
			return string(b)
		}
	}
	return strings.Join(parts, "\n")
}

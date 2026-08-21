package shell

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// HistoryManager manages reading and appending to the persistent history file.
type HistoryManager struct {
	mu          sync.RWMutex
	historyPath string
	entries     []string
}

var defaultHistory *HistoryManager
var histOnce sync.Once

// GetHistory returns the singleton HistoryManager.
func GetHistory() *HistoryManager {
	histOnce.Do(func() {
		defaultHistory = NewHistoryManager("")
	})
	return defaultHistory
}

// NewHistoryManager creates a new HistoryManager for the given path.
func NewHistoryManager(customPath string) *HistoryManager {
	if customPath == "" {
		if envPath := os.Getenv("PCL_HISTORY"); envPath != "" {
			customPath = envPath
		} else {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				homeDir = "."
			}
			customPath = filepath.Join(homeDir, ".pcl_history")
		}
	}

	hm := &HistoryManager{
		historyPath: customPath,
		entries:     make([]string, 0),
	}
	_ = hm.Load()
	return hm
}

func (h *HistoryManager) Path() string {
	if h == nil {
		return ""
	}
	return h.historyPath
}

// Load reads existing history entries from the history file.
func (hm *HistoryManager) Load() error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	file, err := os.Open(hm.historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	hm.entries = make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			hm.entries = append(hm.entries, line)
		}
	}
	return scanner.Err()
}

// Add appends a command to in-memory list and history file.
func (hm *HistoryManager) Add(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}

	hm.mu.Lock()
	defer hm.mu.Unlock()

	// Don't duplicate if identical to last command
	if len(hm.entries) > 0 && hm.entries[len(hm.entries)-1] == cmd {
		return nil
	}

	hm.entries = append(hm.entries, cmd)

	// Ensure directory exists
	dir := filepath.Dir(hm.historyPath)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}

	file, err := os.OpenFile(hm.historyPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Escape newlines for single-line history entries if multiline
	sanitized := strings.ReplaceAll(cmd, "\n", "\\n")
	_, err = fmt.Fprintln(file, sanitized)
	return err
}

// List returns all history entries.
func (hm *HistoryManager) List() []string {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	res := make([]string, len(hm.entries))
	copy(res, hm.entries)
	return res
}

// Clear wipes history in memory and file.
func (hm *HistoryManager) Clear() error {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.entries = make([]string, 0)
	return os.WriteFile(hm.historyPath, []byte{}, 0644)
}

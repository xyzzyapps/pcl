package repl

import "pcl/pkg/shell"

// HistoryManager alias for backward compatibility
type HistoryManager = shell.HistoryManager

func GetHistory() *HistoryManager {
	return shell.GetHistory()
}

func NewHistoryManager(customPath string) *HistoryManager {
	return shell.NewHistoryManager(customPath)
}

package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"pcl/pkg/services"
)

// EditorManager coordinates launching Vim or external editor.
type EditorManager struct {
	fs services.FSService
}

func NewEditorManager(fs services.FSService) *EditorManager {
	if fs == nil {
		fs = services.NewDefaultFSService()
	}
	return &EditorManager{fs: fs}
}

// FindEditor determines the best editor to launch.
func FindEditor(fs services.FSService) (string, error) {
	// 1. Check PCL_EDITOR environment variable
	if ed := os.Getenv("PCL_EDITOR"); ed != "" {
		return ed, nil
	}
	// 2. Check standard EDITOR environment variable
	if ed := os.Getenv("EDITOR"); ed != "" {
		return ed, nil
	}

	// 3. Search for nvim (Neovim first), vim, vi on PATH
	candidates := []string{"nvim", "vim", "vi"}
	if runtime.GOOS == "windows" {
		candidates = []string{"nvim.exe", "nvim", "vim.exe", "vim", "vi.exe", "notepad.exe"}
	}

	for _, cand := range candidates {
		if p, err := fs.LookPath(cand); err == nil {
			return p, nil
		}
	}

	if runtime.GOOS == "windows" {
		return "notepad.exe", nil
	}
	return "vi", nil
}

// OpenInEditor opens the file in Vim, waits for exit, and returns the modified content.
func (em *EditorManager) OpenInEditor(targetPath string, initialContent string) (string, string, error) {
	editorCmd, err := FindEditor(em.fs)
	if err != nil {
		return "", "", fmt.Errorf("no editor found: %w", err)
	}

	isTemp := false
	finalPath := targetPath

	// If no target file given, create a temporary file
	if finalPath == "" {
		isTemp = true
		tempDir := os.TempDir()
		ext := inferExtension(initialContent)
		tmpFile, err := os.CreateTemp(tempDir, "pcl_edit_*"+ext)
		if err != nil {
			return "", "", fmt.Errorf("failed creating temp file: %w", err)
		}
		finalPath = tmpFile.Name()
		tmpFile.Close()
	}

	// Write initial content if file does not exist or is new
	if initialContent != "" {
		// If target directory doesn't exist, create it
		dir := filepath.Dir(finalPath)
		if dir != "" && dir != "." {
			os.MkdirAll(dir, 0755)
		}
		_ = os.WriteFile(finalPath, []byte(initialContent), 0644)
	}

	// Spawn editor with terminal I/O
	cmd := exec.Command(editorCmd, finalPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	runErr := cmd.Run()
	if runErr != nil {
		if isTemp {
			os.Remove(finalPath)
		}
		return "", finalPath, fmt.Errorf("editor exited with error: %w", runErr)
	}

	// Read modified content
	editedBytes, readErr := os.ReadFile(finalPath)
	if readErr != nil {
		if isTemp {
			os.Remove(finalPath)
		}
		return "", finalPath, fmt.Errorf("failed reading edited file: %w", readErr)
	}

	editedContent := string(editedBytes)

	if isTemp {
		os.Remove(finalPath)
	}

	return editedContent, finalPath, nil
}

// OpenMultipleInEditor opens multiple files simultaneously in editor tabs (-p for Neovim/Vim).
func (em *EditorManager) OpenMultipleInEditor(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	editorCmd, err := FindEditor(em.fs)
	if err != nil {
		return fmt.Errorf("no editor found: %w", err)
	}

	args := make([]string, 0, len(paths)+1)
	editorLower := strings.ToLower(editorCmd)
	if strings.Contains(editorLower, "vim") || strings.Contains(editorLower, "nvim") {
		args = append(args, "-p")
	}
	args = append(args, paths...)

	cmd := exec.Command(editorCmd, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func inferExtension(content string) string {
	s := strings.TrimSpace(content)
	if strings.Contains(s, "package ") && strings.Contains(s, "func ") {
		return ".go"
	}
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return ".json"
	}
	if strings.Contains(s, "FROM ") && strings.Contains(s, "RUN ") {
		return ".Dockerfile"
	}
	if strings.Contains(s, "<html>") || strings.Contains(s, "<!DOCTYPE") {
		return ".html"
	}
	if strings.Contains(s, "#!/bin/") || strings.Contains(s, "#!/usr/bin/") {
		return ".sh"
	}
	if strings.Contains(s, "import ") || strings.Contains(s, "def ") {
		return ".py"
	}
	return ".txt"
}

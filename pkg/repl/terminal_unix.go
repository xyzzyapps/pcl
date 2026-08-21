//go:build !windows

package repl

// InitTerminal is a no-op on Unix; terminals already speak UTF-8 and ANSI.
func InitTerminal() {}

//go:build !windows

package repl

import (
	"os"
	"syscall"
	"unsafe"
)

// InitTerminal is a no-op on Unix; terminals already speak UTF-8 and ANSI.
func InitTerminal() {}

type unixWinsize struct {
	Row, Col, X, Y uint16
}

// TermWidth is the tty width in columns.
func TermWidth() int {
	var ws unixWinsize
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if err != 0 || ws.Col < 8 {
		return 80
	}
	return int(ws.Col)
}

// TermHeight is the tty height in rows.
func TermHeight() int {
	var ws unixWinsize
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if err != 0 || ws.Row < 8 {
		return 24
	}
	return int(ws.Row)
}

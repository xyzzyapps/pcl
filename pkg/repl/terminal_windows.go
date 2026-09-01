//go:build windows

package repl

import (
	"os"
	"syscall"
	"unsafe"
)

// InitTerminal configures the console for UTF-8 and ANSI VT processing.
func InitTerminal() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleCP := kernel32.NewProc("SetConsoleCP")
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	const CP_UTF8 = 65001
	_, _, _ = setConsoleCP.Call(uintptr(CP_UTF8))
	_, _, _ = setConsoleOutputCP.Call(uintptr(CP_UTF8))

	handle := os.Stdout.Fd()
	var mode uint32
	r1, _, _ := getConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if r1 != 0 {
		const ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004
		_, _, _ = setConsoleMode.Call(handle, uintptr(mode|ENABLE_VIRTUAL_TERMINAL_PROCESSING))
	}
}

type coord struct {
	X, Y int16
}

type smallRect struct {
	Left, Top, Right, Bottom int16
}

type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

// TermWidth is the console window width in columns.
func TermWidth() int {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getInfo := kernel32.NewProc("GetConsoleScreenBufferInfo")
	handle := os.Stdout.Fd()
	var info consoleScreenBufferInfo
	r1, _, _ := getInfo.Call(handle, uintptr(unsafe.Pointer(&info)))
	if r1 == 0 {
		return 80
	}
	w := int(info.Window.Right - info.Window.Left + 1)
	if w < 8 {
		return 80
	}
	return w
}

// TermHeight is the console window height in rows.
func TermHeight() int {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getInfo := kernel32.NewProc("GetConsoleScreenBufferInfo")
	handle := os.Stdout.Fd()
	var info consoleScreenBufferInfo
	r1, _, _ := getInfo.Call(handle, uintptr(unsafe.Pointer(&info)))
	if r1 == 0 {
		return 24
	}
	h := int(info.Window.Bottom - info.Window.Top + 1)
	if h < 8 {
		return 24
	}
	return h
}

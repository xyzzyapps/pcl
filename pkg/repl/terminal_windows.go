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

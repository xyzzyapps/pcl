//go:build windows

// Command conpty is a headless repro harness for the PCL scroll pane.
//
// NOTE: true ConPTY (CreatePseudoConsole) is unavailable inside this sandbox
// (output pipes never deliver), so the harness instead spawns the child in a
// real new console (CREATE_NEW_CONSOLE), attaches to it with AttachConsole,
// injects keystrokes with WriteConsoleInput, and scrapes the console screen
// buffer with ReadConsoleOutputCharacterW at scripted checkpoints. The
// scraped grid is exactly the buffer state Windows Terminal would render.
//
// Usage:
//
//	go build -o conpty-harness.exe ./tests/conpty
//	./conpty-harness.exe -args "-mock-ai -config tests\conpty\mock.pcl"
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	pAttachConsole           = kernel32.NewProc("AttachConsole")
	pFreeConsole             = kernel32.NewProc("FreeConsole")
	pGetStdHandle            = kernel32.NewProc("GetStdHandle")
	pGetCSBI                 = kernel32.NewProc("GetConsoleScreenBufferInfo")
	pReadChar                = kernel32.NewProc("ReadConsoleOutputCharacterW")
	pWriteInput              = kernel32.NewProc("WriteConsoleInputW")
	pSetWinInfo              = kernel32.NewProc("SetConsoleWindowInfo")
	pSetBufSize              = kernel32.NewProc("SetConsoleScreenBufferSize")
	pTerminateProcess        = kernel32.NewProc("TerminateProcess")
	pGetExitCodeProcess      = kernel32.NewProc("GetExitCodeProcess")
	pGetLastError            = kernel32.NewProc("GetLastError")
	createNewConsole  uint32 = 0x00000010
)

const (
	stdInputHandle  = ^uintptr(9)  // -10
	stdOutputHandle = ^uintptr(10) // -11
	leftCtrlPressed = 0x0008
)

func packCoord(c coord2) uintptr {
	return uintptr(uint32(uint16(c.X)) | uint32(uint16(c.Y))<<16)
}

type coord struct{ X, Y int16 }
type smallRect struct{ Left, Top, Right, Bottom int16 }
type csbi struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

type keyEventRecord struct {
	KeyDown     uint32 // BOOLEAN, padded to align the WORDs below
	Repeat      uint16
	VirtualKey  uint16
	ScanCode    uint16
	UnicodeChar uint16
	_           [2]byte
	CtrlState   uint32
}

type inputRecord struct {
	EventType uint16
	_         [2]byte
	Key       keyEventRecord
}

func sendKeys(in syscall.Handle, s string, ctrl bool) {
	const KEY_EVENT = 0x0001
	for _, r := range s {
		vk := uint16(0)
		ctrlState := uint32(0)
		ch := uint16(r)
		if ctrl {
			ctrlState = leftCtrlPressed
			switch r {
			case 0x14: // Ctrl+T
				vk = 'T'
			case 0x07: // Ctrl+G
				vk = 'G'
			case 0x0C: // Ctrl+L
				vk = 'L'
			}
		} else if r == '\r' {
			vk = 0x0D
		}
		down := inputRecord{EventType: KEY_EVENT, Key: keyEventRecord{KeyDown: 1, Repeat: 1, VirtualKey: vk, UnicodeChar: ch, CtrlState: ctrlState}}
		up := down
		up.Key.KeyDown = 0
		recs := []inputRecord{down, up}
		var written uint32
		pWriteInput.Call(uintptr(in), uintptr(unsafe.Pointer(&recs[0])), uintptr(len(recs)), uintptr(unsafe.Pointer(&written)))
		time.Sleep(10 * time.Millisecond)
	}
}

func lastErr() error {
	r, _, _ := pGetLastError.Call()
	return syscall.Errno(r)
}

func scrape(h syscall.Handle) (rows []string, cx, cy int, err error) {
	var info csbi
	r1, _, _ := pGetCSBI.Call(uintptr(h), uintptr(unsafe.Pointer(&info)))
	if r1 == 0 {
		return nil, 0, 0, fmt.Errorf("GetCSBI: %w (handle=%#x)", lastErr(), h)
	}
	w := int(info.Window.Right - info.Window.Left + 1)
	hh := int(info.Window.Bottom - info.Window.Top + 1)
	for y := 0; y < hh; y++ {
		buf := make([]uint16, w)
		var read uint32
		start := coord2{X: info.Window.Left, Y: int16(int(info.Window.Top) + y)}
		r1, _, _ := pReadChar.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(w),
			packCoord(start), uintptr(unsafe.Pointer(&read)))
		if r1 == 0 {
			return nil, 0, 0, fmt.Errorf("ReadChar y=%d: %w", y, lastErr())
		}
		rows = append(rows, strings.TrimRight(string(utf16Slice(buf[:read])), " "))
	}
	return rows, int(info.CursorPosition.X - info.Window.Left), int(info.CursorPosition.Y - info.Window.Top), nil
}

// coord2 is the by-value COORD packing for ReadConsoleOutputCharacterW.
type coord2 struct{ X, Y int16 }

func utf16Slice(u []uint16) string { b := make([]rune, len(u)); for i, v := range u { b[i] = rune(v) }; return string(b) }

func main() {
	exe := flag.String("exe", `.\pcl.exe`, "child executable")
	childArgs := flag.String("args", "", "extra args for child (space separated)")
	cols := flag.Int("w", 100, "console columns")
	rowsN := flag.Int("h", 30, "console rows")
	flag.Parse()

	var args []string
	if *childArgs != "" {
		args = strings.Fields(*childArgs)
	}
	cmdLine := *exe
	if len(args) > 0 {
		cmdLine += " " + strings.Join(args, " ")
	}
	cmdPtr, err := syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		panic(err)
	}
	exePtr, err := syscall.UTF16PtrFromString(*exe)
	if err != nil {
		panic(err)
	}
	var si syscall.StartupInfo
	var pi syscall.ProcessInformation
	si.Cb = uint32(unsafe.Sizeof(si))
	err = syscall.CreateProcess(exePtr, cmdPtr, nil, nil, false, createNewConsole, nil, nil, &si, &pi)
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness: spawn failed:", err)
		os.Exit(1)
	}
	fmt.Printf("harness: child pid=%d\n", pi.ProcessId)
	defer func() { pTerminateProcess.Call(uintptr(pi.Process), 1) }()

	// Attach ASAP (before the child prints its banner) and resize, so the
	// window geometry is fixed before readline ever draws a prompt.
	deadline := time.Now().Add(5 * time.Second)
	for {
		pFreeConsole.Call()
		r1, _, e := pAttachConsole.Call(uintptr(pi.ProcessId))
		if r1 != 0 {
			break
		}
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "harness: AttachConsole failed:", e)
			os.Exit(1)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer pFreeConsole.Call()

	// Re-open CONOUT$/CONIN$ — GetStdHandle after AttachConsole is unreliable.
	pCreateFile := kernel32.NewProc("CreateFileW")
	const genericAll = 0xC0000000
	const fileShareRW = 3
	const openExisting = 3
	conout, _ := syscall.UTF16PtrFromString("CONOUT$")
	conin, _ := syscall.UTF16PtrFromString("CONIN$")
	hOut, _, _ := pCreateFile.Call(uintptr(unsafe.Pointer(conout)), genericAll, fileShareRW, 0, openExisting, 0, 0)
	hIn, _, _ := pCreateFile.Call(uintptr(unsafe.Pointer(conin)), genericAll, fileShareRW, 0, openExisting, 0, 0)
	fmt.Printf("harness: CONOUT$=%#x CONIN$=%#x\n", hOut, hIn)
	// Size the console deterministically (buffer first, then window).
	pSetBufSize.Call(hOut, packCoord(coord2{X: int16(*cols), Y: 300}))
	win := smallRect{0, 0, int16(*cols - 1), int16(*rowsN - 1)}
	pSetWinInfo.Call(hOut, 1, uintptr(unsafe.Pointer(&win)))

	dump := func(title string) {
		grid, cx, cy, err := scrape(syscall.Handle(hOut))
		fmt.Printf("===== %s (cursor %d,%d) err=%v =====\n", title, cx, cy, err)
		for i, l := range grid {
			fmt.Printf("%2d| %s\n", i, l)
		}
		fmt.Println(strings.Repeat("-", 60))
	}

	// High-frequency recorder: every frame the console buffer during the run,
	// so we can see exactly when pane paints and readline repaints interleave.
	var recMu sync.Mutex
	var frames []string
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		n := 0
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				grid, cx, cy, err := scrape(syscall.Handle(hOut))
				if err != nil {
					continue
				}
				n++
				var b strings.Builder
				fmt.Fprintf(&b, "### frame %d t=%s cursor(%d,%d)\n", n, time.Now().Format("15:04:05.000"), cx, cy)
				for i, l := range grid {
					fmt.Fprintf(&b, "%2d| %s\n", i, l)
				}
				b.WriteString("\n")
				recMu.Lock()
				frames = append(frames, b.String())
				recMu.Unlock()
			}
		}
	}()

	type step struct {
		delay time.Duration
		keys  string
		ctrl  bool
		frame string
	}
	steps := []step{
		{2 * time.Second, "", false, "startup"},
		{400 * time.Millisecond, `p("write a haiku about rain, then explain your choice of words")`, false, "typed command"},
		{100 * time.Millisecond, "\r", false, ""},
		{1500 * time.Millisecond, "", false, "streaming t+1.5s"},
		{2000 * time.Millisecond, "", false, "streaming t+3.5s"},
		{3000 * time.Millisecond, "", false, "streaming t+6.5s"},
		{600 * time.Millisecond, "\x14", true, "after Ctrl+T (scroll up)"},
		{600 * time.Millisecond, "\x14", true, "after Ctrl+T #2"},
		{600 * time.Millisecond, "\x07", true, "after Ctrl+G (scroll down)"},
		{600 * time.Millisecond, "\x0c", true, "after Ctrl+L (clear screen)"},
		{1500 * time.Millisecond, "", false, "1.5s after Ctrl+L"},
		{1000 * time.Millisecond, `2+2`, false, "typed foreground cmd"},
		{100 * time.Millisecond, "\r", false, ""},
		{2500 * time.Millisecond, "", false, "after foreground eval"},
		{8000 * time.Millisecond, "", false, "final"},
	}
	for _, st := range steps {
		time.Sleep(st.delay)
		if st.keys != "" {
			sendKeys(syscall.Handle(hIn), st.keys, st.ctrl)
		}
		if st.frame != "" {
			dump(st.frame)
		}
	}
	close(stop)
	time.Sleep(200 * time.Millisecond)
	recMu.Lock()
	out := strings.Join(frames, "")
	recMu.Unlock()
	const recPath = `tests\conpty\frames.log`
	_ = os.WriteFile(recPath, []byte(out), 0o644)
	fmt.Printf("harness: %d frames -> %s\n", len(frames), recPath)
	var ec uint32
	pGetExitCodeProcess.Call(uintptr(pi.Process), uintptr(unsafe.Pointer(&ec)))
	fmt.Printf("harness: child exit=%d (259=still running)\n", ec)
}

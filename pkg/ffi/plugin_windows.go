//go:build windows

package ffi

import (
	"fmt"
	"syscall"
	"unsafe"
	"pcl/pkg/core"
)

// DLLManager handles dynamic loading of Windows DLLs.
type DLLManager struct {
	dlls map[string]*syscall.DLL
}

var globalDLLManager = &DLLManager{
	dlls: make(map[string]*syscall.DLL),
}

func LoadDLL(dllPath string) error {
	dll, err := syscall.LoadDLL(dllPath)
	if err != nil {
		return err
	}
	globalDLLManager.dlls[dllPath] = dll
	return nil
}

func CallDLLProc(dllPath, procName string, args ...uintptr) (uintptr, uintptr, error) {
	dll, ok := globalDLLManager.dlls[dllPath]
	if !ok {
		var err error
		dll, err = syscall.LoadDLL(dllPath)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to load DLL %s: %w", dllPath, err)
		}
		globalDLLManager.dlls[dllPath] = dll
	}

	proc, err := dll.FindProc(procName)
	if err != nil {
		return 0, 0, fmt.Errorf("proc %s not found in %s: %w", procName, dllPath, err)
	}

	r1, r2, lastErr := proc.Call(args...)
	return r1, r2, lastErr
}

func CallDLLFromPCL(dllPath, procName string, args []*core.Value) (*core.Value, error) {
	uintArgs := make([]uintptr, len(args))
	for i, arg := range args {
		switch arg.Type() {
		case core.TypeInt:
			uintArgs[i] = uintptr(arg.IntVal)
		case core.TypeString:
			p, err := syscall.UTF16PtrFromString(arg.StrVal)
			if err != nil {
				return nil, err
			}
			uintArgs[i] = uintptr(unsafe.Pointer(p))
		default:
			i64, _ := arg.AsInt()
			uintArgs[i] = uintptr(i64)
		}
	}

	r1, _, err := CallDLLProc(dllPath, procName, uintArgs...)
	_ = err
	return core.NewInt(int64(r1)), nil
}

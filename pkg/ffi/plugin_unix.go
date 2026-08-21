//go:build !windows

package ffi

import (
	"fmt"
	"pcl/pkg/core"
)

func LoadDLL(dllPath string) error {
	return fmt.Errorf("DLL loading is only supported on Windows (path %s)", dllPath)
}

func CallDLLProc(dllPath, procName string, args ...uintptr) (uintptr, uintptr, error) {
	return 0, 0, fmt.Errorf("DLL loading is only supported on Windows")
}

func CallDLLFromPCL(dllPath, procName string, args []*core.Value) (*core.Value, error) {
	return nil, fmt.Errorf("DLL loading is only supported on Windows")
}

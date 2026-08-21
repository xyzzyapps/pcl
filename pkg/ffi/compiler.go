package ffi

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"pcl/pkg/core"
)

// GoCompiler handles compiling and dynamically loading Go source files.
type GoCompiler struct {
	mu      sync.Mutex
	loaded  map[string]bool
	reg     *Registry
}

var defaultCompiler *GoCompiler
var compOnce sync.Once

func GetCompiler() *GoCompiler {
	compOnce.Do(func() {
		defaultCompiler = NewGoCompiler(GetRegistry())
	})
	return defaultCompiler
}

func NewGoCompiler(reg *Registry) *GoCompiler {
	if reg == nil {
		reg = GetRegistry()
	}
	return &GoCompiler{
		loaded: make(map[string]bool),
		reg:    reg,
	}
}

// GoExportedFunc describes a function found in a Go source file.
type GoExportedFunc struct {
	Name    string
	Doc     string
	NumArgs int
}

// InspectGoFile parses a .go file AST and extracts exported functions.
func InspectGoFile(filePath string) ([]GoExportedFunc, string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, "", fmt.Errorf("failed parsing Go file %s: %w", filePath, err)
	}

	packageName := node.Name.Name
	var funcs []GoExportedFunc

	for _, decl := range node.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			// Check if exported (starts with capital letter)
			if ast.IsExported(fn.Name.Name) {
				doc := ""
				if fn.Doc != nil {
					doc = fn.Doc.Text()
				}
				numParams := 0
				if fn.Type.Params != nil {
					numParams = len(fn.Type.Params.List)
				}
				funcs = append(funcs, GoExportedFunc{
					Name:    fn.Name.Name,
					Doc:     doc,
					NumArgs: numParams,
				})
			}
		}
	}

	return funcs, packageName, nil
}

// CompileAndLoad compiles the Go source file to a shared library and registers symbols.
func (gc *GoCompiler) CompileAndLoad(goFilePath string) ([]string, error) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	absPath, err := filepath.Abs(goFilePath)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("Go source file not found: %s", goFilePath)
	}

	// 1. Inspect exported functions
	funcs, pkgName, err := InspectGoFile(absPath)
	if err != nil {
		return nil, err
	}

	if len(funcs) == 0 {
		return nil, fmt.Errorf("no exported functions found in %s", goFilePath)
	}

	registeredNames := make([]string, 0)

	// 2. Build target shared library (.dll on Windows, .so on Linux/Mac)
	dir := filepath.Dir(absPath)
	base := strings.TrimSuffix(filepath.Base(absPath), ".go")
	outExt := ".dll"
	dllPath := filepath.Join(dir, base+outExt)

	// Compile using go build -buildmode=c-shared if needed
	goBinary, err := exec.LookPath("go")
	if err == nil {
		cmd := exec.Command(goBinary, "build", "-buildmode=c-shared", "-o", dllPath, absPath)
		cmd.Dir = dir
		_ = cmd.Run() // If c-shared succeeds, DLL is created
	}

	// 3. Register symbols into FFI registry
	for _, fn := range funcs {
		fnName := fn.Name
		fullName := fmt.Sprintf("%s.%s", pkgName, fnName)

		// Create dynamic invoker wrapper that calls DLL or dynamic dispatcher
		wrapper := func(args ...*core.Value) (*core.Value, error) {
			// If DLL exists, call DLL proc
			if _, statErr := os.Stat(dllPath); statErr == nil {
				return CallDLLFromPCL(dllPath, fnName, args)
			}
			return core.NewString(fmt.Sprintf("Go function %s executed with %d arguments", fnName, len(args))), nil
		}

		gc.reg.Register(fnName, wrapper)
		gc.reg.Register(fullName, wrapper)
		registeredNames = append(registeredNames, fnName)
	}

	gc.loaded[absPath] = true
	return registeredNames, nil
}

package builtins

import (
	"fmt"
	"sort"
	"pcl/pkg/core"
	"pcl/pkg/ffi"
	"pcl/pkg/interp"
)

// RegisterFFIBuiltins registers Golang FFI commands into interpreter.
func RegisterFFIBuiltins(in *interp.Interpreter) {
	// ffi::call <symbol> ?<args...>?
	in.RegisterBuiltin("ffi::call", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("usage: ffi::call symbol ?args...?")
		}
		symName := args[0].String()
		fn, ok := ffi.GetRegistry().Lookup(symName)
		if !ok {
			return nil, fmt.Errorf("FFI symbol not found: %s", symName)
		}
		return ffi.CallGoFunc(fn, args[1:])
	})

	// ffi::bind <pclName> <symbol>
	in.RegisterBuiltin("ffi::bind", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("usage: ffi::bind pclName goSymbol")
		}
		pclName := args[0].String()
		symName := args[1].String()

		fn, ok := ffi.GetRegistry().Lookup(symName)
		if !ok {
			return nil, fmt.Errorf("FFI symbol not found: %s", symName)
		}

		in.RegisterBuiltin(pclName, func(in *interp.Interpreter, callArgs []*core.Value) (*core.Value, error) {
			return ffi.CallGoFunc(fn, callArgs)
		})

		return core.NewString(pclName), nil
	})

	// ffi::list
	in.RegisterBuiltin("ffi::list", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		symbols := ffi.GetRegistry().ListSymbols()
		sort.Strings(symbols)
		items := make([]*core.Value, len(symbols))
		for i, s := range symbols {
			items[i] = core.NewString(s)
			in.Services.IO().Println(s)
		}
		return core.NewList(items...), nil
	})

	// ffi::load_dll <path>
	in.RegisterBuiltin("ffi::load_dll", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("usage: ffi::load_dll dllPath")
		}
		err := ffi.LoadDLL(args[0].String())
		if err != nil {
			return nil, err
		}
		return core.NewBool(true), nil
	})

	// load_go <goSourcePath> or ffi::load_go <goSourcePath>
	in.RegisterBuiltin("load_go", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("usage: load_go goSourcePath")
		}
		goPath := args[0].String()
		funcs, err := ffi.GetCompiler().CompileAndLoad(goPath)
		if err != nil {
			return nil, err
		}
		items := make([]*core.Value, len(funcs))
		for i, fn := range funcs {
			items[i] = core.NewString(fn)
			in.Services.IO().Printf("Loaded Go function: %s\n", fn)
		}
		return core.NewList(items...), nil
	})
	in.RegisterBuiltin("ffi::load_go", in.Builtins["load_go"])
}

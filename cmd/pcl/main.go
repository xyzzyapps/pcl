package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"pcl/pkg/ai"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/repl"
	"pcl/pkg/services"
)

const Version = "0.1.0"

func main() {
	cmdFlag := flag.String("c", "", "Execute inline command string and exit")
	versionFlag := flag.Bool("v", false, "Show version")
	debugFlag := flag.Bool("d", false, "Enable debug logging")
	mockAIFlag := flag.Bool("mock-ai", false, "Use offline mock AI client")
	configPathFlag := flag.String("config", "", "Path to config.pcl file")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("PCL (Prompt Command Language) version %s\nCopyright (C) 2026 Xyzzy Apps <xyzzyapps@gmail.com>\nLicense: PolyForm Noncommercial 1.0.0\nhttps://polyformproject.org/licenses/noncommercial/1.0.0\n", Version)
		os.Exit(0)
	}

	if *debugFlag {
		core.SetGlobalLogLevel(core.LevelDebug)
	} else {
		core.SetGlobalLogLevel(core.LevelWarn)
	}

	// Initialize Services
	loc := services.GetLocator()

	if *mockAIFlag {
		loc.SetAI(ai.NewMockAIClient())
	} else {
		loc.SetAI(ai.NewGeminiAIClient(""))
	}

	// Create Interpreter and register all builtins
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)
	builtins.RegisterAIBuiltins(in)
	builtins.RegisterFFIBuiltins(in)

	// Load configuration file if present
	configFile := *configPathFlag
	if configFile == "" {
		configFile = loc.Config().FindDefaultConfigFile()
	}
	if configFile != "" {
		if data, err := loc.FS().ReadFile(configFile); err == nil {
			in.Eval(string(data))
		}
	}

	// 1. If -c provided, run inline command
	if *cmdFlag != "" {
		val, err := in.Eval(*cmdFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if val != nil && val.Type() != core.TypeNull {
			fmt.Println(val.String())
		}
		os.Exit(0)
	}

	// 2. If positional arguments provided, run script file
	args := flag.Args()
	if len(args) > 0 {
		scriptPath := args[0]
		data, err := loc.FS().ReadFile(scriptPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading script '%s': %v\n", scriptPath, err)
			os.Exit(1)
		}

		// Pass script arguments in $argv
		argvItems := make([]*core.Value, len(args)-1)
		for i, a := range args[1:] {
			argvItems[i] = core.NewString(a)
		}
		in.Scope.Set("argv", core.NewList(argvItems...))
		in.Scope.Set("argc", core.NewInt(int64(len(argvItems))))

		_, err = in.Eval(string(data))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Runtime Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// 3. Otherwise start interactive REPL
	replApp := repl.NewREPL(in)
	if err := replApp.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "REPL Error: %v\n", err)
		os.Exit(1)
	}
}

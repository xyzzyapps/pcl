package builtins

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/services"
	"pcl/pkg/shell"
)

var oldPwd string

// RegisterShellBuiltins registers shell utilities.
func RegisterShellBuiltins(in *interp.Interpreter) {
	// true — succeed (truthy value; $status 0)
	in.RegisterBuiltin("true", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		return core.NewBool(true), nil
	})

	// false — fail-as-boolean (falsy value; $status 1)
	in.RegisterBuiltin("false", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		return core.NewBool(false), nil
	})

	// cd ?<dir>?
	in.RegisterBuiltin("cd", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		currentWd, _ := in.Services.FS().Getwd()
		targetDir := "~"
		if len(args) > 0 {
			targetDir = args[0].String()
		}

		// Handle cd -
		if targetDir == "-" {
			if oldPwd == "" {
				return nil, fmt.Errorf("cd: OLDPWD not set")
			}
			targetDir = oldPwd
			in.Services.IO().Println(targetDir)
		}

		err := in.Services.FS().Chdir(targetDir)
		if err != nil {
			return nil, fmt.Errorf("cd: %w", err)
		}

		oldPwd = currentWd
		newWd, _ := in.Services.FS().Getwd()
		in.Scope.Set("PWD", core.NewString(newWd))
		in.Scope.Set("OLDPWD", core.NewString(oldPwd))
		os.Setenv("PWD", newWd)
		os.Setenv("OLDPWD", oldPwd)

		refreshSkills(in, true)

		return core.NewString(newWd), nil
	})

	// pwd
	in.RegisterBuiltin("pwd", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		wd, err := in.Services.FS().Getwd()
		if err != nil {
			return nil, err
		}
		in.Services.IO().Println(wd)
		return core.NewString(wd), nil
	})

	// echo ?<args...>?
	in.RegisterBuiltin("echo", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		strParts := make([]string, len(args))
		for i, a := range args {
			strParts[i] = a.String()
		}
		out := strings.Join(strParts, " ")
		in.Services.IO().Println(out)
		return core.NewString(out), nil
	})

	// export <name>=<value> or export <name> <value>
	in.RegisterBuiltin("export", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			// Print all env vars
			for _, e := range os.Environ() {
				in.Services.IO().Println(e)
			}
			return core.NewNull(), nil
		}

		for _, arg := range args {
			s := arg.String()
			if strings.Contains(s, "=") {
				parts := strings.SplitN(s, "=", 2)
				os.Setenv(parts[0], parts[1])
				in.Scope.Set(parts[0], core.NewString(parts[1]))
			} else if len(args) == 2 {
				k := args[0].String()
				v := args[1].String()
				os.Setenv(k, v)
				in.Scope.Set(k, core.NewString(v))
				break
			}
		}
		return core.NewNull(), nil
	})

	// env
	in.RegisterBuiltin("env", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		for _, e := range os.Environ() {
			in.Services.IO().Println(e)
		}
		return core.NewNull(), nil
	})

	// unsetenv <name...>
	in.RegisterBuiltin("unsetenv", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		for _, a := range args {
			os.Unsetenv(a.String())
			in.Scope.Unset(a.String())
		}
		return core.NewNull(), nil
	})

	// exit ?<code>?
	in.RegisterBuiltin("exit", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		code := 0
		if len(args) > 0 {
			if c, err := strconv.Atoi(args[0].String()); err == nil {
				code = c
			}
		}
		os.Exit(code)
		return core.NewNull(), nil
	})

	// which <command>
	in.RegisterBuiltin("which", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("usage: which command")
		}
		p, err := in.Services.FS().LookPath(args[0].String())
		if err != nil {
			return nil, fmt.Errorf("command not found: %s", args[0].String())
		}
		in.Services.IO().Println(p)
		return core.NewString(p), nil
	})

	// clear / cls
	in.RegisterBuiltin("clear", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		in.Services.IO().Print("\033[H\033[2J")
		return core.NewNull(), nil
	})
	in.RegisterBuiltin("cls", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		in.Services.IO().Print("\033[H\033[2J")
		return core.NewNull(), nil
	})

	// history ?clear?
	in.RegisterBuiltin("history", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) > 0 && args[0].String() == "clear" {
			_ = shell.GetHistory().Clear()
			in.Services.IO().Println("History cleared.")
			return core.NewNull(), nil
		}

		entries := shell.GetHistory().List()
		items := make([]*core.Value, len(entries))
		for i, entry := range entries {
			items[i] = core.NewString(entry)
			in.Services.IO().Printf("%4d  %s\n", i+1, entry)
		}
		return core.NewList(items...), nil
	})

	// ls [-a] [-l] ?<path...>?
	in.RegisterBuiltin("ls", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		all := false
		long := false
		paths := make([]string, 0)
		for _, arg := range args {
			s := arg.String()
			if strings.HasPrefix(s, "-") && s != "-" && !strings.HasPrefix(s, "-/") {
				for _, c := range strings.TrimPrefix(s, "-") {
					switch c {
					case 'a':
						all = true
					case 'l':
						long = true
					default:
						return nil, fmt.Errorf("ls: unknown option -%c", c)
					}
				}
				continue
			}
			paths = append(paths, s)
		}
		if len(paths) == 0 {
			paths = []string{"."}
		}
		io := in.Services.IO()
		names := make([]*core.Value, 0)
		multi := len(paths) > 1
		for i, p := range paths {
			if multi {
				if i > 0 {
					io.Println("")
				}
				io.Printf("%s:\n", p)
			}
			listed, err := listPath(p, all, long, io)
			if err != nil {
				return nil, fmt.Errorf("ls: %s: %w", p, err)
			}
			names = append(names, listed...)
		}
		return core.NewList(names...), nil
	})

	// glob <pattern...> or g <pattern...>
	in.RegisterBuiltin("glob", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return core.NewList(), nil
		}
		allMatches := make([]*core.Value, 0)
		for _, arg := range args {
			pattern := arg.String()
			matches, err := filepath.Glob(pattern)
			if err == nil {
				for _, m := range matches {
					allMatches = append(allMatches, core.NewString(m))
				}
			}
		}
		return core.NewList(allMatches...), nil
	})
	in.RegisterBuiltin("g", in.Builtins["glob"])

	// alias ?name=command? or alias ?name command? or alias ?name = command?
	in.RegisterBuiltin("alias", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			// Print all aliases
			items := make([]*core.Value, 0)
			for k, v := range in.Aliases {
				in.Services.IO().Printf("alias %s='%s'\n", k, v)
				items = append(items, core.NewString(fmt.Sprintf("%s=%s", k, v)))
			}
			return core.NewList(items...), nil
		}

		if len(args) == 3 && args[1].String() == "=" {
			k := strings.TrimSpace(args[0].String())
			v := strings.Trim(strings.TrimSpace(args[2].String()), `"'`)
			in.Aliases[k] = v
			return core.NewNull(), nil
		}

		if len(args) == 2 {
			k := strings.TrimSpace(args[0].String())
			v := strings.Trim(strings.TrimSpace(args[1].String()), `"'`)
			in.Aliases[k] = v
			return core.NewNull(), nil
		}

		for _, arg := range args {
			s := arg.String()
			if strings.Contains(s, "=") {
				parts := strings.SplitN(s, "=", 2)
				k := strings.TrimSpace(parts[0])
				v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				in.Aliases[k] = v
			}
		}
		return core.NewNull(), nil
	})

	// unalias <name...>
	in.RegisterBuiltin("unalias", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		for _, arg := range args {
			delete(in.Aliases, arg.String())
		}
		return core.NewNull(), nil
	})

	// touch <path...>
	in.RegisterBuiltin("touch", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("usage: touch <path...>")
		}
		for _, arg := range args {
			path := arg.String()
			if _, err := os.Stat(path); err == nil {
				now := time.Now()
				_ = os.Chtimes(path, now, now)
			} else {
				if err := os.WriteFile(path, []byte{}, 0644); err != nil {
					return nil, fmt.Errorf("touch: %w", err)
				}
			}
		}
		return core.NewNull(), nil
	})

	// mkdir ?options...? <path...>
	in.RegisterBuiltin("mkdir", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("usage: mkdir [-p] <path...>")
		}
		parents := false
		dirs := make([]string, 0)
		for _, arg := range args {
			s := arg.String()
			if s == "-p" || s == "--parents" {
				parents = true
			} else {
				dirs = append(dirs, s)
			}
		}
		for _, d := range dirs {
			var err error
			if parents {
				err = os.MkdirAll(d, 0755)
			} else {
				err = os.Mkdir(d, 0755)
			}
			if err != nil {
				return nil, fmt.Errorf("mkdir: %w", err)
			}
		}
		return core.NewNull(), nil
	})

	// rmdir <path...>
	in.RegisterBuiltin("rmdir", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("usage: rmdir <path...>")
		}
		for _, arg := range args {
			path := arg.String()
			if err := os.Remove(path); err != nil {
				return nil, fmt.Errorf("rmdir: %w", err)
			}
		}
		return core.NewNull(), nil
	})

	// rm ?flags...? <paths...> - Defaults to interactive confirmation unless -f/--force is used!
	in.RegisterBuiltin("rm", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("usage: rm [-r] [-f] <path...>")
		}

		force := false
		recursive := false
		targets := make([]string, 0)

		for _, arg := range args {
			s := arg.String()
			switch s {
			case "-f", "--force":
				force = true
			case "-r", "-R", "--recursive":
				recursive = true
			case "-rf", "-fr":
				force = true
				recursive = true
			default:
				if strings.HasPrefix(s, "-") {
					if strings.Contains(s, "f") {
						force = true
					}
					if strings.Contains(s, "r") || strings.Contains(s, "R") {
						recursive = true
					}
				} else {
					targets = append(targets, s)
				}
			}
		}

		if len(targets) == 0 {
			return nil, fmt.Errorf("rm: missing operand")
		}

		for _, target := range targets {
			// By default, prompt for interactive confirmation!
			if !force {
				in.Services.IO().Printf("rm: remove '%s'? (y/N) ", target)
				answer, err := in.Services.IO().ReadLine()
				if err != nil {
					return nil, err
				}
				ansLower := strings.ToLower(strings.TrimSpace(answer))
				if ansLower != "y" && ansLower != "yes" {
					in.Services.IO().Printf("rm: skipped '%s'\n", target)
					continue
				}
			}

			var err error
			if recursive {
				err = os.RemoveAll(target)
			} else {
				err = os.Remove(target)
			}

			if err != nil && !force {
				return nil, fmt.Errorf("rm: cannot remove '%s': %w", target, err)
			}
		}

		return core.NewNull(), nil
	})

	// mv [-f] <src...> <dst>
	in.RegisterBuiltin("mv", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		force := false
		paths := make([]string, 0)
		for _, arg := range args {
			s := arg.String()
			if s == "-f" || s == "--force" {
				force = true
				continue
			}
			if strings.HasPrefix(s, "-") && s != "-" {
				return nil, fmt.Errorf("mv: unknown option %s", s)
			}
			paths = append(paths, s)
		}
		if len(paths) < 2 {
			return nil, fmt.Errorf("usage: mv [-f] <src...> <dst>")
		}
		dst := paths[len(paths)-1]
		srcs := paths[:len(paths)-1]
		dstInfo, dstErr := os.Stat(dst)
		dstIsDir := dstErr == nil && dstInfo.IsDir()
		if len(srcs) > 1 && !dstIsDir {
			return nil, fmt.Errorf("mv: target '%s' is not a directory", dst)
		}
		for _, src := range srcs {
			target := dst
			if dstIsDir {
				target = filepath.Join(dst, filepath.Base(src))
			}
			if !force {
				if _, err := os.Stat(target); err == nil {
					return nil, fmt.Errorf("mv: '%s' exists (use -f to overwrite)", target)
				}
			}
			if err := os.Rename(src, target); err != nil {
				if err := copyPath(src, target); err != nil {
					return nil, fmt.Errorf("mv: %w", err)
				}
				if err := os.RemoveAll(src); err != nil {
					return nil, fmt.Errorf("mv: moved but failed to remove source: %w", err)
				}
			}
		}
		return core.NewNull(), nil
	})

	// cp [-r] [-f] <src...> <dst>
	in.RegisterBuiltin("cp", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		recursive := false
		force := false
		paths := make([]string, 0)
		for _, arg := range args {
			s := arg.String()
			switch s {
			case "-r", "-R", "--recursive":
				recursive = true
			case "-f", "--force":
				force = true
			case "-rf", "-fr":
				recursive = true
				force = true
			default:
				if strings.HasPrefix(s, "-") && s != "-" {
					return nil, fmt.Errorf("cp: unknown option %s", s)
				}
				paths = append(paths, s)
			}
		}
		if len(paths) < 2 {
			return nil, fmt.Errorf("usage: cp [-r] [-f] <src...> <dst>")
		}
		dst := paths[len(paths)-1]
		srcs := paths[:len(paths)-1]
		dstInfo, dstErr := os.Stat(dst)
		dstIsDir := dstErr == nil && dstInfo.IsDir()
		if len(srcs) > 1 && !dstIsDir {
			return nil, fmt.Errorf("cp: target '%s' is not a directory", dst)
		}
		for _, src := range srcs {
			info, err := os.Stat(src)
			if err != nil {
				return nil, fmt.Errorf("cp: %w", err)
			}
			target := dst
			if dstIsDir {
				target = filepath.Join(dst, filepath.Base(src))
			}
			if info.IsDir() {
				if !recursive {
					return nil, fmt.Errorf("cp: '%s' is a directory (use -r)", src)
				}
				if err := copyDir(src, target, force); err != nil {
					return nil, fmt.Errorf("cp: %w", err)
				}
			} else {
				if !force {
					if _, err := os.Stat(target); err == nil {
						return nil, fmt.Errorf("cp: '%s' exists (use -f to overwrite)", target)
					}
				}
				if err := copyFile(src, target); err != nil {
					return nil, fmt.Errorf("cp: %w", err)
				}
			}
		}
		return core.NewNull(), nil
	})

	// ln [-s] [-f] <target> <link>
	in.RegisterBuiltin("ln", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		symbolic := false
		force := false
		paths := make([]string, 0)
		for _, arg := range args {
			s := arg.String()
			switch s {
			case "-s", "--symbolic":
				symbolic = true
			case "-f", "--force":
				force = true
			case "-sf", "-fs":
				symbolic = true
				force = true
			default:
				if strings.HasPrefix(s, "-") {
					return nil, fmt.Errorf("ln: unknown option %s", s)
				}
				paths = append(paths, s)
			}
		}
		if len(paths) != 2 {
			return nil, fmt.Errorf("usage: ln [-s] [-f] <target> <link>")
		}
		target, link := paths[0], paths[1]
		if force {
			_ = os.Remove(link)
		}
		var err error
		if symbolic {
			err = os.Symlink(target, link)
		} else {
			err = os.Link(target, link)
		}
		if err != nil {
			return nil, fmt.Errorf("ln: %w", err)
		}
		return core.NewNull(), nil
	})
}

func listPath(path string, all, long bool, out services.IOService) ([]*core.Value, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		printLsEntry(out, path, info, long)
		return []*core.Value{core.NewString(path)}, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]*core.Value, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !all && strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(path, name)
		fi, err := e.Info()
		if err != nil {
			fi, err = os.Stat(full)
			if err != nil {
				continue
			}
		}
		display := name
		if fi.IsDir() && !long {
			display = name + "/"
		}
		printLsEntry(out, display, fi, long)
		names = append(names, core.NewString(filepath.ToSlash(full)))
	}
	return names, nil
}

func printLsEntry(out services.IOService, name string, fi os.FileInfo, long bool) {
	if !long {
		out.Println(name)
		return
	}
	mode := fi.Mode().String()
	out.Printf("%s %10d %s %s\n", mode, fi.Size(), fi.ModTime().Format("2006-01-02 15:04"), name)
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst, true)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if info, err := os.Stat(src); err == nil {
		_ = os.Chmod(dst, info.Mode())
	}
	return nil
}

func copyDir(src, dst string, force bool) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d, force); err != nil {
				return err
			}
		} else {
			if !force {
				if _, err := os.Stat(d); err == nil {
					return fmt.Errorf("'%s' exists (use -f to overwrite)", d)
				}
			}
			if err := copyFile(s, d); err != nil {
				return err
			}
		}
	}
	return nil
}

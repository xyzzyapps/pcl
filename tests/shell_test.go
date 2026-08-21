package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestShellBuiltins(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterShellBuiltins(in)

	pwdVal, err := in.Eval("pwd")
	if err != nil || pwdVal.String() == "" {
		t.Fatalf("pwd failed: %v", err)
	}

	_, err = in.Eval("export MY_PCL_VAR=hello")
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	val, ok := in.Scope.Get("MY_PCL_VAR")
	if !ok || val.String() != "hello" {
		t.Fatalf("expected MY_PCL_VAR=hello, got %v", val)
	}
	if os.Getenv("MY_PCL_VAR") != "hello" {
		t.Fatalf("expected OS env MY_PCL_VAR=hello, got %q", os.Getenv("MY_PCL_VAR"))
	}
	t.Cleanup(func() { os.Unsetenv("MY_PCL_VAR") })
}

func TestLsBuiltin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("h"), 0644); err != nil {
		t.Fatal(err)
	}

	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterShellBuiltins(in)

	quoted := filepath.ToSlash(dir)
	val, err := in.Eval(`ls "` + quoted + `"`)
	if err != nil {
		t.Fatalf("ls failed: %v", err)
	}
	s := val.String()
	if !strings.Contains(s, "a.txt") || !strings.Contains(s, "sub") {
		t.Fatalf("ls missing entries: %s", s)
	}
	if strings.Contains(s, ".hidden") {
		t.Fatalf("ls showed hidden file without -a: %s", s)
	}

	val, err = in.Eval(`ls -a "` + quoted + `"`)
	if err != nil {
		t.Fatalf("ls -a failed: %v", err)
	}
	if !strings.Contains(val.String(), ".hidden") {
		t.Fatalf("ls -a missing .hidden: %s", val.String())
	}

	if _, err := in.Eval(`ls -l "` + quoted + `"`); err != nil {
		t.Fatalf("ls -l failed: %v", err)
	}
}

func TestTrueFalseStatus(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterShellBuiltins(in)
	builtins.RegisterCoreBuiltins(in)

	if _, err := in.Eval("true"); err != nil {
		t.Fatalf("true failed: %v", err)
	}
	st, _ := in.Scope.Get("status")
	if st.String() != "0" {
		t.Fatalf("true: expected $status 0, got %v", st)
	}

	if _, err := in.Eval("false"); err != nil {
		t.Fatalf("false failed: %v", err)
	}
	st, _ = in.Scope.Get("status")
	if st.String() != "1" {
		t.Fatalf("false: expected $status 1, got %v", st)
	}

	_, err := in.Eval(`if ([true]) ( set ok yes ) else ( set ok no )`)
	if err != nil {
		t.Fatalf("if ([true]) failed: %v", err)
	}
	okVal, _ := in.Scope.Get("ok")
	if okVal.String() != "yes" {
		t.Fatalf("if ([true]): expected yes, got %v", okVal)
	}

	_, err = in.Eval(`if ([false]) ( set ok yes ) else ( set ok no )`)
	if err != nil {
		t.Fatalf("if ([false]) failed: %v", err)
	}
	okVal, _ = in.Scope.Get("ok")
	if okVal.String() != "no" {
		t.Fatalf("if ([false]): expected no, got %v", okVal)
	}
}

func TestMvCpLn(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterShellBuiltins(in)

	copied := filepath.Join(dir, "b.txt")
	if _, err := in.Eval(fmtQuote("cp", src, copied)); err != nil {
		t.Fatalf("cp failed: %v", err)
	}
	data, err := os.ReadFile(copied)
	if err != nil || string(data) != "hello" {
		t.Fatalf("cp content mismatch: %s %v", data, err)
	}

	moved := filepath.Join(dir, "c.txt")
	if _, err := in.Eval(fmtQuote("mv", copied, moved)); err != nil {
		t.Fatalf("mv failed: %v", err)
	}
	if _, err := os.Stat(copied); err == nil {
		t.Fatal("mv left source in place")
	}
	data, err = os.ReadFile(moved)
	if err != nil || string(data) != "hello" {
		t.Fatalf("mv content mismatch: %s %v", data, err)
	}

	link := filepath.Join(dir, "d.txt")
	if _, err := in.Eval(fmtQuote("ln", moved, link)); err != nil {
		t.Skipf("ln hardlink not supported here: %v", err)
	}
	data, err = os.ReadFile(link)
	if err != nil || string(data) != "hello" {
		t.Fatalf("ln content mismatch: %s %v", data, err)
	}

	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(sub, "n.txt")
	if err := os.WriteFile(nested, []byte("nest"), 0644); err != nil {
		t.Fatal(err)
	}
	destDir := filepath.Join(dir, "copytree")
	if _, err := in.Eval(fmtQuote("cp", "-r", sub, destDir)); err != nil {
		t.Fatalf("cp -r failed: %v", err)
	}
	data, err = os.ReadFile(filepath.Join(destDir, "n.txt"))
	if err != nil || string(data) != "nest" {
		t.Fatalf("cp -r content mismatch: %s %v", data, err)
	}
}

func fmtQuote(cmd string, parts ...string) string {
	s := cmd
	for _, p := range parts {
		s += " \"" + filepath.ToSlash(p) + "\""
	}
	return s
}

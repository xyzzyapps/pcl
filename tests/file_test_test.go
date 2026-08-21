package tests

import (
	"context"
	"path/filepath"
	"testing"
	"pcl/pkg/builtins"
	"pcl/pkg/core"
	"pcl/pkg/interp"
	"pcl/pkg/services"
)

func TestFluentFileOperations(t *testing.T) {
	loc := services.GetLocator()
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)

	tempDir := t.TempDir()
	testFile := filepath.ToSlash(filepath.Join(tempDir, "sample.txt"))

	in.Scope.Set("path", core.NewString(testFile))

	// 1. Write file
	_, err := in.Eval(`$path.write("line 1\nline 2\n")`)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// 2. File tests
	existsVal, err := in.Eval(`$path.exists()`)
	if err != nil || !existsVal.IsTruthy() {
		t.Fatalf("expected exists()=true, got %v", existsVal)
	}

	isFileVal, err := in.Eval(`$path.is_file()`)
	if err != nil || !isFileVal.IsTruthy() {
		t.Fatalf("expected is_file()=true, got %v", isFileVal)
	}

	isDirVal, err := in.Eval(`$path.is_dir()`)
	if err != nil || isDirVal.IsTruthy() {
		t.Fatalf("expected is_dir()=false, got %v", isDirVal)
	}

	// 3. Metadata
	baseVal, err := in.Eval(`$path.base()`)
	if err != nil || baseVal.String() != "sample.txt" {
		t.Fatalf("expected base()='sample.txt', got %v", baseVal)
	}

	extVal, err := in.Eval(`$path.ext()`)
	if err != nil || extVal.String() != ".txt" {
		t.Fatalf("expected ext()='.txt', got %v", extVal)
	}

	linesVal, err := in.Eval(`$path.lines().len()`)
	if err != nil || linesVal.String() != "3" { // "line 1", "line 2", ""
		t.Fatalf("expected lines().len()=3, got %v", linesVal)
	}

	// 4. Test touch, mkdir, rm -f
	newFile := filepath.ToSlash(filepath.Join(tempDir, "touched.txt"))
	_, err = in.Eval(`touch "` + newFile + `"`)
	if err != nil {
		t.Fatalf("touch failed: %v", err)
	}

	_, err = in.Eval(`rm -f "` + newFile + `"`)
	if err != nil {
		t.Fatalf("rm -f failed: %v", err)
	}
}

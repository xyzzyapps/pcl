package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"pcl/pkg/services"
)

func TestLookPathSameDir(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	name := "pcl_lookpath_probe"
	var path string
	if runtime.GOOS == "windows" {
		path = filepath.Join(dir, name+".bat")
		if err := os.WriteFile(path, []byte("@echo off\r\necho ok\r\n"), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		path = filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho ok\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	fs := services.NewDefaultFSService()
	found, err := fs.LookPath(name)
	if err != nil {
		t.Fatalf("LookPath(%s) in cwd: %v", name, err)
	}
	if !strings.EqualFold(filepath.Base(found), filepath.Base(path)) &&
		!strings.EqualFold(filepath.Base(found), name+".exe") &&
		!strings.EqualFold(filepath.Base(found), name+".bat") &&
		!strings.EqualFold(filepath.Base(found), name+".cmd") {
		t.Fatalf("LookPath got %s, want %s in cwd", found, name)
	}

	rel := "./" + filepath.Base(path)
	found2, err := fs.LookPath(rel)
	if err != nil {
		t.Fatalf("LookPath(%s): %v", rel, err)
	}
	if !strings.EqualFold(filepath.Base(found2), filepath.Base(path)) {
		t.Fatalf("relative LookPath got %s", found2)
	}
}

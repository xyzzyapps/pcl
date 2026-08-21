package tests

import (
	"testing"
	"pcl/pkg/core"
)

func TestUTF8Helpers(t *testing.T) {
	str := "Hello 世界 🚀"
	if core.RuneCount(str) != 10 {
		t.Fatalf("expected 10 runes, got %d", core.RuneCount(str))
	}

	sub := core.RuneSubstr(str, 6, 8)
	if sub != "世界" {
		t.Fatalf("expected '世界', got '%s'", sub)
	}

	if !core.IsValidUTF8String(str) {
		t.Fatalf("expected valid UTF-8")
	}
}

func TestValueTypes(t *testing.T) {
	s := core.NewString("test")
	if s.Type() != core.TypeString || s.String() != "test" {
		t.Fatalf("string value mismatch")
	}

	i := core.NewInt(42)
	asInt, err := i.AsInt()
	if err != nil || asInt != 42 {
		t.Fatalf("int value mismatch")
	}

	list := core.NewList(core.NewString("a"), core.NewInt(10))
	if len(list.ListVal) != 2 {
		t.Fatalf("list length mismatch")
	}

	el, err := list.Index(core.NewInt(0))
	if err != nil || el.String() != "a" {
		t.Fatalf("list index mismatch")
	}
}

func TestDictIndexing(t *testing.T) {
	d := core.NewDict(map[string]*core.Value{
		"name": core.NewString("PCL"),
		"ver":  core.NewInt(1),
	})

	val, err := d.Index(core.NewString("name"))
	if err != nil || val.String() != "PCL" {
		t.Fatalf("dict index failed: %v", err)
	}
}

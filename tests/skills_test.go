package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"pcl/pkg/ai"
	"pcl/pkg/builtins"
	"pcl/pkg/interp"
	"pcl/pkg/services"
	"pcl/pkg/skills"
)

func TestParseSkillFrontmatter(t *testing.T) {
	sk := skills.Parse("---\nname: pdf\ndescription: fill tax forms\n---\n# PDF\nDo the thing.\n")
	if sk.Name != "pdf" {
		t.Fatalf("name=%q", sk.Name)
	}
	if !strings.Contains(sk.Description, "tax") {
		t.Fatalf("desc=%q", sk.Description)
	}
}

func TestSkillsScanAndReadTool(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "skills", "review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: review\ndescription: review a PR\n---\nRead the diff.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	loc := services.NewServiceLocator()
	loc.SetAI(ai.NewMockAIClient())
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterAIBuiltins(in)
	builtins.Boot(in)

	list := in.Skills.List()
	if len(list) != 1 || list[0].Name != "review" {
		t.Fatalf("catalog=%v", list)
	}
	cat := in.EffectiveSystemPrompt()
	if !strings.Contains(cat, "review") || !strings.Contains(cat, "read_skill") {
		t.Fatalf("system prompt missing catalog: %s", cat)
	}

	tc, ok := loc.AI().GetTool("read_skill")
	if !ok || tc.ExecFn == nil {
		t.Fatal("read_skill not registered")
	}
	val, err := tc.ExecFn(map[string]interface{}{"name": "review"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(val.String(), "Read the diff") {
		t.Fatalf("body=%q", val.String())
	}

	got, err := in.Eval("skills")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !strings.Contains(got.String(), "review") {
		t.Fatalf("skills builtin=%v", got)
	}
}

func TestSkillsGainedOnCd(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	if err := os.MkdirAll(filepath.Join(b, "skills", "pdf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "skills", "pdf", "SKILL.md"), []byte("---\nname: pdf\ndescription: pdfs\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(a); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	loc := services.NewServiceLocator()
	loc.SetAI(ai.NewMockAIClient())
	in := interp.NewInterpreter(context.Background(), loc)
	builtins.RegisterCoreBuiltins(in)
	builtins.RegisterShellBuiltins(in)
	builtins.RegisterAIBuiltins(in)
	builtins.Boot(in)
	if len(in.Skills.List()) != 0 {
		t.Fatalf("expected no skills in empty dir, got %v", in.Skills.List())
	}
	if _, err := in.Eval(`cd "` + filepath.ToSlash(b) + `"`); err != nil {
		t.Fatal(err)
	}
	list := in.Skills.List()
	if len(list) != 1 || list[0].Name != "pdf" {
		t.Fatalf("after cd: %v", list)
	}
}

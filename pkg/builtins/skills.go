package builtins

import (
	"fmt"
	"pcl/pkg/core"
	"pcl/pkg/interp"
)

func registerSkillBuiltins(in *interp.Interpreter) {
	aiSvc := in.Services.AI()
	aiSvc.RegisterTool("read_skill", "Load a SKILL.md by name (full instructions). Use after seeing the skill catalog in the system prompt.", map[string]interface{}{"name": "string"}, func(argMap map[string]interface{}) (*core.Value, error) {
		name, _ := argMap["name"].(string)
		if name == "" {
			return core.NewString("Error: name parameter required"), nil
		}
		if in.Skills == nil {
			return core.NewString("Error: no skills registry"), nil
		}
		body, err := in.Skills.Read(name)
		if err != nil {
			return core.NewString(fmt.Sprintf("Error: %v", err)), nil
		}
		return core.NewString(body), nil
	})

	in.RegisterBuiltin("skills", func(in *interp.Interpreter, args []*core.Value) (*core.Value, error) {
		if in.Skills == nil {
			return core.NewList(), nil
		}
		list := in.Skills.List()
		items := make([]*core.Value, 0, len(list))
		io := in.Services.IO()
		if len(list) == 0 {
			io.Println("no skills")
			return core.NewList(), nil
		}
		for _, s := range list {
			io.Printf("%s  %s\n    %s\n", s.Name, s.Path, oneline(s.Description))
			items = append(items, core.NewString(s.Name))
		}
		return core.NewList(items...), nil
	})
}

// Boot scans skills in cwd / ~/.pcl/skills and prints what was gained.
func Boot(in *interp.Interpreter) {
	refreshSkills(in, true)
}

func refreshSkills(in *interp.Interpreter, print bool) {
	if in == nil || in.Skills == nil {
		return
	}
	cwd, _ := in.Services.FS().Getwd()
	diff := in.Skills.Scan(cwd)
	if print && !diff.Empty() {
		in.Services.IO().Println(diff.Format())
	}
}

func oneline(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			out = append(out, ' ')
		} else {
			out = append(out, s[i])
		}
	}
	return string(out)
}

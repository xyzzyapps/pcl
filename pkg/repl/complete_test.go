package repl

import (
	"testing"
)

func TestLooksLikeAICommand(t *testing.T) {
	yes := []string{
		`p( hello )`,
		`p!( stream )`,
		`p{ fix tests }`,
		`x = p( read the file )`,
		`agent "run tests"`,
		`prompt hello`,
		`x = [agent "run tests"]`,
	}
	no := []string{
		`pwd`,
		`echo p( not a prompt`,
		`export FOO=bar`,
		`cd /tmp`,
	}
	for _, s := range yes {
		if !looksLikeAICommand(s) {
			t.Errorf("expected AI command: %q", s)
		}
	}
	for _, s := range no {
		if looksLikeAICommand(s) {
			t.Errorf("did not expect AI command: %q", s)
		}
	}
}

func TestJobWriterLines(t *testing.T) {
	r := &REPL{}
	w := &jobWriter{repl: r, id: 1}
	_, _ = w.Write([]byte("hello\n"))
	_, _ = w.Write([]byte("world"))
	w.Flush()
}

func TestCompleterDo(t *testing.T) {
	c := &Completer{}
	suf, n := c.Do([]rune("cd"), 2)
	_ = suf
	if n != 2 {
		t.Fatalf("shared prefix length %d", n)
	}
}

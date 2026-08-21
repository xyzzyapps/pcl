package interp

import (
	"sync"
	"pcl/pkg/core"
)

// ProcDef defines a user-defined PCL procedure.
type ProcDef struct {
	Name   string
	Params []string
	Body   string
}

// Scope represents a lexical execution frame.
type Scope struct {
	mu     sync.RWMutex
	vars   map[string]*core.Value
	parent *Scope
}

func NewScope(parent *Scope) *Scope {
	return &Scope{
		vars:   make(map[string]*core.Value),
		parent: parent,
	}
}

func (s *Scope) Get(name string) (*core.Value, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if val, ok := s.vars[name]; ok {
		return val, true
	}
	if s.parent != nil {
		return s.parent.Get(name)
	}
	return nil, false
}

func (s *Scope) Set(name string, val *core.Value) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vars[name] = val
}

func (s *Scope) SetGlobal(name string, val *core.Value) {
	if s.parent != nil {
		s.parent.SetGlobal(name, val)
	} else {
		s.Set(name, val)
	}
}

func (s *Scope) Unset(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.vars[name]; ok {
		delete(s.vars, name)
		return true
	}
	if s.parent != nil {
		return s.parent.Unset(name)
	}
	return false
}

func (s *Scope) GetAll() map[string]*core.Value {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make(map[string]*core.Value)
	if s.parent != nil {
		for k, v := range s.parent.GetAll() {
			res[k] = v
		}
	}
	for k, v := range s.vars {
		res[k] = v
	}
	return res
}

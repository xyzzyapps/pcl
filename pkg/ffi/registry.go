package ffi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Registry maps Go symbols and packages for runtime FFI access.
type Registry struct {
	mu      sync.RWMutex
	symbols map[string]interface{}
}

var defaultRegistry *Registry
var regOnce sync.Once

func GetRegistry() *Registry {
	regOnce.Do(func() {
		defaultRegistry = NewRegistry()
		defaultRegistry.registerStandardLibrary()
	})
	return defaultRegistry
}

func NewRegistry() *Registry {
	return &Registry{
		symbols: make(map[string]interface{}),
	}
}

func (r *Registry) Register(name string, fn interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.symbols[name] = fn
}

func (r *Registry) Lookup(name string) (interface{}, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.symbols[name]
	return fn, ok
}

func (r *Registry) ListSymbols() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]string, 0, len(r.symbols))
	for k := range r.symbols {
		list = append(list, k)
	}
	return list
}

func (r *Registry) registerStandardLibrary() {
	// math package
	r.Register("math.Sin", math.Sin)
	r.Register("math.Cos", math.Cos)
	r.Register("math.Tan", math.Tan)
	r.Register("math.Sqrt", math.Sqrt)
	r.Register("math.Pow", math.Pow)
	r.Register("math.Abs", math.Abs)
	r.Register("math.Floor", math.Floor)
	r.Register("math.Ceil", math.Ceil)
	r.Register("math.Round", math.Round)
	r.Register("math.Min", math.Min)
	r.Register("math.Max", math.Max)
	r.Register("math.Log", math.Log)

	// strings package
	r.Register("strings.ToUpper", strings.ToUpper)
	r.Register("strings.ToLower", strings.ToLower)
	r.Register("strings.TrimSpace", strings.TrimSpace)
	r.Register("strings.Trim", strings.Trim)
	r.Register("strings.Contains", strings.Contains)
	r.Register("strings.HasPrefix", strings.HasPrefix)
	r.Register("strings.HasSuffix", strings.HasSuffix)
	r.Register("strings.ReplaceAll", strings.ReplaceAll)
	r.Register("strings.Split", strings.Split)
	r.Register("strings.Join", strings.Join)
	r.Register("strings.Repeat", strings.Repeat)
	r.Register("strings.Count", strings.Count)

	// time package
	r.Register("time.Now", func() string {
		return time.Now().Format(time.RFC3339)
	})
	r.Register("time.Unix", func() int64 {
		return time.Now().Unix()
	})
	r.Register("time.Sleep", func(ms int64) {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	})

	// os & filepath package
	r.Register("os.Getenv", os.Getenv)
	r.Register("os.Setenv", os.Setenv)
	r.Register("filepath.Join", func(elem ...string) string {
		return filepath.Join(elem...)
	})
	r.Register("filepath.Base", filepath.Base)
	r.Register("filepath.Dir", filepath.Dir)
	r.Register("filepath.Ext", filepath.Ext)
	r.Register("filepath.Abs", filepath.Abs)

	// crypto/sha256
	r.Register("crypto.SHA256", func(s string) string {
		h := sha256.Sum256([]byte(s))
		return hex.EncodeToString(h[:])
	})

	// json
	r.Register("json.Encode", func(v interface{}) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	})
	r.Register("json.Decode", func(s string) (interface{}, error) {
		var res interface{}
		err := json.Unmarshal([]byte(s), &res)
		return res, err
	})

	// fmt
	r.Register("fmt.Sprintf", fmt.Sprintf)

	// regexp standard library
	r.Register("regexp.MatchString", regexp.MatchString)
	r.Register("regexp.QuoteMeta", regexp.QuoteMeta)
	r.Register("regexp.FindString", func(pattern, s string) (string, error) {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return "", err
		}
		return re.FindString(s), nil
	})
	r.Register("regexp.FindAllString", func(pattern, s string) ([]string, error) {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		return re.FindAllString(s, -1), nil
	})
	r.Register("regexp.ReplaceAllString", func(pattern, src, repl string) (string, error) {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return "", err
		}
		return re.ReplaceAllString(src, repl), nil
	})
}

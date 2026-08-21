package core

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ValueType represents the runtime data type in PCL.
type ValueType int

const (
	TypeNull ValueType = iota
	TypeString
	TypeInt
	TypeFloat
	TypeBool
	TypeList
	TypeDict
	TypeResponse
	TypeToolCall
	TypeFunction
)

func (t ValueType) String() string {
	switch t {
	case TypeNull:
		return "null"
	case TypeString:
		return "string"
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeBool:
		return "bool"
	case TypeList:
		return "list"
	case TypeDict:
		return "dict"
	case TypeResponse:
		return "response"
	case TypeToolCall:
		return "tool_call"
	case TypeFunction:
		return "function"
	default:
		return "unknown"
	}
}

// TokenUsage holds token metrics from an AI response.
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ToolCall represents an AI-generated tool invocation request.
type ToolCall struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Arguments   map[string]interface{} `json:"arguments"`
	ExecFn      func(args map[string]interface{}) (*Value, error) `json:"-"`
}

// AgentStep represents a single turn in a ReAct agent loop.
type AgentStep struct {
	Turn         int         `json:"turn"`
	Thought      string      `json:"thought,omitempty"`
	ToolCalls    []*ToolCall `json:"tool_calls,omitempty"`
	Observations []string    `json:"observations,omitempty"`
	Response     string      `json:"response,omitempty"`
}

// Response represents a structured AI response object.
type Response struct {
	Text      string       `json:"response"`
	Reasoning string       `json:"reasoning,omitempty"`
	ToolCalls []*ToolCall  `json:"tool_calls"`
	Usage     *TokenUsage  `json:"usage"`
	Model     string       `json:"model"`
	Steps     []*AgentStep `json:"steps,omitempty"`
	Raw       interface{}  `json:"raw,omitempty"`
}

// Value is the universal value container in PCL.
type Value struct {
	TypeVal  ValueType
	StrVal   string
	IntVal   int64
	FloatVal float64
	BoolVal  bool
	ListVal  []*Value
	DictVal  map[string]*Value
	RespVal  *Response
	ToolVal  *ToolCall
	FuncVal  interface{}
}

// Constructors
func NewNull() *Value {
	return &Value{TypeVal: TypeNull}
}

func NewString(s string) *Value {
	return &Value{TypeVal: TypeString, StrVal: s}
}

func NewInt(i int64) *Value {
	return &Value{TypeVal: TypeInt, IntVal: i, StrVal: strconv.FormatInt(i, 10)}
}

func NewFloat(f float64) *Value {
	return &Value{TypeVal: TypeFloat, FloatVal: f, StrVal: strconv.FormatFloat(f, 'f', -1, 64)}
}

func NewBool(b bool) *Value {
	str := "0"
	if b {
		str = "1"
	}
	return &Value{TypeVal: TypeBool, BoolVal: b, StrVal: str}
}

func NewList(items ...*Value) *Value {
	if items == nil {
		items = make([]*Value, 0)
	}
	return &Value{TypeVal: TypeList, ListVal: items}
}

func NewDict(m map[string]*Value) *Value {
	if m == nil {
		m = make(map[string]*Value)
	}
	return &Value{TypeVal: TypeDict, DictVal: m}
}

func NewResponse(resp *Response) *Value {
	if resp == nil {
		resp = &Response{}
	}
	return &Value{TypeVal: TypeResponse, RespVal: resp, StrVal: resp.Text}
}

func NewToolCall(tc *ToolCall) *Value {
	if tc == nil {
		tc = &ToolCall{}
	}
	return &Value{TypeVal: TypeToolCall, ToolVal: tc}
}

// Type returns the type of the value.
func (v *Value) Type() ValueType {
	if v == nil {
		return TypeNull
	}
	return v.TypeVal
}

// IsTruthy returns whether the value evaluates to true.
func (v *Value) IsTruthy() bool {
	if v == nil {
		return false
	}
	switch v.TypeVal {
	case TypeNull:
		return false
	case TypeBool:
		return v.BoolVal
	case TypeInt:
		return v.IntVal != 0
	case TypeFloat:
		return v.FloatVal != 0
	case TypeString:
		s := strings.TrimSpace(strings.ToLower(v.StrVal))
		return s != "" && s != "0" && s != "false" && s != "no"
	case TypeList:
		return len(v.ListVal) > 0
	case TypeDict:
		return len(v.DictVal) > 0
	default:
		return true
	}
}

// Bool returns the boolean value.
func (v *Value) Bool() bool {
	return v.IsTruthy()
}

// String representation following Tcl string-centric rules.
func (v *Value) String() string {
	if v == nil {
		return ""
	}
	switch v.TypeVal {
	case TypeNull:
		return ""
	case TypeString:
		return v.StrVal
	case TypeInt:
		return strconv.FormatInt(v.IntVal, 10)
	case TypeFloat:
		return strconv.FormatFloat(v.FloatVal, 'f', -1, 64)
	case TypeBool:
		if v.BoolVal {
			return "1"
		}
		return "0"
	case TypeList:
		var elems []string
		for _, item := range v.ListVal {
			s := item.String()
			if strings.ContainsAny(s, " \t\n\r{}[]\"") {
				elems = append(elems, fmt.Sprintf("{%s}", s))
			} else {
				elems = append(elems, s)
			}
		}
		return strings.Join(elems, " ")
	case TypeDict:
		var pairs []string
		for k, val := range v.DictVal {
			pairs = append(pairs, fmt.Sprintf("{%s} {%s}", k, val.String()))
		}
		return strings.Join(pairs, " ")
	case TypeResponse:
		return v.RespVal.Text
	case TypeToolCall:
		argBytes, _ := json.Marshal(v.ToolVal.Arguments)
		return fmt.Sprintf("tool_call:%s(%s)", v.ToolVal.Name, string(argBytes))
	case TypeFunction:
		return "<function>"
	default:
		return v.StrVal
	}
}

// AsInt converts value to int64.
func (v *Value) AsInt() (int64, error) {
	if v == nil {
		return 0, nil
	}
	switch v.TypeVal {
	case TypeInt:
		return v.IntVal, nil
	case TypeFloat:
		return int64(v.FloatVal), nil
	case TypeBool:
		if v.BoolVal {
			return 1, nil
		}
		return 0, nil
	case TypeString:
		return strconv.ParseInt(strings.TrimSpace(v.StrVal), 0, 64)
	default:
		return 0, fmt.Errorf("cannot convert %s to int", v.TypeVal)
	}
}

// AsFloat converts value to float64.
func (v *Value) AsFloat() (float64, error) {
	if v == nil {
		return 0, nil
	}
	switch v.TypeVal {
	case TypeFloat:
		return v.FloatVal, nil
	case TypeInt:
		return float64(v.IntVal), nil
	case TypeBool:
		if v.BoolVal {
			return 1.0, nil
		}
		return 0.0, nil
	case TypeString:
		return strconv.ParseFloat(strings.TrimSpace(v.StrVal), 64)
	default:
		return 0, fmt.Errorf("cannot convert %s to float", v.TypeVal)
	}
}

// AsBool converts value to boolean.
func (v *Value) AsBool() bool {
	return v.IsTruthy()
}

// Index retrieves an element by integer or string key.
func (v *Value) Index(key *Value) (*Value, error) {
	if v == nil {
		return NewNull(), nil
	}
	switch v.TypeVal {
	case TypeList:
		idx, err := key.AsInt()
		if err != nil {
			return nil, fmt.Errorf("list index must be integer, got %s", key.String())
		}
		if idx < 0 {
			idx = int64(len(v.ListVal)) + idx
		}
		if idx < 0 || idx >= int64(len(v.ListVal)) {
			return nil, fmt.Errorf("list index %d out of range (length %d)", idx, len(v.ListVal))
		}
		return v.ListVal[idx], nil

	case TypeDict:
		k := key.String()
		if val, ok := v.DictVal[k]; ok {
			return val, nil
		}
		return NewNull(), nil

	case TypeResponse:
		k := strings.ToLower(key.String())
		switch k {
		case "response", "text":
			return NewString(v.RespVal.Text), nil
		case "reasoning", "thought", "thinking":
			return NewString(v.RespVal.Reasoning), nil
		case "tools", "tool_calls":
			items := make([]*Value, len(v.RespVal.ToolCalls))
			for i, tc := range v.RespVal.ToolCalls {
				items[i] = NewToolCall(tc)
			}
			return NewList(items...), nil
		case "usage":
			if v.RespVal.Usage == nil {
				return NewNull(), nil
			}
			m := map[string]*Value{
				"input_tokens":  NewInt(int64(v.RespVal.Usage.InputTokens)),
				"output_tokens": NewInt(int64(v.RespVal.Usage.OutputTokens)),
				"total_tokens":  NewInt(int64(v.RespVal.Usage.TotalTokens)),
			}
			return NewDict(m), nil
		case "model":
			return NewString(v.RespVal.Model), nil
		case "steps":
			items := make([]*Value, len(v.RespVal.Steps))
			for i, st := range v.RespVal.Steps {
				obsList := make([]*Value, len(st.Observations))
				for j, obs := range st.Observations {
					obsList[j] = NewString(obs)
				}
				stepTools := make([]*Value, len(st.ToolCalls))
				for j, tc := range st.ToolCalls {
					stepTools[j] = NewToolCall(tc)
				}
				stepDict := map[string]*Value{
					"turn":         NewInt(int64(st.Turn)),
					"thought":      NewString(st.Thought),
					"tool_calls":   NewList(stepTools...),
					"observations": NewList(obsList...),
					"response":     NewString(st.Response),
				}
				items[i] = NewDict(stepDict)
			}
			return NewList(items...), nil
		case "tools_used":
			seen := make(map[string]bool)
			var list []*Value
			for _, st := range v.RespVal.Steps {
				for _, tc := range st.ToolCalls {
					if !seen[tc.Name] {
						seen[tc.Name] = true
						list = append(list, NewString(tc.Name))
					}
				}
			}
			for _, tc := range v.RespVal.ToolCalls {
				if !seen[tc.Name] {
					seen[tc.Name] = true
					list = append(list, NewString(tc.Name))
				}
			}
			return NewList(list...), nil
		case "turn_count", "turns":
			return NewInt(int64(len(v.RespVal.Steps))), nil
		default:
			return NewNull(), nil
		}

	case TypeToolCall:
		k := strings.ToLower(key.String())
		switch k {
		case "id":
			return NewString(v.ToolVal.ID), nil
		case "name":
			return NewString(v.ToolVal.Name), nil
		case "arguments", "args":
			m := make(map[string]*Value)
			for ak, av := range v.ToolVal.Arguments {
				m[ak] = FromNative(av)
			}
			return NewDict(m), nil
		default:
			return NewNull(), nil
		}

	case TypeString:
		runes := []rune(v.StrVal)
		idx, err := key.AsInt()
		if err != nil {
			return nil, fmt.Errorf("string index must be integer, got %s", key.String())
		}
		if idx < 0 {
			idx = int64(len(runes)) + idx
		}
		if idx < 0 || idx >= int64(len(runes)) {
			return nil, fmt.Errorf("string index %d out of range", idx)
		}
		return NewString(string(runes[idx])), nil

	default:
		return nil, fmt.Errorf("indexing not supported on type %s", v.TypeVal)
	}
}

// FromNative converts generic Go interfaces to PCL Values.
func FromNative(val interface{}) *Value {
	if val == nil {
		return NewNull()
	}
	switch v := val.(type) {
	case *Value:
		return v
	case string:
		return NewString(v)
	case int:
		return NewInt(int64(v))
	case int64:
		return NewInt(v)
	case float64:
		return NewFloat(v)
	case bool:
		return NewBool(v)
	case []interface{}:
		items := make([]*Value, len(v))
		for i, el := range v {
			items[i] = FromNative(el)
		}
		return NewList(items...)
	case []*Value:
		return NewList(v...)
	case map[string]interface{}:
		m := make(map[string]*Value)
		for k, el := range v {
			m[k] = FromNative(el)
		}
		return NewDict(m)
	case map[string]*Value:
		return NewDict(v)
	case *Response:
		return NewResponse(v)
	case *ToolCall:
		return NewToolCall(v)
	default:
		return NewString(fmt.Sprintf("%v", v))
	}
}

// ToNative converts a Value back to standard Go primitives.
func (v *Value) ToNative() interface{} {
	if v == nil {
		return nil
	}
	switch v.TypeVal {
	case TypeNull:
		return nil
	case TypeString:
		return v.StrVal
	case TypeInt:
		return v.IntVal
	case TypeFloat:
		return v.FloatVal
	case TypeBool:
		return v.BoolVal
	case TypeList:
		res := make([]interface{}, len(v.ListVal))
		for i, el := range v.ListVal {
			res[i] = el.ToNative()
		}
		return res
	case TypeDict:
		res := make(map[string]interface{})
		for k, el := range v.DictVal {
			res[k] = el.ToNative()
		}
		return res
	case TypeResponse:
		return v.RespVal
	case TypeToolCall:
		return v.ToolVal
	default:
		return v.StrVal
	}
}

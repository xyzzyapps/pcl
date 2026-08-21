package core

import (
	"errors"
	"fmt"
	"strings"
)

// Loop flow-control sentinels (not runtime failures).
var (
	ErrBreak    = errors.New("break")
	ErrContinue = errors.New("continue")
)

func IsFlow(err error) bool {
	return errors.Is(err, ErrBreak) || errors.Is(err, ErrContinue)
}

// ErrorCode categorizes interpreter errors.
type ErrorCode int

const (
	ErrSyntax ErrorCode = iota
	ErrRuntime
	ErrCommandNotFound
	ErrInvalidArgs
	ErrVariableNotFound
	ErrFFI
	ErrAI
	ErrIO
	ErrProcess
)

func (c ErrorCode) String() string {
	switch c {
	case ErrSyntax:
		return "SyntaxError"
	case ErrRuntime:
		return "RuntimeError"
	case ErrCommandNotFound:
		return "CommandNotFound"
	case ErrInvalidArgs:
		return "InvalidArgs"
	case ErrVariableNotFound:
		return "VariableNotFound"
	case ErrFFI:
		return "FFIError"
	case ErrAI:
		return "AIError"
	case ErrIO:
		return "IOError"
	case ErrProcess:
		return "ProcessError"
	default:
		return "Error"
	}
}

// PCLError is a rich structured error in PCL with position and stack trace.
type PCLError struct {
	Code     ErrorCode
	Message  string
	Line     int
	Column   int
	File     string
	Frames   []string
	CauseErr error
}

func NewError(code ErrorCode, msg string) *PCLError {
	return &PCLError{
		Code:    code,
		Message: msg,
		Frames:  make([]string, 0),
	}
}

func NewErrorf(code ErrorCode, format string, args ...interface{}) *PCLError {
	return &PCLError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Frames:  make([]string, 0),
	}
}

func (e *PCLError) WithPosition(file string, line, col int) *PCLError {
	e.File = file
	e.Line = line
	e.Column = col
	return e
}

func (e *PCLError) WithFrame(frame string) *PCLError {
	e.Frames = append(e.Frames, frame)
	return e
}

func (e *PCLError) WithCause(err error) *PCLError {
	e.CauseErr = err
	return e
}

func (e *PCLError) Error() string {
	var sb strings.Builder
	if e.File != "" || e.Line > 0 {
		sb.WriteString(fmt.Sprintf("%s:%d:%d: ", e.File, e.Line, e.Column))
	}
	sb.WriteString(fmt.Sprintf("%s: %s", e.Code.String(), e.Message))
	if e.CauseErr != nil {
		sb.WriteString(fmt.Sprintf(" (caused by: %v)", e.CauseErr))
	}
	if len(e.Frames) > 0 {
		sb.WriteString("\nStack trace:\n")
		for i := len(e.Frames) - 1; i >= 0; i-- {
			sb.WriteString(fmt.Sprintf("  at %s\n", e.Frames[i]))
		}
	}
	return sb.String()
}

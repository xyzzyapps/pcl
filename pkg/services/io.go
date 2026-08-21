package services

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

// IOService defines input/output streaming operations.
type IOService interface {
	Stdin() io.Reader
	Stdout() io.Writer
	Stderr() io.Writer
	Print(a ...interface{})
	Printf(format string, a ...interface{})
	Println(a ...interface{})
	PrintError(a ...interface{})
	PrintfError(format string, a ...interface{})
	ReadLine() (string, error)
}

// DefaultIOService uses standard OS streams.
type DefaultIOService struct {
	in     io.Reader
	out    io.Writer
	err    io.Writer
	reader *bufio.Reader
}

func NewDefaultIOService() *DefaultIOService {
	return &DefaultIOService{
		in:     os.Stdin,
		out:    os.Stdout,
		err:    os.Stderr,
		reader: bufio.NewReader(os.Stdin),
	}
}

func NewCustomIOService(in io.Reader, out, err io.Writer) *DefaultIOService {
	var r *bufio.Reader
	if in != nil {
		r = bufio.NewReader(in)
	}
	return &DefaultIOService{
		in:     in,
		out:    out,
		err:    err,
		reader: r,
	}
}

func (s *DefaultIOService) Stdin() io.Reader {
	return s.in
}

func (s *DefaultIOService) Stdout() io.Writer {
	return s.out
}

func (s *DefaultIOService) Stderr() io.Writer {
	return s.err
}

func (s *DefaultIOService) Print(a ...interface{}) {
	fmt.Fprint(s.out, a...)
}

func (s *DefaultIOService) Printf(format string, a ...interface{}) {
	fmt.Fprintf(s.out, format, a...)
}

func (s *DefaultIOService) Println(a ...interface{}) {
	fmt.Fprintln(s.out, a...)
}

func (s *DefaultIOService) PrintError(a ...interface{}) {
	fmt.Fprint(s.err, a...)
}

func (s *DefaultIOService) PrintfError(format string, a ...interface{}) {
	fmt.Fprintf(s.err, format, a...)
}

func (s *DefaultIOService) ReadLine() (string, error) {
	if s.reader == nil {
		return "", io.EOF
	}
	line, err := s.reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	// Trim trailing \r\n
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line, nil
}

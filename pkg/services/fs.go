package services

import (
	"os"
	"os/exec"
	"path/filepath"
)

// FSService defines file system operations.
type FSService interface {
	Getwd() (string, error)
	Chdir(dir string) error
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error
	AppendFile(filename string, data []byte, perm os.FileMode) error
	Stat(name string) (os.FileInfo, error)
	LookPath(file string) (string, error)
	UserHomeDir() (string, error)
}

// DefaultFSService uses native OS filesystem calls.
type DefaultFSService struct {
	cwd string
}

func NewDefaultFSService() *DefaultFSService {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	return &DefaultFSService{
		cwd: wd,
	}
}

func (f *DefaultFSService) Getwd() (string, error) {
	wd, err := os.Getwd()
	if err == nil {
		f.cwd = wd
	}
	return f.cwd, nil
}

func (f *DefaultFSService) Chdir(dir string) error {
	// Expand home directory if starts with ~
	if dir == "~" || len(dir) > 1 && (dir[:2] == "~/" || dir[:2] == "~\\") {
		home, err := os.UserHomeDir()
		if err == nil {
			if dir == "~" {
				dir = home
			} else {
				dir = filepath.Join(home, dir[2:])
			}
		}
	}
	err := os.Chdir(dir)
	if err == nil {
		f.cwd, _ = os.Getwd()
	}
	return err
}

func (f *DefaultFSService) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (f *DefaultFSService) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (f *DefaultFSService) AppendFile(filename string, data []byte, perm os.FileMode) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func (f *DefaultFSService) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (f *DefaultFSService) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (f *DefaultFSService) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}



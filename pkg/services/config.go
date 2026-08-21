package services

import (
	"os"
	"path/filepath"
	"sync"
)

// ConfigService manages runtime configurations and loads ~/.pclrc or ./config.pcl.
type ConfigService interface {
	Get(key string) string
	Set(key, val string)
	GetAll() map[string]string
	LoadConfigFile(path string) error
	FindDefaultConfigFile() string
}

type DefaultConfigService struct {
	mu     sync.RWMutex
	values map[string]string
}

func NewDefaultConfigService() *DefaultConfigService {
	cfg := &DefaultConfigService{
		values: make(map[string]string),
	}
	// Defaults
	cfg.values["model"] = "gemini-2.5-flash"
	cfg.values["temperature"] = "0.7"
	cfg.values["prompt"] = "pcl> "
	cfg.values["multiline_prompt"] = "...> "
	cfg.values["system_prompt"] = "You are an intelligent assistant integrated into the PCL shell."
	return cfg
}

func (c *DefaultConfigService) Get(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.values[key]
}

func (c *DefaultConfigService) Set(key, val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = val
}

func (c *DefaultConfigService) GetAll() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make(map[string]string, len(c.values))
	for k, v := range c.values {
		res[k] = v
	}
	return res
}

func (c *DefaultConfigService) LoadConfigFile(path string) error {
	// Loading and evaluating is typically performed by Interpreter running source config.pcl
	_, err := os.Stat(path)
	return err
}

func (c *DefaultConfigService) FindDefaultConfigFile() string {
	// 1. Check current directory config.pcl
	if _, err := os.Stat("config.pcl"); err == nil {
		return "config.pcl"
	}
	// 2. Check ~/.pclrc
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".pclrc")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

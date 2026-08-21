package services

import (
	"sync"
)

// ServiceLocator manages references to core infrastructure services.
type ServiceLocator struct {
	mu      sync.RWMutex
	io      IOService
	fs      FSService
	proc    ProcessService
	ai      AIService
	config  ConfigService
}

var globalLocator *ServiceLocator
var once sync.Once

// GetLocator returns the singleton ServiceLocator instance.
func GetLocator() *ServiceLocator {
	once.Do(func() {
		globalLocator = NewServiceLocator()
	})
	return globalLocator
}

// NewServiceLocator creates a new locator instance initialized with defaults.
func NewServiceLocator() *ServiceLocator {
	loc := &ServiceLocator{}
	loc.io = NewDefaultIOService()
	loc.fs = NewDefaultFSService()
	loc.proc = NewDefaultProcessService(loc.fs)
	loc.config = NewDefaultConfigService()
	return loc
}

// IO service getters/setters
func (l *ServiceLocator) IO() IOService {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.io
}

func (l *ServiceLocator) SetIO(svc IOService) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.io = svc
}

// FS service getters/setters
func (l *ServiceLocator) FS() FSService {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.fs
}

func (l *ServiceLocator) SetFS(svc FSService) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fs = svc
}

// Process service getters/setters
func (l *ServiceLocator) Process() ProcessService {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.proc
}

func (l *ServiceLocator) SetProcess(svc ProcessService) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.proc = svc
}

// AI service getters/setters
func (l *ServiceLocator) AI() AIService {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.ai
}

func (l *ServiceLocator) SetAI(svc AIService) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ai = svc
}

// Config service getters/setters
func (l *ServiceLocator) Config() ConfigService {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.config
}

func (l *ServiceLocator) SetConfig(svc ConfigService) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.config = svc
}

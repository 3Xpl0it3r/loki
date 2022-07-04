package filesystem

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type ManagerInterface interface {
	// Register is used to add handler to manager
	Register(id string, handler Handler)
	//
	Get() Handler
	Stop()

	AwaitAllClosed() bool
}

// Manager is used to manager all handler that register to manager
type Manager struct {
	sync.Mutex
	handlers map[string]Handler
}

func newHandlerManager() *Manager {
	h := &Manager{
		handlers: make(map[string]Handler),
	}
	return h
}

// if handler is register in manager then return false
func (m *Manager) Get(id string) Handler {
	if _, ok := m.handlers[id]; ok {
		handler := m.handlers[id]
		return handler
	}
	return nil
}

// if handler, use cow
func (m *Manager) unRegister(id string) error {
	if _, ok := m.handlers[id]; !ok {
		return fmt.Errorf("handler %s is not register in manager", id)
	}
	ht := m.clone()
	delete(ht, id)
	// lock is no need in here ,because pointer operator is atomic, in intel x86 architecture 1/4/8 byte operator is atomic
	m.handlers = ht
	return nil
}

// AddHandler add handler,  use cow tel to avoid lock
func (m *Manager) Register(id string, handler Handler) error {
	if _, ok := m.handlers[id]; ok {
		return fmt.Errorf("handler %s has installed ", id)
	}
	ht := m.clone()
	ht[id] = handler
	// here is no need lock ,because pointer write/read is atomic
	m.handlers = ht

	return nil
}

// clone
func (m *Manager) clone() map[string]Handler {
	ht := make(map[string]Handler, len(m.handlers))
	for k, v := range m.handlers {
		ht[k] = v
	}
	return ht
}

func (m *Manager) Stop() {
	for _, v := range m.handlers {
		v.Stop()
	}
}

func (m *Manager) HandlerSize() int {
	return len(m.handlers)
}


func(m *Manager)AwaitComplete()error{
	timeoutCtx, cancel:= context.WithTimeout(context.Background(), 5 * time.Minute)
	defer cancel()
	for len(m.handlers) != 0{
		select {
		case <-timeoutCtx.Done():
			return timeoutCtx.Err()
		default:
			time.Sleep(1 * time.Second)
		}
	}
	return nil
}
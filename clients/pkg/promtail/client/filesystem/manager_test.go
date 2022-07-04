package filesystem

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestManager_register(t *testing.T) {
	t.Parallel()
	cases1 := map[string]Handler{
		"h1": &FakeFileHandler{},
		"h2": &FakeFileHandler{},
		"h3": &FakeFileHandler{},
		"h4": &FakeFileHandler{},
	}
	manager := Manager{
		handlers: make(map[string]Handler),
	}
	for id, h := range cases1 {
		t.Run(id, func(t *testing.T) {
			require.NoError(t, manager.Register(id, h))
		})
	}
	assert.Error(t, manager.Register("h1", nil))
}

func TestManager_unregister(t *testing.T) {
	t.Parallel()
	case1 := map[string]Handler{
		"h1": &FakeFileHandler{},
		"h2": &FakeFileHandler{},
		"h3": &FakeFileHandler{},
		"h4": &FakeFileHandler{},
	}
	manager := Manager{handlers: case1}

	for id, _ := range case1 {
		t.Run(id, func(t *testing.T) {
			require.NoError(t, manager.unRegister(id))
		})
	}
}

func TestManagerRW(t *testing.T) {
	t.Parallel()
	cases1 := map[string]Handler{
		"h1": &FakeFileHandler{},
		"h2": &FakeFileHandler{},
		"h3": &FakeFileHandler{},
		"h4": &FakeFileHandler{},
	}
	cases2 := map[string]Handler{
		"h5": &FakeFileHandler{},
		"h6": &FakeFileHandler{},
		"h7": &FakeFileHandler{},
		"h8": &FakeFileHandler{},
	}
	manager := Manager{handlers: cases1}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go __getWorker(t, wg, &manager, cases2)
	go __unregister(t, wg, &manager, cases1)
	wg.Wait()
}

func __getWorker(t *testing.T, wg *sync.WaitGroup, m *Manager, handlers map[string]Handler) {
	defer wg.Done()
	for id, h := range handlers {
		t.Run(id, func(t *testing.T) {
			require.NoError(t, m.Register(id, h))
		})
	}
}

func __unregister(t *testing.T, wg *sync.WaitGroup, m *Manager, handlers map[string]Handler) {
	defer wg.Done()
	for id, _ := range handlers {
		t.Run(id, func(t *testing.T) {
			require.NoError(t, m.unRegister(id))
		})
	}
}

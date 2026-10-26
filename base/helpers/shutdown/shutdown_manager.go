package shutdown

import (
	"context"
	"sync"
)

type Manager struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{ctx: ctx, cancel: cancel}
}

func (m *Manager) Context() context.Context {
	return m.ctx
}

func (m *Manager) AddWorker() {
	m.wg.Add(1)
}

func (m *Manager) WorkerDone() {
	m.wg.Done()
}

func (m *Manager) Shutdown() {
	m.cancel()
}

func (m *Manager) Wait() {
	m.wg.Wait()
}

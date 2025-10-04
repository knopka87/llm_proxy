package ocr

import (
	"context"
	"sync"
)

type Engine interface {
	Name() string
	Recognize(ctx context.Context, image []byte, opt Options) (string, error)
}

type Manager struct {
	def Engine
	m   sync.Map // chatID -> Engine
}

func NewManager(defaultEngine Engine) *Manager {
	return &Manager{def: defaultEngine}
}

func (m *Manager) Get(chatID int64) Engine {
	if v, ok := m.m.Load(chatID); ok {
		return v.(Engine)
	}
	return m.def
}
func (m *Manager) Set(chatID int64, e Engine) {
	m.m.Store(chatID, e)
}

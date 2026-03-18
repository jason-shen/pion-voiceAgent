package room

import (
	"log"
	"sync"

	"github.com/voiceagent/server/internal/config"
)

type Manager struct {
	cfg   *config.Config
	mu    sync.RWMutex
	rooms map[string]*Room
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:   cfg,
		rooms: make(map[string]*Room),
	}
}

func (m *Manager) GetOrCreate(roomID string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()

	if r, ok := m.rooms[roomID]; ok {
		return r
	}

	r := NewRoom(roomID, m.cfg)
	m.rooms[roomID] = r
	log.Printf("[manager] created room %s", roomID)
	return r
}

func (m *Manager) Get(roomID string) *Room {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rooms[roomID]
}

func (m *Manager) Remove(roomID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rooms[roomID]; ok {
		r.Close()
		delete(m.rooms, roomID)
		log.Printf("[manager] removed room %s", roomID)
	}
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, r := range m.rooms {
		r.Close()
		delete(m.rooms, id)
	}
	log.Println("[manager] all rooms closed")
}

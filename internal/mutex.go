package internal

import "sync"

type MutexByRoom struct {
	mu       *sync.Mutex // protects the map
	roomToMu map[string]*sync.Mutex
}

func NewMutexByRoom() *MutexByRoom {
	return &MutexByRoom{
		mu:       &sync.Mutex{},
		roomToMu: make(map[string]*sync.Mutex),
	}
}

func (m *MutexByRoom) Lock(roomID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	roomMu := m.roomToMu[roomID]
	if roomMu == nil {
		roomMu = &sync.Mutex{}
	}
	roomMu.Lock()
	m.roomToMu[roomID] = roomMu
}

func (m *MutexByRoom) Unlock(roomID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	roomMu := m.roomToMu[roomID]
	if roomMu == nil {
		panic("MutexByRoom: Unlock before Lock")
	}
	roomMu.Unlock()
}

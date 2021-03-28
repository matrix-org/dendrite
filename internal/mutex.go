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
	roomMu := m.roomToMu[roomID]
	if roomMu == nil {
		roomMu = &sync.Mutex{}
	}
	m.roomToMu[roomID] = roomMu
	m.mu.Unlock()
	// don't lock inside m.mu else we can deadlock
	roomMu.Lock()
}

func (m *MutexByRoom) Unlock(roomID string) {
	m.mu.Lock()
	roomMu := m.roomToMu[roomID]
	if roomMu == nil {
		panic("MutexByRoom: Unlock before Lock")
	}
	m.mu.Unlock()

	roomMu.Unlock()
}

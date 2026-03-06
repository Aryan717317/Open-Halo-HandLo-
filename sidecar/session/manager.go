package session

import (
	"sync"
)

type Session struct {
	PeerID        string
	EncryptionKey []byte
	HMACKey       []byte
	Token         string
	IsPaired      bool
	TempPublicKey string // Initiator's public key during handshake
}

type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

func (m *Manager) Add(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.PeerID] = s
}

func (m *Manager) Get(peerID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[peerID]
	return s, ok
}

func (m *Manager) GetByToken(token string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if s.Token == token {
			return s, bool(true)
		}
	}
	return nil, false
}

func (m *Manager) Iterate(fn func(*Session)) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		fn(s)
	}
}

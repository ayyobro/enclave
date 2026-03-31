package client

import "sync"

// PresenceTracker maintains online/offline status of contacts.
type PresenceTracker struct {
	mu     sync.RWMutex
	online map[string]bool // pubkey -> online
}

func NewPresenceTracker() *PresenceTracker {
	return &PresenceTracker{
		online: make(map[string]bool),
	}
}

func (p *PresenceTracker) SetOnline(pubKey string, online bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.online[pubKey] = online
}

func (p *PresenceTracker) IsOnline(pubKey string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.online[pubKey]
}

func (p *PresenceTracker) SetAll(users map[string]bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range users {
		p.online[k] = v
	}
}

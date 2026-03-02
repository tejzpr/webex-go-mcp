package auth

import (
	"fmt"
	"sync"
	"time"
)

// MemoryStore implements Store using in-memory maps with mutex synchronization.
// This is the default store — fast but not persistent across restarts.
type MemoryStore struct {
	mu           sync.RWMutex
	tokens       map[string]*TokenRecord
	authCodes    map[string]*AuthCodeRecord
	pendingAuths map[string]*PendingAuth
	clients      map[string]*RegisteredClient
	stopCleanup  chan struct{}
}

// NewMemoryStore creates a new in-memory store with periodic cleanup.
func NewMemoryStore(cleanupInterval time.Duration) *MemoryStore {
	ms := &MemoryStore{
		tokens:       make(map[string]*TokenRecord),
		authCodes:    make(map[string]*AuthCodeRecord),
		pendingAuths: make(map[string]*PendingAuth),
		clients:      make(map[string]*RegisteredClient),
		stopCleanup:  make(chan struct{}),
	}
	go ms.cleanup(cleanupInterval)
	return ms
}

// --- Token records ---

func (ms *MemoryStore) StoreToken(webexAccessToken, webexRefreshToken string, expiresIn int) (string, error) {
	opaque, err := generateSecureToken(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate opaque token: %w", err)
	}

	record := &TokenRecord{
		OpaqueToken:       opaque,
		WebexAccessToken:  webexAccessToken,
		WebexRefreshToken: webexRefreshToken,
		ExpiresAt:         time.Now().Add(time.Duration(expiresIn) * time.Second),
		CreatedAt:         time.Now(),
	}

	ms.mu.Lock()
	ms.tokens[opaque] = record
	ms.mu.Unlock()

	return opaque, nil
}

func (ms *MemoryStore) LookupToken(opaqueToken string) (*TokenRecord, bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	record, ok := ms.tokens[opaqueToken]
	if !ok {
		return nil, false
	}
	return record, true
}

func (ms *MemoryStore) UpdateWebexToken(opaqueToken, newAccessToken, newRefreshToken string, expiresIn int) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if record, ok := ms.tokens[opaqueToken]; ok {
		record.WebexAccessToken = newAccessToken
		record.WebexRefreshToken = newRefreshToken
		record.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	}
	return nil
}

func (ms *MemoryStore) RevokeToken(opaqueToken string) {
	ms.mu.Lock()
	delete(ms.tokens, opaqueToken)
	ms.mu.Unlock()
}

// --- Authorization codes ---

func (ms *MemoryStore) StoreAuthCode(record *AuthCodeRecord) error {
	ms.mu.Lock()
	ms.authCodes[record.Code] = record
	ms.mu.Unlock()
	return nil
}

func (ms *MemoryStore) ConsumeAuthCode(code string) (*AuthCodeRecord, bool) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	record, ok := ms.authCodes[code]
	if !ok {
		return nil, false
	}
	delete(ms.authCodes, code)
	if time.Now().After(record.ExpiresAt) {
		return nil, false
	}
	return record, true
}

// --- Pending auth state ---

func (ms *MemoryStore) StorePendingAuth(pending *PendingAuth) error {
	ms.mu.Lock()
	ms.pendingAuths[pending.State] = pending
	ms.mu.Unlock()
	return nil
}

func (ms *MemoryStore) ConsumePendingAuth(state string) (*PendingAuth, bool) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	pending, ok := ms.pendingAuths[state]
	if !ok {
		return nil, false
	}
	delete(ms.pendingAuths, state)
	if time.Since(pending.CreatedAt) > 10*time.Minute {
		return nil, false
	}
	return pending, true
}

// --- Client registry ---

func (ms *MemoryStore) RegisterClient(req *RegistrationRequest) (*RegisteredClient, error) {
	client, err := prepareClientRegistration(req)
	if err != nil {
		return nil, err
	}

	ms.mu.Lock()
	ms.clients[client.ClientID] = client
	ms.mu.Unlock()

	return client, nil
}

func (ms *MemoryStore) RegisterClientWithID(clientID, redirectURI string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if existing, ok := ms.clients[clientID]; ok {
		if matchesRedirectURI(existing.RedirectURIs, redirectURI) {
			return nil
		}
		existing.RedirectURIs = append(existing.RedirectURIs, redirectURI)
		return nil
	}

	ms.clients[clientID] = &RegisteredClient{
		ClientID:                clientID,
		RedirectURIs:            []string{redirectURI},
		TokenEndpointAuthMethod: "none",
		GrantTypes:              []string{"authorization_code"},
		ResponseTypes:           []string{"code"},
		CreatedAt:               time.Now(),
	}
	return nil
}

func (ms *MemoryStore) LookupClient(clientID string) (*RegisteredClient, bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	client, ok := ms.clients[clientID]
	return client, ok
}

func (ms *MemoryStore) ValidateRedirectURI(clientID, redirectURI string) bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	client, ok := ms.clients[clientID]
	if !ok {
		return false
	}
	return matchesRedirectURI(client.RedirectURIs, redirectURI)
}

// --- Lifecycle ---

func (ms *MemoryStore) Close() error {
	close(ms.stopCleanup)
	return nil
}

func (ms *MemoryStore) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ms.stopCleanup:
			return
		case <-ticker.C:
			now := time.Now()
			ms.mu.Lock()
			for k, v := range ms.authCodes {
				if now.After(v.ExpiresAt) {
					delete(ms.authCodes, k)
				}
			}
			for k, v := range ms.pendingAuths {
				if now.Sub(v.CreatedAt) > 10*time.Minute {
					delete(ms.pendingAuths, k)
				}
			}
			ms.mu.Unlock()
		}
	}
}

package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// TokenRecord holds the Webex tokens associated with an opaque Bearer token.
type TokenRecord struct {
	OpaqueToken       string    `json:"opaque_token"`
	WebexAccessToken  string    `json:"webex_access_token"`
	WebexRefreshToken string    `json:"webex_refresh_token"`
	ExpiresAt         time.Time `json:"expires_at"`
	UserID            string    `json:"user_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// AuthCodeRecord holds a pending authorization code awaiting exchange.
type AuthCodeRecord struct {
	Code              string    `json:"code"`
	ClientID          string    `json:"client_id"`
	RedirectURI       string    `json:"redirect_uri"`
	CodeVerifier      string    `json:"code_verifier,omitempty"`
	WebexAccessToken  string    `json:"webex_access_token"`
	WebexRefreshToken string    `json:"webex_refresh_token"`
	WebexExpiresIn    int       `json:"webex_expires_in"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// PendingAuth holds state for an in-flight OAuth authorization request.
type PendingAuth struct {
	State             string `json:"state"`
	ClientID          string `json:"client_id"`
	ClientRedirectURI string `json:"client_redirect_uri"`
	ClientState       string `json:"client_state,omitempty"`
	CodeChallenge     string `json:"code_challenge,omitempty"`
	CodeChallengeMethod string `json:"code_challenge_method,omitempty"`
	WebexCodeVerifier string `json:"webex_code_verifier"`
	CreatedAt         time.Time `json:"created_at"`
}

// TokenStore manages opaque tokens, auth codes, and pending auth state in memory.
type TokenStore struct {
	mu           sync.RWMutex
	tokens       map[string]*TokenRecord   // opaque token -> record
	authCodes    map[string]*AuthCodeRecord // auth code -> record
	pendingAuths map[string]*PendingAuth   // state -> pending auth
}

// NewTokenStore creates a new in-memory token store.
func NewTokenStore() *TokenStore {
	ts := &TokenStore{
		tokens:       make(map[string]*TokenRecord),
		authCodes:    make(map[string]*AuthCodeRecord),
		pendingAuths: make(map[string]*PendingAuth),
	}
	go ts.cleanup()
	return ts
}

// StoreToken saves a token record and returns the opaque token.
func (ts *TokenStore) StoreToken(webexAccessToken, webexRefreshToken string, expiresIn int) (string, error) {
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

	ts.mu.Lock()
	ts.tokens[opaque] = record
	ts.mu.Unlock()

	return opaque, nil
}

// LookupToken retrieves the token record for an opaque token.
func (ts *TokenStore) LookupToken(opaqueToken string) (*TokenRecord, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	record, ok := ts.tokens[opaqueToken]
	if !ok {
		return nil, false
	}
	return record, true
}

// UpdateWebexToken updates the Webex tokens for an existing opaque token (used after refresh).
func (ts *TokenStore) UpdateWebexToken(opaqueToken, newAccessToken, newRefreshToken string, expiresIn int) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if record, ok := ts.tokens[opaqueToken]; ok {
		record.WebexAccessToken = newAccessToken
		record.WebexRefreshToken = newRefreshToken
		record.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	}
}

// RevokeToken removes an opaque token.
func (ts *TokenStore) RevokeToken(opaqueToken string) {
	ts.mu.Lock()
	delete(ts.tokens, opaqueToken)
	ts.mu.Unlock()
}

// StoreAuthCode saves an authorization code record.
func (ts *TokenStore) StoreAuthCode(record *AuthCodeRecord) {
	ts.mu.Lock()
	ts.authCodes[record.Code] = record
	ts.mu.Unlock()
}

// ConsumeAuthCode retrieves and deletes an authorization code (one-time use).
func (ts *TokenStore) ConsumeAuthCode(code string) (*AuthCodeRecord, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	record, ok := ts.authCodes[code]
	if !ok {
		return nil, false
	}
	delete(ts.authCodes, code)
	if time.Now().After(record.ExpiresAt) {
		return nil, false
	}
	return record, true
}

// StorePendingAuth saves a pending authorization state.
func (ts *TokenStore) StorePendingAuth(pending *PendingAuth) {
	ts.mu.Lock()
	ts.pendingAuths[pending.State] = pending
	ts.mu.Unlock()
}

// ConsumePendingAuth retrieves and deletes a pending auth by state (one-time use).
func (ts *TokenStore) ConsumePendingAuth(state string) (*PendingAuth, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	pending, ok := ts.pendingAuths[state]
	if !ok {
		return nil, false
	}
	delete(ts.pendingAuths, state)
	// Expire after 10 minutes
	if time.Since(pending.CreatedAt) > 10*time.Minute {
		return nil, false
	}
	return pending, true
}

// cleanup periodically removes expired entries.
func (ts *TokenStore) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		ts.mu.Lock()
		for k, v := range ts.authCodes {
			if now.After(v.ExpiresAt) {
				delete(ts.authCodes, k)
			}
		}
		for k, v := range ts.pendingAuths {
			if now.Sub(v.CreatedAt) > 10*time.Minute {
				delete(ts.pendingAuths, k)
			}
		}
		ts.mu.Unlock()
	}
}

// generateSecureToken generates a cryptographically secure random hex string.
func generateSecureToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateAuthCode generates a random authorization code.
func GenerateAuthCode() (string, error) {
	return generateSecureToken(16)
}

// GenerateState generates a random state parameter.
func GenerateState() (string, error) {
	return generateSecureToken(16)
}

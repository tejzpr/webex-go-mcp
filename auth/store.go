package auth

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// NOTE: The old TokenStore struct and its methods have been removed.
// All storage is now handled through the Store interface (see storage.go)
// with implementations in storage_memory.go, storage_sqlite.go, and storage_postgres.go.

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
	State               string    `json:"state"`
	ClientID            string    `json:"client_id"`
	ClientRedirectURI   string    `json:"client_redirect_uri"`
	ClientState         string    `json:"client_state,omitempty"`
	CodeChallenge       string    `json:"code_challenge,omitempty"`
	CodeChallengeMethod string    `json:"code_challenge_method,omitempty"`
	WebexCodeVerifier   string    `json:"webex_code_verifier"`
	CreatedAt           time.Time `json:"created_at"`
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

package auth

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Store is the persistence interface for OAuth state: tokens, auth codes,
// pending authorization flows, and dynamically registered clients.
// Implementations: MemoryStore (default), SQLiteStore, PostgresStore.
type Store interface {
	// --- Token records ---

	// StoreToken persists a new token record and returns the generated opaque token.
	StoreToken(webexAccessToken, webexRefreshToken string, expiresIn int) (string, error)

	// LookupToken retrieves a token record by opaque token.
	LookupToken(opaqueToken string) (*TokenRecord, bool)

	// UpdateWebexToken updates the Webex tokens for an existing opaque token (after refresh).
	UpdateWebexToken(opaqueToken, newAccessToken, newRefreshToken string, expiresIn int) error

	// RevokeToken removes an opaque token.
	RevokeToken(opaqueToken string)

	// --- Authorization codes ---

	// StoreAuthCode persists an authorization code record.
	StoreAuthCode(record *AuthCodeRecord) error

	// ConsumeAuthCode retrieves and deletes an authorization code (one-time use).
	ConsumeAuthCode(code string) (*AuthCodeRecord, bool)

	// --- Pending auth state ---

	// StorePendingAuth persists a pending authorization state.
	StorePendingAuth(pending *PendingAuth) error

	// ConsumePendingAuth retrieves and deletes a pending auth by state (one-time use).
	ConsumePendingAuth(state string) (*PendingAuth, bool)

	// --- Client registry (DCR) ---

	// RegisterClient creates a new dynamically registered client.
	RegisterClient(req *RegistrationRequest) (*RegisteredClient, error)

	// RegisterClientWithID registers (or updates) a client with a known client_id.
	RegisterClientWithID(clientID, redirectURI string) error

	// LookupClient retrieves a registered client by client_id.
	LookupClient(clientID string) (*RegisteredClient, bool)

	// ValidateRedirectURI checks if the given redirect_uri is allowed for the client.
	ValidateRedirectURI(clientID, redirectURI string) bool

	// --- Lifecycle ---

	// Close releases any resources held by the store (DB connections, etc.).
	Close() error
}

// StoreConfig holds configuration for creating a Store.
type StoreConfig struct {
	// Type is the store backend: "memory", "sqlite", or "postgres".
	Type string

	// DSN is the data source name for sqlite/postgres.
	// SQLite example: "file:webex-mcp.db" or "/path/to/data.db"
	// Postgres example: "postgres://user:pass@host:5432/dbname?sslmode=disable"
	DSN string

	// CleanupInterval is how often expired entries are purged.
	CleanupInterval time.Duration
}

// NewStore creates a Store based on the given configuration.
func NewStore(cfg StoreConfig) (Store, error) {
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = 1 * time.Minute
	}

	switch cfg.Type {
	case "", "memory":
		return NewMemoryStore(cfg.CleanupInterval), nil
	case "sqlite":
		if cfg.DSN == "" {
			log.Printf("[Store] Using SQLite store at default path: %s", DefaultSQLitePath())
		} else {
			log.Printf("[Store] Using SQLite store at: %s", cfg.DSN)
		}
		return NewSQLiteStore(cfg.DSN, cfg.CleanupInterval)
	case "postgres":
		log.Printf("[Store] Using PostgreSQL store")
		return NewPostgresStore(cfg.DSN, cfg.CleanupInterval)
	default:
		return nil, fmt.Errorf("unknown store type %q: must be 'memory', 'sqlite', or 'postgres'", cfg.Type)
	}
}

// prepareClientRegistration builds a RegisteredClient from a RegistrationRequest,
// generating a client_id, optional client_secret, and applying defaults.
// This is shared logic used by all Store implementations.
func prepareClientRegistration(req *RegistrationRequest) (*RegisteredClient, error) {
	clientID, err := generateSecureToken(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate client_id: %w", err)
	}

	var clientSecret string
	authMethod := req.TokenEndpointAuthMethod
	if authMethod == "" {
		authMethod = "none"
	}
	if authMethod == "client_secret_post" || authMethod == "client_secret_basic" {
		clientSecret, err = generateSecureToken(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate client_secret: %w", err)
		}
	}

	grantTypes := req.GrantTypes
	if len(grantTypes) == 0 {
		grantTypes = []string{"authorization_code"}
	}
	responseTypes := req.ResponseTypes
	if len(responseTypes) == 0 {
		responseTypes = []string{"code"}
	}

	return &RegisteredClient{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		RedirectURIs:            req.RedirectURIs,
		ClientName:              req.ClientName,
		TokenEndpointAuthMethod: authMethod,
		GrantTypes:              grantTypes,
		ResponseTypes:           responseTypes,
		CreatedAt:               time.Now(),
	}, nil
}

// matchesRedirectURI checks if target is present in the uris slice.
func matchesRedirectURI(uris []string, target string) bool {
	for _, uri := range uris {
		if uri == target {
			return true
		}
	}
	return false
}

// marshalClientJSON returns the JSON-encoded redirect URIs, grant types, and response types for DB storage.
func marshalClientJSON(client *RegisteredClient) (redirectURIs, grantTypes, responseTypes string) {
	r, _ := json.Marshal(client.RedirectURIs)
	g, _ := json.Marshal(client.GrantTypes)
	rt, _ := json.Marshal(client.ResponseTypes)
	return string(r), string(g), string(rt)
}

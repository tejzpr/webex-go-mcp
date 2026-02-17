package auth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore implements Store using a SQLite database.
type SQLiteStore struct {
	db          *sql.DB
	stopCleanup chan struct{}
}

// DefaultSQLitePath returns the default SQLite database path: ~/mcps/webex-go-mcp/store.db
func DefaultSQLitePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, "mcps", "webex-go-mcp", "store.db")
}

// NewSQLiteStore creates a new SQLite-backed store.
// dsn examples: "/path/to/store.db", ":memory:"
// If dsn is empty, defaults to ~/mcps/webex-go-mcp/store.db
func NewSQLiteStore(dsn string, cleanupInterval time.Duration) (*SQLiteStore, error) {
	if dsn == "" {
		dsn = DefaultSQLitePath()
	}

	// Ensure the parent directory exists
	if dsn != ":memory:" {
		dir := filepath.Dir(dsn)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create store directory %s: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	if err := createSQLiteTables(db); err != nil {
		db.Close()
		return nil, err
	}

	s := &SQLiteStore{
		db:          db,
		stopCleanup: make(chan struct{}),
	}
	go s.cleanup(cleanupInterval)
	return s, nil
}

func createSQLiteTables(db *sql.DB) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS tokens (
			opaque_token TEXT PRIMARY KEY,
			webex_access_token TEXT NOT NULL,
			webex_refresh_token TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			user_id TEXT,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS auth_codes (
			code TEXT PRIMARY KEY,
			client_id TEXT NOT NULL,
			redirect_uri TEXT NOT NULL,
			code_verifier TEXT,
			webex_access_token TEXT NOT NULL,
			webex_refresh_token TEXT NOT NULL,
			webex_expires_in INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS pending_auths (
			state TEXT PRIMARY KEY,
			client_id TEXT NOT NULL,
			client_redirect_uri TEXT NOT NULL,
			client_state TEXT,
			code_challenge TEXT,
			code_challenge_method TEXT,
			webex_code_verifier TEXT NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS clients (
			client_id TEXT PRIMARY KEY,
			client_secret TEXT,
			redirect_uris TEXT NOT NULL,
			client_name TEXT,
			token_endpoint_auth_method TEXT,
			grant_types TEXT NOT NULL,
			response_types TEXT NOT NULL,
			created_at DATETIME NOT NULL
		)`,
	}

	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}
	return nil
}

// --- Token records ---

func (s *SQLiteStore) StoreToken(webexAccessToken, webexRefreshToken string, expiresIn int) (string, error) {
	opaque, err := generateSecureToken(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate opaque token: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(expiresIn) * time.Second)

	_, err = s.db.Exec(
		`INSERT INTO tokens (opaque_token, webex_access_token, webex_refresh_token, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		opaque, webexAccessToken, webexRefreshToken, expiresAt, now,
	)
	if err != nil {
		return "", fmt.Errorf("failed to store token: %w", err)
	}
	return opaque, nil
}

func (s *SQLiteStore) LookupToken(opaqueToken string) (*TokenRecord, bool) {
	row := s.db.QueryRow(
		`SELECT opaque_token, webex_access_token, webex_refresh_token, expires_at, user_id, created_at
		 FROM tokens WHERE opaque_token = ?`, opaqueToken,
	)

	var r TokenRecord
	var userID sql.NullString
	if err := row.Scan(&r.OpaqueToken, &r.WebexAccessToken, &r.WebexRefreshToken, &r.ExpiresAt, &userID, &r.CreatedAt); err != nil {
		return nil, false
	}
	if userID.Valid {
		r.UserID = userID.String
	}
	return &r, true
}

func (s *SQLiteStore) UpdateWebexToken(opaqueToken, newAccessToken, newRefreshToken string, expiresIn int) {
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
	s.db.Exec(
		`UPDATE tokens SET webex_access_token = ?, webex_refresh_token = ?, expires_at = ? WHERE opaque_token = ?`,
		newAccessToken, newRefreshToken, expiresAt, opaqueToken,
	)
}

func (s *SQLiteStore) RevokeToken(opaqueToken string) {
	s.db.Exec(`DELETE FROM tokens WHERE opaque_token = ?`, opaqueToken)
}

// --- Authorization codes ---

func (s *SQLiteStore) StoreAuthCode(record *AuthCodeRecord) {
	s.db.Exec(
		`INSERT INTO auth_codes (code, client_id, redirect_uri, code_verifier, webex_access_token, webex_refresh_token, webex_expires_in, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.Code, record.ClientID, record.RedirectURI, record.CodeVerifier,
		record.WebexAccessToken, record.WebexRefreshToken, record.WebexExpiresIn,
		record.CreatedAt, record.ExpiresAt,
	)
}

func (s *SQLiteStore) ConsumeAuthCode(code string) (*AuthCodeRecord, bool) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, false
	}
	defer tx.Rollback()

	row := tx.QueryRow(
		`SELECT code, client_id, redirect_uri, code_verifier, webex_access_token, webex_refresh_token, webex_expires_in, created_at, expires_at
		 FROM auth_codes WHERE code = ?`, code,
	)

	var r AuthCodeRecord
	if err := row.Scan(&r.Code, &r.ClientID, &r.RedirectURI, &r.CodeVerifier,
		&r.WebexAccessToken, &r.WebexRefreshToken, &r.WebexExpiresIn,
		&r.CreatedAt, &r.ExpiresAt); err != nil {
		return nil, false
	}

	tx.Exec(`DELETE FROM auth_codes WHERE code = ?`, code)
	tx.Commit()

	if time.Now().After(r.ExpiresAt) {
		return nil, false
	}
	return &r, true
}

// --- Pending auth state ---

func (s *SQLiteStore) StorePendingAuth(pending *PendingAuth) {
	s.db.Exec(
		`INSERT INTO pending_auths (state, client_id, client_redirect_uri, client_state, code_challenge, code_challenge_method, webex_code_verifier, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		pending.State, pending.ClientID, pending.ClientRedirectURI, pending.ClientState,
		pending.CodeChallenge, pending.CodeChallengeMethod, pending.WebexCodeVerifier, pending.CreatedAt,
	)
}

func (s *SQLiteStore) ConsumePendingAuth(state string) (*PendingAuth, bool) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, false
	}
	defer tx.Rollback()

	row := tx.QueryRow(
		`SELECT state, client_id, client_redirect_uri, client_state, code_challenge, code_challenge_method, webex_code_verifier, created_at
		 FROM pending_auths WHERE state = ?`, state,
	)

	var p PendingAuth
	var clientState, codeChallenge, codeChallengeMethod sql.NullString
	if err := row.Scan(&p.State, &p.ClientID, &p.ClientRedirectURI, &clientState,
		&codeChallenge, &codeChallengeMethod, &p.WebexCodeVerifier, &p.CreatedAt); err != nil {
		return nil, false
	}
	if clientState.Valid {
		p.ClientState = clientState.String
	}
	if codeChallenge.Valid {
		p.CodeChallenge = codeChallenge.String
	}
	if codeChallengeMethod.Valid {
		p.CodeChallengeMethod = codeChallengeMethod.String
	}

	tx.Exec(`DELETE FROM pending_auths WHERE state = ?`, state)
	tx.Commit()

	if time.Since(p.CreatedAt) > 10*time.Minute {
		return nil, false
	}
	return &p, true
}

// --- Client registry ---

func (s *SQLiteStore) RegisterClient(req *RegistrationRequest) (*RegisteredClient, error) {
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

	client := &RegisteredClient{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		RedirectURIs:            req.RedirectURIs,
		ClientName:              req.ClientName,
		TokenEndpointAuthMethod: authMethod,
		GrantTypes:              grantTypes,
		ResponseTypes:           responseTypes,
		CreatedAt:               time.Now(),
	}

	redirectURIsJSON, _ := json.Marshal(client.RedirectURIs)
	grantTypesJSON, _ := json.Marshal(client.GrantTypes)
	responseTypesJSON, _ := json.Marshal(client.ResponseTypes)

	_, err = s.db.Exec(
		`INSERT INTO clients (client_id, client_secret, redirect_uris, client_name, token_endpoint_auth_method, grant_types, response_types, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		client.ClientID, client.ClientSecret, string(redirectURIsJSON), client.ClientName,
		client.TokenEndpointAuthMethod, string(grantTypesJSON), string(responseTypesJSON), client.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store client: %w", err)
	}

	return client, nil
}

func (s *SQLiteStore) RegisterClientWithID(clientID, redirectURI string) {
	// Try to get existing client
	existing, ok := s.LookupClient(clientID)
	if ok {
		for _, uri := range existing.RedirectURIs {
			if uri == redirectURI {
				return
			}
		}
		existing.RedirectURIs = append(existing.RedirectURIs, redirectURI)
		redirectURIsJSON, _ := json.Marshal(existing.RedirectURIs)
		s.db.Exec(`UPDATE clients SET redirect_uris = ? WHERE client_id = ?`, string(redirectURIsJSON), clientID)
		return
	}

	redirectURIs, _ := json.Marshal([]string{redirectURI})
	grantTypes, _ := json.Marshal([]string{"authorization_code"})
	responseTypes, _ := json.Marshal([]string{"code"})

	s.db.Exec(
		`INSERT OR IGNORE INTO clients (client_id, redirect_uris, token_endpoint_auth_method, grant_types, response_types, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		clientID, string(redirectURIs), "none", string(grantTypes), string(responseTypes), time.Now(),
	)
}

func (s *SQLiteStore) LookupClient(clientID string) (*RegisteredClient, bool) {
	row := s.db.QueryRow(
		`SELECT client_id, client_secret, redirect_uris, client_name, token_endpoint_auth_method, grant_types, response_types, created_at
		 FROM clients WHERE client_id = ?`, clientID,
	)

	var c RegisteredClient
	var clientSecret, clientName sql.NullString
	var redirectURIsJSON, grantTypesJSON, responseTypesJSON string

	if err := row.Scan(&c.ClientID, &clientSecret, &redirectURIsJSON, &clientName,
		&c.TokenEndpointAuthMethod, &grantTypesJSON, &responseTypesJSON, &c.CreatedAt); err != nil {
		return nil, false
	}

	if clientSecret.Valid {
		c.ClientSecret = clientSecret.String
	}
	if clientName.Valid {
		c.ClientName = clientName.String
	}
	json.Unmarshal([]byte(redirectURIsJSON), &c.RedirectURIs)
	json.Unmarshal([]byte(grantTypesJSON), &c.GrantTypes)
	json.Unmarshal([]byte(responseTypesJSON), &c.ResponseTypes)

	return &c, true
}

func (s *SQLiteStore) ValidateRedirectURI(clientID, redirectURI string) bool {
	client, ok := s.LookupClient(clientID)
	if !ok {
		return false
	}
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			return true
		}
	}
	return false
}

// --- Lifecycle ---

func (s *SQLiteStore) Close() error {
	close(s.stopCleanup)
	return s.db.Close()
}

func (s *SQLiteStore) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCleanup:
			return
		case <-ticker.C:
			now := time.Now()
			s.db.Exec(`DELETE FROM auth_codes WHERE expires_at < ?`, now)
			s.db.Exec(`DELETE FROM pending_auths WHERE created_at < ?`, now.Add(-10*time.Minute))
		}
	}
}

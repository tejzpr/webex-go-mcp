package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LocalToken is a Webex OAuth token set persisted on the local machine for
// STDIO mode. It is keyed to the Webex Integration client ID that minted it.
type LocalToken struct {
	ClientID              string    `json:"client_id"`
	Scopes                string    `json:"scopes,omitempty"`
	AccessToken           string    `json:"access_token"`
	RefreshToken          string    `json:"refresh_token,omitempty"`
	ExpiresAt             time.Time `json:"expires_at"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at,omitempty"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// accessTokenValid reports whether the access token is usable for at least `skew` more.
func (t *LocalToken) accessTokenValid(now time.Time, skew time.Duration) bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	if t.ExpiresAt.IsZero() {
		return true
	}
	return now.Add(skew).Before(t.ExpiresAt)
}

// canRefresh reports whether the refresh token exists and has not expired.
func (t *LocalToken) canRefresh(now time.Time) bool {
	if t == nil || t.RefreshToken == "" {
		return false
	}
	return t.RefreshTokenExpiresAt.IsZero() || now.Before(t.RefreshTokenExpiresAt)
}

// localTokenFromResponse converts a Webex token response into a LocalToken.
// If the response omits a refresh token, the previous one is retained.
func localTokenFromResponse(clientID, scopes string, resp *WebexTokenResponse, previous *LocalToken, now time.Time) *LocalToken {
	tok := &LocalToken{
		ClientID:     clientID,
		Scopes:       scopes,
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		UpdatedAt:    now,
	}
	if resp.ExpiresIn > 0 {
		tok.ExpiresAt = now.Add(time.Duration(resp.ExpiresIn) * time.Second)
	}
	if resp.RefreshTokenExpiresIn > 0 {
		tok.RefreshTokenExpiresAt = now.Add(time.Duration(resp.RefreshTokenExpiresIn) * time.Second)
	}
	if tok.RefreshToken == "" && previous != nil {
		tok.RefreshToken = previous.RefreshToken
		tok.RefreshTokenExpiresAt = previous.RefreshTokenExpiresAt
	}
	return tok
}

// FileTokenStore persists a single LocalToken as JSON in a file readable only
// by the current user (0600, parent directory 0700).
type FileTokenStore struct {
	path string
}

// NewFileTokenStore returns a token store backed by the given file path.
func NewFileTokenStore(path string) *FileTokenStore {
	return &FileTokenStore{path: path}
}

// Path returns the file path backing this store.
func (s *FileTokenStore) Path() string {
	return s.path
}

// DefaultTokenFilePath returns the default location of the STDIO OAuth token
// file: <user config dir>/webex-go-mcp/oauth-token.json.
// (macOS: ~/Library/Application Support, Linux: $XDG_CONFIG_HOME or ~/.config,
// Windows: %AppData%.)
func DefaultTokenFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config directory: %w", err)
	}
	return filepath.Join(dir, "webex-go-mcp", "oauth-token.json"), nil
}

// Load reads the token file. It returns (nil, nil) when the file does not exist.
func (s *FileTokenStore) Load() (*LocalToken, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read token file %s: %w", s.path, err)
	}
	var tok LocalToken
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("parse token file %s: %w", s.path, err)
	}
	return &tok, nil
}

// Save atomically writes the token file with 0600 permissions.
func (s *FileTokenStore) Save(tok *LocalToken) error {
	if tok == nil {
		return fmt.Errorf("token is nil")
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create token directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".oauth-token-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp token file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("chmod temp token file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write temp token file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("sync temp token file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp token file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		cleanup()
		return fmt.Errorf("replace token file: %w", err)
	}
	return nil
}

// Delete removes the token file. Missing files are not an error.
func (s *FileTokenStore) Delete() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete token file %s: %w", s.path, err)
	}
	return nil
}

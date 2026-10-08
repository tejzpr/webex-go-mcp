package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WebexCommunity/webex-go-sdk/v2/webexsdk"
)

func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func newTestLocalManager(t *testing.T, mutate func(*LocalOAuthConfig)) *LocalOAuthManager {
	t.Helper()
	cfg := LocalOAuthConfig{
		OAuth: &OAuthConfig{
			ClientID:     "client-123",
			ClientSecret: "secret-xyz",
			Scopes:       "spark:all",
			RedirectURI:  fmt.Sprintf("http://127.0.0.1:%d/callback", freeLoopbackPort(t)),
		},
		TokenFile: filepath.Join(t.TempDir(), "sub", "oauth-token.json"),
		SDKConfig: &webexsdk.Config{BaseURL: "https://example.com"},
		LoginWait: -1,
		LoginTTL:  time.Minute,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	m, err := NewLocalOAuthManager(cfg)
	if err != nil {
		t.Fatalf("NewLocalOAuthManager() error = %v", err)
	}
	m.openBrowser = func(string) error { return nil }
	m.exchange = func(code, verifier string) (*WebexTokenResponse, error) {
		return nil, errors.New("exchange not expected")
	}
	m.refresh = func(string) (*WebexTokenResponse, error) {
		return nil, errors.New("refresh not expected")
	}
	t.Cleanup(m.Close)
	return m
}

func TestValidateLoopbackRedirectURI(t *testing.T) {
	tests := []struct {
		uri     string
		wantErr bool
	}{
		{"http://localhost:8765/callback", false},
		{"http://127.0.0.1:9000/cb", false},
		{"http://[::1]:9000/cb", false},
		{"http://localhost:8765", false},
		{"https://localhost:8765/callback", true},
		{"http://example.com:8765/callback", true},
		{"http://localhost/callback", true},
		{"::not a url", true},
	}
	for _, tt := range tests {
		_, err := ValidateLoopbackRedirectURI(tt.uri)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateLoopbackRedirectURI(%q) error = %v, wantErr %v", tt.uri, err, tt.wantErr)
		}
	}
}

func TestNewLocalOAuthManagerDefaults(t *testing.T) {
	m, err := NewLocalOAuthManager(LocalOAuthConfig{
		OAuth:     &OAuthConfig{ClientID: "id", ClientSecret: "secret"},
		TokenFile: filepath.Join(t.TempDir(), "t.json"),
	})
	if err != nil {
		t.Fatalf("NewLocalOAuthManager() error = %v", err)
	}
	if m.RedirectURI() != DefaultLocalRedirectURI {
		t.Errorf("RedirectURI = %q, want %q", m.RedirectURI(), DefaultLocalRedirectURI)
	}
	if m.cfg.OAuth.Scopes != "spark:all" {
		t.Errorf("Scopes = %q, want spark:all", m.cfg.OAuth.Scopes)
	}
	if m.cfg.LoginWait != defaultLocalLoginWait {
		t.Errorf("LoginWait = %v, want %v", m.cfg.LoginWait, defaultLocalLoginWait)
	}

	if _, err := NewLocalOAuthManager(LocalOAuthConfig{OAuth: &OAuthConfig{ClientID: "id"}}); err == nil {
		t.Error("expected error when client secret is missing")
	}
}

func TestFileTokenStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "token.json")
	store := NewFileTokenStore(path)

	got, err := store.Load()
	if err != nil || got != nil {
		t.Fatalf("Load() on missing file = %v, %v; want nil, nil", got, err)
	}

	want := &LocalToken{
		ClientID:     "c",
		AccessToken:  "a",
		RefreshToken: "r",
		ExpiresAt:    time.Now().Add(time.Hour).UTC().Truncate(time.Second),
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("token file perm = %o, want 600", perm)
		}
	}

	got, err = store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.AccessToken != "a" || got.RefreshToken != "r" || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := store.Delete(); err != nil {
		t.Fatalf("second Delete() error = %v", err)
	}
}

func TestLocalTokenFromResponseKeepsPreviousRefreshToken(t *testing.T) {
	now := time.Now()
	prev := &LocalToken{RefreshToken: "old-refresh", RefreshTokenExpiresAt: now.Add(24 * time.Hour)}
	tok := localTokenFromResponse("c", "s", &WebexTokenResponse{AccessToken: "new", ExpiresIn: 3600}, prev, now)
	if tok.RefreshToken != "old-refresh" {
		t.Errorf("RefreshToken = %q, want old-refresh", tok.RefreshToken)
	}
	if !tok.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Errorf("ExpiresAt = %v, want %v", tok.ExpiresAt, now.Add(time.Hour))
	}
}

func TestLocalOAuthManagerUsesStoredToken(t *testing.T) {
	m := newTestLocalManager(t, nil)
	if err := m.store.Save(&LocalToken{
		ClientID:    "client-123",
		AccessToken: "stored-access",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	tok, err := m.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken() error = %v", err)
	}
	if tok != "stored-access" {
		t.Errorf("AccessToken() = %q, want stored-access", tok)
	}
	if !m.Status().Authenticated {
		t.Error("Status().Authenticated = false, want true")
	}

	client, err := m.Resolver()(context.Background())
	if err != nil || client == nil {
		t.Fatalf("Resolver() = %v, %v", client, err)
	}
	client2, _ := m.Resolver()(context.Background())
	if client != client2 {
		t.Error("expected resolver to reuse cached client for the same token")
	}
}

func TestLocalOAuthManagerIgnoresTokenForOtherClient(t *testing.T) {
	m := newTestLocalManager(t, nil)
	if err := m.store.Save(&LocalToken{
		ClientID:    "someone-else",
		AccessToken: "foreign",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	// Status() reloads the token file from disk.
	if m.Status().Authenticated {
		t.Error("token issued for a different client ID must be ignored")
	}
}

func TestLocalOAuthManagerRefreshesExpiredToken(t *testing.T) {
	m := newTestLocalManager(t, nil)
	if err := m.store.Save(&LocalToken{
		ClientID:     "client-123",
		AccessToken:  "expired",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	var calls int32
	m.refresh = func(rt string) (*WebexTokenResponse, error) {
		atomic.AddInt32(&calls, 1)
		if rt != "refresh-1" {
			t.Errorf("refresh token = %q, want refresh-1", rt)
		}
		return &WebexTokenResponse{AccessToken: "fresh", ExpiresIn: 3600, RefreshToken: "refresh-2"}, nil
	}

	tok, err := m.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken() error = %v", err)
	}
	if tok != "fresh" {
		t.Errorf("AccessToken() = %q, want fresh", tok)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("refresh calls = %d, want 1", calls)
	}

	saved, err := m.store.Load()
	if err != nil || saved == nil {
		t.Fatalf("Load() = %v, %v", saved, err)
	}
	if saved.AccessToken != "fresh" || saved.RefreshToken != "refresh-2" {
		t.Errorf("persisted token = %+v, want fresh/refresh-2", saved)
	}

	// Second call uses the cached token without refreshing.
	if _, err := m.AccessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("refresh calls after cached use = %d, want 1", calls)
	}
}

func noRedirectClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestLocalOAuthManagerLoginRequiredReturnsURL(t *testing.T) {
	m := newTestLocalManager(t, nil)

	_, err := m.AccessToken(context.Background())
	var lre *LoginRequiredError
	if !errors.As(err, &lre) {
		t.Fatalf("AccessToken() error = %v, want *LoginRequiredError", err)
	}
	if !strings.HasSuffix(lre.LoginURL, localLoginLandingPath) {
		t.Errorf("LoginURL = %q, want suffix %q", lre.LoginURL, localLoginLandingPath)
	}
	if !strings.Contains(lre.Error(), lre.LoginURL) {
		t.Errorf("error message %q should contain login URL", lre.Error())
	}
	if !m.Status().LoginInProgress {
		t.Error("Status().LoginInProgress = false, want true")
	}

	// Landing page redirects to the Webex authorize URL.
	resp, err := noRedirectClient().Get(lre.LoginURL)
	if err != nil {
		t.Fatalf("GET landing: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("landing status = %d, want 302", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != lre.AuthURL {
		t.Errorf("landing Location = %q, want %q", loc, lre.AuthURL)
	}
	authURL, _ := url.Parse(lre.AuthURL)
	q := authURL.Query()
	if q.Get("client_id") != "client-123" || q.Get("code_challenge_method") != "S256" || q.Get("state") == "" {
		t.Errorf("unexpected authorize params: %v", q)
	}
	if q.Get("redirect_uri") != m.RedirectURI() {
		t.Errorf("authorize redirect_uri = %q, want %q", q.Get("redirect_uri"), m.RedirectURI())
	}

	// A second call reuses the same in-flight sign-in.
	_, err2 := m.AccessToken(context.Background())
	var lre2 *LoginRequiredError
	if !errors.As(err2, &lre2) || lre2.AuthURL != lre.AuthURL {
		t.Errorf("expected the same in-flight sign-in to be reused")
	}
}

// simulateBrowser follows the authorize URL's state back to the loopback callback.
func simulateBrowser(redirectURI string, extra url.Values) func(string) error {
	return func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		q := url.Values{"state": {u.Query().Get("state")}}
		for k, v := range extra {
			q[k] = v
		}
		go func() {
			// Errors surface via the flow result; don't touch t from this goroutine.
			if resp, err := noRedirectClient().Get(redirectURI + "?" + q.Encode()); err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
}

func TestLocalOAuthManagerLoopbackLoginSuccess(t *testing.T) {
	m := newTestLocalManager(t, func(c *LocalOAuthConfig) {
		c.OpenBrowser = true
		c.LoginWait = 5 * time.Second
	})
	m.openBrowser = simulateBrowser(m.RedirectURI(), url.Values{"code": {"auth-code"}})

	var gotVerifier string
	m.exchange = func(code, verifier string) (*WebexTokenResponse, error) {
		if code != "auth-code" {
			t.Errorf("exchange code = %q, want auth-code", code)
		}
		gotVerifier = verifier
		return &WebexTokenResponse{AccessToken: "user-access", ExpiresIn: 3600, RefreshToken: "user-refresh", RefreshTokenExpiresIn: 7200}, nil
	}

	tok, err := m.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken() error = %v", err)
	}
	if tok != "user-access" {
		t.Errorf("AccessToken() = %q, want user-access", tok)
	}
	if gotVerifier == "" {
		t.Error("expected PKCE verifier to be passed to the token exchange")
	}

	saved, err := m.store.Load()
	if err != nil || saved == nil || saved.AccessToken != "user-access" || saved.ClientID != "client-123" {
		t.Fatalf("persisted token = %+v, err %v", saved, err)
	}
	if st := m.Status(); !st.Authenticated || st.LoginInProgress {
		t.Errorf("Status() = %+v, want authenticated and no login in progress", st)
	}
}

func TestLocalOAuthManagerLoopbackLoginDenied(t *testing.T) {
	m := newTestLocalManager(t, func(c *LocalOAuthConfig) {
		c.OpenBrowser = true
		c.LoginWait = 5 * time.Second
	})
	m.openBrowser = simulateBrowser(m.RedirectURI(), url.Values{"error": {"access_denied"}})

	_, err := m.AccessToken(context.Background())
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("AccessToken() error = %v, want access_denied", err)
	}
	if m.Status().LoginInProgress {
		t.Error("denied sign-in should end the flow")
	}
}

func TestLocalOAuthManagerCallbackRejectsWrongState(t *testing.T) {
	m := newTestLocalManager(t, nil)
	if _, _, err := m.StartLogin(); err != nil {
		t.Fatal(err)
	}

	resp, err := noRedirectClient().Get(m.RedirectURI() + "?code=x&state=wrong")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if !m.Status().LoginInProgress {
		t.Error("a mismatched state must not end the in-flight sign-in")
	}
}

func TestLocalOAuthManagerLogout(t *testing.T) {
	m := newTestLocalManager(t, nil)
	if err := m.store.Save(&LocalToken{ClientID: "client-123", AccessToken: "a", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if !m.Status().Authenticated {
		t.Fatal("expected authenticated before logout")
	}
	if err := m.Logout(); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if m.Status().Authenticated {
		t.Error("expected unauthenticated after logout")
	}
	if _, err := os.Stat(m.TokenFile()); !os.IsNotExist(err) {
		t.Errorf("token file should be removed, stat err = %v", err)
	}
}

func TestExchangeAndRefreshWebexToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if r.Form.Get("client_id") != "id" || r.Form.Get("client_secret") != "secret" {
			t.Errorf("missing client credentials: %v", r.Form)
		}
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "c" || r.Form.Get("code_verifier") != "v" || r.Form.Get("redirect_uri") != "http://localhost:1/cb" {
				t.Errorf("unexpected exchange form: %v", r.Form)
			}
			fmt.Fprint(w, `{"access_token":"a1","expires_in":10,"refresh_token":"r1"}`)
		case "refresh_token":
			if r.Form.Get("refresh_token") != "r1" {
				t.Errorf("unexpected refresh form: %v", r.Form)
			}
			fmt.Fprint(w, `{"access_token":"a2","expires_in":10}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	orig := webexAccessTokenURL
	webexAccessTokenURL = srv.URL
	defer func() { webexAccessTokenURL = orig }()

	cfg := &OAuthConfig{ClientID: "id", ClientSecret: "secret", RedirectURI: "http://localhost:1/cb"}
	tok, err := ExchangeWebexCode(cfg, "c", "v")
	if err != nil || tok.AccessToken != "a1" || tok.RefreshToken != "r1" {
		t.Fatalf("ExchangeWebexCode() = %+v, %v", tok, err)
	}
	tok, err = RefreshWebexToken(cfg, "r1")
	if err != nil || tok.AccessToken != "a2" {
		t.Fatalf("RefreshWebexToken() = %+v, %v", tok, err)
	}
}

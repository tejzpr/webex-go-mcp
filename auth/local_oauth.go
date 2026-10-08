package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	webex "github.com/WebexCommunity/webex-go-sdk/v2"
	"github.com/WebexCommunity/webex-go-sdk/v2/webexsdk"
)

// Local (STDIO-mode) OAuth for Webex Integrations.
//
// STDIO MCP servers own stdin/stdout, so they cannot prompt the user directly.
// Instead this follows the native-app OAuth pattern (RFC 8252): a short-lived
// HTTP listener is started on the loopback redirect URI registered with the
// Webex Integration, the user's browser is opened to the Webex consent page
// (authorization code + PKCE), and the resulting tokens are persisted to a
// per-user token file and refreshed automatically.

const (
	// DefaultLocalRedirectURI is the redirect URI used in STDIO OAuth mode when
	// none is configured. It must be registered on the Webex Integration.
	DefaultLocalRedirectURI = "http://localhost:8765/callback"

	localTokenRefreshSkew   = 5 * time.Minute
	defaultLocalLoginTTL    = 10 * time.Minute
	defaultLocalLoginWait   = 60 * time.Second
	localLoginLandingPath   = "/login"
	localLoginServerTimeout = 10 * time.Second
)

var (
	errLoginWaitTimeout = errors.New("timed out waiting for Webex sign-in")
	errLoginExpired     = errors.New("the Webex sign-in link expired before it was completed")
	errLoginCancelled   = errors.New("the Webex sign-in was cancelled")
	errNoLocalToken     = errors.New("no Webex OAuth token is stored")
)

// LocalOAuthConfig configures STDIO-mode Webex Integration authentication.
type LocalOAuthConfig struct {
	// OAuth holds the Webex Integration client ID/secret, scopes and the
	// loopback redirect URI. ServerURL is unused.
	OAuth *OAuthConfig
	// TokenFile is where tokens are persisted. Defaults to DefaultTokenFilePath().
	TokenFile string
	// SDKConfig is used to build Webex clients from the access token.
	SDKConfig *webexsdk.Config
	// OpenBrowser opens the user's browser automatically when sign-in starts.
	OpenBrowser bool
	// LoginWait is how long a tool call blocks waiting for an interactive
	// sign-in to complete before returning a "sign-in required" error.
	// Zero means use the default (60s); negative means don't wait.
	LoginWait time.Duration
	// LoginTTL is how long the loopback listener stays up waiting for the
	// browser redirect. Defaults to 10 minutes.
	LoginTTL time.Duration
}

// LoginRequiredError is returned by the local resolver when no usable token is
// available and the user has to complete a browser sign-in.
type LoginRequiredError struct {
	LoginURL string
	AuthURL  string
	Cause    error
}

func (e *LoginRequiredError) Error() string {
	var b strings.Builder
	b.WriteString("Webex sign-in required.")
	if e.LoginURL != "" {
		fmt.Fprintf(&b, " Open %s in a browser (a browser window may already have opened) and sign in with your Webex account, then retry this request.", e.LoginURL)
	} else {
		b.WriteString(" Run `webex-go-mcp login` or call the webex_auth_login tool, then retry this request.")
	}
	if e.Cause != nil && !errors.Is(e.Cause, errNoLocalToken) {
		fmt.Fprintf(&b, " (reason: %v)", e.Cause)
	}
	return b.String()
}

func (e *LoginRequiredError) Unwrap() error { return e.Cause }

// LocalAuthStatus describes the current STDIO OAuth state.
type LocalAuthStatus struct {
	Authenticated         bool       `json:"authenticated"`
	ClientID              string     `json:"clientId"`
	Scopes                string     `json:"scopes,omitempty"`
	RedirectURI           string     `json:"redirectUri"`
	TokenFile             string     `json:"tokenFile"`
	AccessTokenExpiresAt  *time.Time `json:"accessTokenExpiresAt,omitempty"`
	RefreshTokenExpiresAt *time.Time `json:"refreshTokenExpiresAt,omitempty"`
	CanRefresh            bool       `json:"canRefresh"`
	LoginInProgress       bool       `json:"loginInProgress"`
	LoginURL              string     `json:"loginUrl,omitempty"`
}

// LocalOAuthManager owns the STDIO-mode Webex OAuth token lifecycle.
type LocalOAuthManager struct {
	cfg         LocalOAuthConfig
	redirectURL *url.URL
	store       *FileTokenStore

	mu        sync.Mutex
	token     *LocalToken
	client    *webex.WebexClient
	clientTok string
	flow      *loginFlow

	// Hooks (overridable in tests).
	now         func() time.Time
	openBrowser func(string) error
	exchange    func(code, verifier string) (*WebexTokenResponse, error)
	refresh     func(refreshToken string) (*WebexTokenResponse, error)
	newClient   func(token string) (*webex.WebexClient, error)
}

type loginFlow struct {
	state    string
	verifier string
	authURL  string
	localURL string
	done     chan struct{}

	// Guarded by LocalOAuthManager.mu.
	err        error
	finished   bool
	exchanging bool
	servers    []*http.Server
	timer      *time.Timer
}

// ValidateLoopbackRedirectURI checks that a redirect URI is suitable for the
// local loopback listener: http scheme, loopback host, explicit port.
func ValidateLoopbackRedirectURI(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid redirect URI %q: %w", raw, err)
	}
	if u.Scheme != "http" {
		return nil, fmt.Errorf("redirect URI %q must use http:// (the local listener does not serve TLS)", raw)
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
	default:
		return nil, fmt.Errorf("redirect URI %q must point at localhost, 127.0.0.1 or [::1] in stdio mode", raw)
	}
	if u.Port() == "" {
		return nil, fmt.Errorf("redirect URI %q must include an explicit port (example: %s)", raw, DefaultLocalRedirectURI)
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u, nil
}

// NewLocalOAuthManager validates config and loads any persisted token.
func NewLocalOAuthManager(cfg LocalOAuthConfig) (*LocalOAuthManager, error) {
	if cfg.OAuth == nil {
		return nil, fmt.Errorf("OAuth config is required")
	}
	if cfg.OAuth.ClientID == "" || cfg.OAuth.ClientSecret == "" {
		return nil, fmt.Errorf("Webex Integration client ID and client secret are required")
	}
	oauthCfg := *cfg.OAuth
	if oauthCfg.RedirectURI == "" {
		oauthCfg.RedirectURI = DefaultLocalRedirectURI
	}
	if oauthCfg.Scopes == "" {
		oauthCfg.Scopes = "spark:all"
	}
	cfg.OAuth = &oauthCfg

	redirectURL, err := ValidateLoopbackRedirectURI(oauthCfg.RedirectURI)
	if err != nil {
		return nil, err
	}

	if cfg.TokenFile == "" {
		path, err := DefaultTokenFilePath()
		if err != nil {
			return nil, err
		}
		cfg.TokenFile = path
	}
	if cfg.LoginTTL <= 0 {
		cfg.LoginTTL = defaultLocalLoginTTL
	}
	if cfg.LoginWait == 0 {
		cfg.LoginWait = defaultLocalLoginWait
	}

	m := &LocalOAuthManager{
		cfg:         cfg,
		redirectURL: redirectURL,
		store:       NewFileTokenStore(cfg.TokenFile),
		now:         time.Now,
		openBrowser: OpenBrowser,
	}
	m.exchange = func(code, verifier string) (*WebexTokenResponse, error) {
		return ExchangeWebexCode(m.cfg.OAuth, code, verifier)
	}
	m.refresh = func(refreshToken string) (*WebexTokenResponse, error) {
		return RefreshWebexToken(m.cfg.OAuth, refreshToken)
	}
	m.newClient = func(token string) (*webex.WebexClient, error) {
		return webex.NewClient(token, m.cfg.SDKConfig)
	}

	m.mu.Lock()
	m.reloadFromDiskLocked()
	m.mu.Unlock()
	return m, nil
}

// TokenFile returns the path of the token file.
func (m *LocalOAuthManager) TokenFile() string { return m.store.Path() }

// RedirectURI returns the effective redirect URI.
func (m *LocalOAuthManager) RedirectURI() string { return m.cfg.OAuth.RedirectURI }

// HasUsableToken reports whether a valid or refreshable token is loaded,
// without performing any network calls.
func (m *LocalOAuthManager) HasUsableToken() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	return m.token.accessTokenValid(now, 0) || m.token.canRefresh(now)
}

// Resolver returns a ClientResolver backed by the local OAuth token. When no
// token is available it starts a browser sign-in and waits up to LoginWait.
func (m *LocalOAuthManager) Resolver() ClientResolver {
	return func(ctx context.Context) (*webex.WebexClient, error) {
		token, err := m.AccessToken(ctx)
		if err != nil {
			return nil, err
		}
		return m.clientFor(token)
	}
}

// ClientIfAuthenticated returns a Webex client for the stored token (refreshing
// it if needed) without ever starting an interactive sign-in.
func (m *LocalOAuthManager) ClientIfAuthenticated() (*webex.WebexClient, error) {
	m.mu.Lock()
	token, err := m.ensureTokenLocked()
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return m.clientFor(token)
}

func (m *LocalOAuthManager) clientFor(token string) (*webex.WebexClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client != nil && m.clientTok == token {
		return m.client, nil
	}
	client, err := m.newClient(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Webex client: %w", err)
	}
	m.client = client
	m.clientTok = token
	return client, nil
}

// AccessToken returns a valid Webex access token, refreshing it if needed.
// If the user must sign in, it starts the loopback flow (opening the browser
// when configured), waits up to LoginWait, and otherwise returns a
// *LoginRequiredError carrying the sign-in URL.
func (m *LocalOAuthManager) AccessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	token, tokenErr := m.ensureTokenLocked()
	if tokenErr == nil {
		m.mu.Unlock()
		return token, nil
	}
	flow, startErr := m.startLoginLocked()
	m.mu.Unlock()

	if startErr != nil {
		return "", fmt.Errorf("Webex sign-in required, but the local sign-in listener could not start: %w", startErr)
	}

	if m.cfg.LoginWait > 0 {
		waitErr := waitForFlow(ctx, flow, m.cfg.LoginWait)
		switch {
		case waitErr == nil:
			m.mu.Lock()
			token, tokenErr = m.ensureTokenLocked()
			m.mu.Unlock()
			if tokenErr == nil {
				return token, nil
			}
		case errors.Is(waitErr, errLoginWaitTimeout), ctx.Err() != nil:
			// Still pending; fall through and hand back the sign-in URL.
		default:
			return "", fmt.Errorf("Webex sign-in did not complete: %v. Retry the request to start a new sign-in", waitErr)
		}
	}

	return "", &LoginRequiredError{LoginURL: flow.localURL, AuthURL: flow.authURL, Cause: tokenErr}
}

// ensureTokenLocked returns a usable access token from memory/disk, refreshing
// if necessary. Caller must hold m.mu.
func (m *LocalOAuthManager) ensureTokenLocked() (string, error) {
	now := m.now()
	if m.token.accessTokenValid(now, localTokenRefreshSkew) {
		return m.token.AccessToken, nil
	}

	// Another process (e.g. `webex-go-mcp login`, or a sibling MCP server) may
	// have written a newer token.
	m.reloadFromDiskLocked()
	if m.token.accessTokenValid(now, localTokenRefreshSkew) {
		return m.token.AccessToken, nil
	}

	var refreshErr error
	if m.token.canRefresh(now) {
		resp, err := m.refresh(m.token.RefreshToken)
		if err == nil {
			tok := localTokenFromResponse(m.cfg.OAuth.ClientID, m.cfg.OAuth.Scopes, resp, m.token, now)
			m.token = tok
			if err := m.store.Save(tok); err != nil {
				log.Printf("[LocalOAuth] refreshed token but failed to persist it: %v", err)
			} else {
				log.Printf("[LocalOAuth] refreshed Webex access token (expires %s)", tok.ExpiresAt.Format(time.RFC3339))
			}
			return tok.AccessToken, nil
		}
		refreshErr = fmt.Errorf("token refresh failed: %w", err)
		log.Printf("[LocalOAuth] %v", refreshErr)
	}

	// Refresh unavailable/failed but the access token hasn't actually expired yet.
	if m.token.accessTokenValid(now, 0) {
		return m.token.AccessToken, nil
	}

	if refreshErr != nil {
		return "", refreshErr
	}
	if m.token != nil {
		return "", errors.New("the stored Webex token has expired")
	}
	return "", errNoLocalToken
}

// reloadFromDiskLocked adopts the token file contents if they belong to this
// client ID and differ from what's in memory. Caller must hold m.mu.
func (m *LocalOAuthManager) reloadFromDiskLocked() {
	tok, err := m.store.Load()
	if err != nil {
		log.Printf("[LocalOAuth] ignoring unreadable token file: %v", err)
		return
	}
	if tok == nil {
		return
	}
	if tok.ClientID != m.cfg.OAuth.ClientID {
		log.Printf("[LocalOAuth] ignoring token file %s: it was issued for a different Webex Integration client ID", m.store.Path())
		return
	}
	if m.token == nil || tok.AccessToken != m.token.AccessToken || tok.RefreshToken != m.token.RefreshToken {
		m.token = tok
	}
}

// StartLogin starts (or reuses) a browser sign-in flow and returns the local
// sign-in URL and the Webex authorize URL.
func (m *LocalOAuthManager) StartLogin() (loginURL, authURL string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	flow, err := m.startLoginLocked()
	if err != nil {
		return "", "", err
	}
	return flow.localURL, flow.authURL, nil
}

// WaitForLogin blocks until the in-progress sign-in finishes, the timeout
// elapses, or ctx is cancelled. It returns nil immediately when there is no
// sign-in in progress.
func (m *LocalOAuthManager) WaitForLogin(ctx context.Context, timeout time.Duration) error {
	m.mu.Lock()
	flow := m.flow
	m.mu.Unlock()
	if flow == nil {
		return nil
	}
	return waitForFlow(ctx, flow, timeout)
}

// Login runs an interactive sign-in to completion (used by `webex-go-mcp login`).
// Progress messages are written to out.
func (m *LocalOAuthManager) Login(ctx context.Context, out io.Writer) error {
	loginURL, authURL, err := m.StartLogin()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Opening your browser to sign in with Webex...\n")
	fmt.Fprintf(out, "If it does not open, visit:\n  %s\n", loginURL)
	if loginURL != authURL {
		fmt.Fprintf(out, "(or directly: %s)\n", authURL)
	}
	fmt.Fprintf(out, "Waiting for sign-in to complete (Ctrl+C to cancel)...\n")

	err = m.WaitForLogin(ctx, m.cfg.LoginTTL+time.Minute)
	if err != nil {
		if ctx.Err() != nil {
			m.Close()
		}
		return err
	}
	if !m.HasUsableToken() {
		return errors.New("sign-in finished but no token was stored")
	}
	fmt.Fprintf(out, "Signed in. Token saved to %s\n", m.store.Path())
	return nil
}

// Logout forgets the stored token (memory and disk) and cancels any sign-in in progress.
func (m *LocalOAuthManager) Logout() error {
	m.mu.Lock()
	m.token = nil
	m.client = nil
	m.clientTok = ""
	flow := m.flow
	err := m.store.Delete()
	m.mu.Unlock()
	if flow != nil {
		m.finishFlow(flow, errLoginCancelled)
	}
	return err
}

// Status reports the current authentication state without network calls.
func (m *LocalOAuthManager) Status() LocalAuthStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadFromDiskLocked()
	now := m.now()
	st := LocalAuthStatus{
		ClientID:    m.cfg.OAuth.ClientID,
		Scopes:      m.cfg.OAuth.Scopes,
		RedirectURI: m.cfg.OAuth.RedirectURI,
		TokenFile:   m.store.Path(),
	}
	if m.token != nil {
		st.Authenticated = m.token.accessTokenValid(now, 0) || m.token.canRefresh(now)
		st.CanRefresh = m.token.canRefresh(now)
		if !m.token.ExpiresAt.IsZero() {
			t := m.token.ExpiresAt
			st.AccessTokenExpiresAt = &t
		}
		if !m.token.RefreshTokenExpiresAt.IsZero() {
			t := m.token.RefreshTokenExpiresAt
			st.RefreshTokenExpiresAt = &t
		}
		if m.token.Scopes != "" {
			st.Scopes = m.token.Scopes
		}
	}
	if m.flow != nil && !m.flow.finished {
		st.LoginInProgress = true
		st.LoginURL = m.flow.localURL
	}
	return st
}

// Close shuts down any in-progress sign-in listener.
func (m *LocalOAuthManager) Close() {
	m.mu.Lock()
	flow := m.flow
	m.mu.Unlock()
	if flow != nil {
		m.finishFlow(flow, errLoginCancelled)
	}
}

// startLoginLocked starts the loopback listener for a new sign-in, or returns
// the one already in progress. Caller must hold m.mu.
func (m *LocalOAuthManager) startLoginLocked() (*loginFlow, error) {
	if m.flow != nil && !m.flow.finished {
		return m.flow, nil
	}

	state, err := GenerateState()
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}
	verifier, err := generateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate PKCE verifier: %w", err)
	}

	flow := &loginFlow{
		state:    state,
		verifier: verifier,
		authURL:  BuildWebexAuthorizeURL(m.cfg.OAuth, state, generateS256Challenge(verifier)),
		done:     make(chan struct{}),
	}
	landing := *m.redirectURL
	landing.Path = localLoginLandingPath
	landing.RawQuery = ""
	landing.Fragment = ""
	flow.localURL = landing.String()

	servers, err := m.listen(flow)
	if err != nil {
		return nil, err
	}
	flow.servers = servers
	flow.timer = time.AfterFunc(m.cfg.LoginTTL, func() { m.finishFlow(flow, errLoginExpired) })
	m.flow = flow

	log.Printf("[LocalOAuth] Webex sign-in started; waiting for browser redirect on %s (sign-in page: %s)", m.cfg.OAuth.RedirectURI, flow.localURL)
	if m.cfg.OpenBrowser && m.openBrowser != nil {
		if err := m.openBrowser(flow.authURL); err != nil {
			log.Printf("[LocalOAuth] could not open a browser automatically (%v); open %s manually", err, flow.localURL)
		}
	}
	return flow, nil
}

// listen binds the loopback address(es) from the redirect URI and serves the
// sign-in handler on each.
func (m *LocalOAuthManager) listen(flow *loginFlow) ([]*http.Server, error) {
	host := m.redirectURL.Hostname()
	port := m.redirectURL.Port()

	var addrs []string
	if host == "localhost" {
		// Browsers may resolve localhost to either family; IPv4 is required, IPv6 best-effort.
		addrs = []string{net.JoinHostPort("127.0.0.1", port), net.JoinHostPort("::1", port)}
	} else {
		addrs = []string{net.JoinHostPort(host, port)}
	}

	var listeners []net.Listener
	for i, addr := range addrs {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			if i == 0 {
				return nil, fmt.Errorf("cannot listen on %s for the OAuth redirect %s (is another process using port %s?): %w", addr, m.cfg.OAuth.RedirectURI, port, err)
			}
			continue
		}
		listeners = append(listeners, l)
	}

	handler := m.flowHandler(flow)
	servers := make([]*http.Server, 0, len(listeners))
	for _, l := range listeners {
		srv := &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: localLoginServerTimeout,
		}
		servers = append(servers, srv)
		go func(srv *http.Server, l net.Listener) {
			if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("[LocalOAuth] sign-in listener on %s stopped: %v", l.Addr(), err)
			}
		}(srv, l)
	}
	return servers, nil
}

// flowHandler serves the landing redirect and the OAuth callback for one flow.
func (m *LocalOAuthManager) flowHandler(flow *loginFlow) http.Handler {
	callbackPath := m.redirectURL.Path
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		q := r.URL.Query()
		isCallback := r.URL.Path == callbackPath && (q.Has("code") || q.Has("error") || q.Has("state"))
		switch {
		case isCallback:
			m.handleCallback(w, r, flow)
		case r.URL.Path == localLoginLandingPath || r.URL.Path == "/":
			m.mu.Lock()
			finished := flow.finished
			m.mu.Unlock()
			if finished {
				renderLocalAuthPage(w, http.StatusGone, "Sign-in link expired", "This sign-in link is no longer active. Retry your request in the MCP client to start a new sign-in.", false)
				return
			}
			http.Redirect(w, r, flow.authURL, http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	})
}

func (m *LocalOAuthManager) handleCallback(w http.ResponseWriter, r *http.Request, flow *loginFlow) {
	q := r.URL.Query()

	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(flow.state)) != 1 {
		// Don't end the flow: this may be a stale tab from an earlier attempt.
		renderLocalAuthPage(w, http.StatusBadRequest, "Sign-in link mismatch", "This response does not match the sign-in in progress. Close this tab and use the most recent sign-in link.", false)
		return
	}

	if e := q.Get("error"); e != "" {
		desc := q.Get("error_description")
		renderLocalAuthPage(w, http.StatusBadRequest, "Webex sign-in failed", fmt.Sprintf("Webex returned: %s %s. You can close this tab and retry.", e, desc), false)
		m.finishFlow(flow, fmt.Errorf("Webex authorization failed: %s %s", e, desc))
		return
	}

	code := q.Get("code")
	if code == "" {
		renderLocalAuthPage(w, http.StatusBadRequest, "Missing authorization code", "Webex did not return an authorization code.", false)
		return
	}

	m.mu.Lock()
	if flow.exchanging {
		m.mu.Unlock()
		renderLocalAuthPage(w, http.StatusOK, "Already handled", "This sign-in has already been processed. You can close this tab.", true)
		return
	}
	if flow.finished {
		m.mu.Unlock()
		renderLocalAuthPage(w, http.StatusGone, "Sign-in link expired", "This sign-in is no longer active. Retry your request in the MCP client to start a new sign-in.", false)
		return
	}
	flow.exchanging = true
	m.mu.Unlock()

	resp, err := m.exchange(code, flow.verifier)
	if err != nil {
		log.Printf("[LocalOAuth] token exchange failed: %v", err)
		renderLocalAuthPage(w, http.StatusBadGateway, "Webex sign-in failed", "Could not exchange the authorization code with Webex. Check the MCP server logs, then retry.", false)
		m.finishFlow(flow, fmt.Errorf("token exchange with Webex failed: %w", err))
		return
	}

	m.mu.Lock()
	tok := localTokenFromResponse(m.cfg.OAuth.ClientID, m.cfg.OAuth.Scopes, resp, nil, m.now())
	m.token = tok
	saveErr := m.store.Save(tok)
	m.mu.Unlock()
	if saveErr != nil {
		log.Printf("[LocalOAuth] signed in, but failed to persist token (it will be kept in memory only): %v", saveErr)
	} else {
		log.Printf("[LocalOAuth] signed in to Webex; token saved to %s", m.store.Path())
	}

	// Render before waking waiters so a CLI `login` doesn't exit mid-response.
	renderLocalAuthPage(w, http.StatusOK, "Signed in to Webex", "You're all set. You can close this tab and return to your MCP client.", true)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	m.finishFlow(flow, nil)
}

// finishFlow marks the flow complete, wakes waiters, and shuts down its listeners.
func (m *LocalOAuthManager) finishFlow(flow *loginFlow, err error) {
	m.mu.Lock()
	if flow.finished {
		m.mu.Unlock()
		return
	}
	flow.finished = true
	flow.err = err
	if flow.timer != nil {
		flow.timer.Stop()
	}
	if m.flow == flow {
		m.flow = nil
	}
	servers := flow.servers
	m.mu.Unlock()

	close(flow.done)
	if err != nil {
		log.Printf("[LocalOAuth] sign-in ended: %v", err)
	}

	// Shutdown waits for in-flight responses (e.g. the success page) to finish.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, srv := range servers {
			_ = srv.Shutdown(ctx)
		}
	}()
}

func waitForFlow(ctx context.Context, flow *loginFlow, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-flow.done:
		return flow.err
	case <-timer.C:
		return errLoginWaitTimeout
	case <-ctx.Done():
		return ctx.Err()
	}
}

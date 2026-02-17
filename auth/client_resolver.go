package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/webexsdk"
)

// ClientResolver resolves a *webex.WebexClient from the request context.
// In STDIO mode it returns a static client; in HTTP mode it extracts the
// per-request client injected by the auth middleware.
type ClientResolver func(ctx context.Context) (*webex.WebexClient, error)

// contextKey is an unexported type for context keys in this package.
type contextKey int

const (
	// webexClientKey is the context key for the per-request Webex client.
	webexClientKey contextKey = iota
	// webexTokenKey is the context key for the raw Webex access token string.
	webexTokenKey
)

// ContextWithWebexClient returns a new context carrying the Webex client.
func ContextWithWebexClient(ctx context.Context, client *webex.WebexClient) context.Context {
	return context.WithValue(ctx, webexClientKey, client)
}

// WebexClientFromContext extracts the Webex client from the context.
func WebexClientFromContext(ctx context.Context) (*webex.WebexClient, bool) {
	client, ok := ctx.Value(webexClientKey).(*webex.WebexClient)
	return client, ok
}

// ContextWithWebexToken returns a new context carrying the raw Webex access token.
func ContextWithWebexToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, webexTokenKey, token)
}

// WebexTokenFromContext extracts the raw Webex access token from the context.
func WebexTokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(webexTokenKey).(string)
	return token, ok
}

// NewStaticClientResolver returns a ClientResolver that always returns the same client.
// Used in STDIO mode where a single WEBEX_ACCESS_TOKEN is shared.
func NewStaticClientResolver(client *webex.WebexClient) ClientResolver {
	return func(ctx context.Context) (*webex.WebexClient, error) {
		return client, nil
	}
}

// NewHTTPClientResolver returns a ClientResolver that extracts the Webex client
// from the request context (set by AuthMiddleware).
func NewHTTPClientResolver() ClientResolver {
	return func(ctx context.Context) (*webex.WebexClient, error) {
		client, ok := WebexClientFromContext(ctx)
		if !ok || client == nil {
			return nil, fmt.Errorf("no authenticated Webex client in context")
		}
		return client, nil
	}
}

// cachedClient holds a cached Webex client with expiry.
type cachedClient struct {
	client    *webex.WebexClient
	expiresAt time.Time
}

// ClientCache caches *webex.WebexClient instances keyed by token hash.
type ClientCache struct {
	mu      sync.RWMutex
	entries map[string]*cachedClient
	ttl     time.Duration
	config  *webexsdk.Config
}

// NewClientCache creates a new client cache with the given TTL and SDK config.
func NewClientCache(ttl time.Duration, config *webexsdk.Config) *ClientCache {
	cc := &ClientCache{
		entries: make(map[string]*cachedClient),
		ttl:     ttl,
		config:  config,
	}
	// Start background cleanup
	go cc.cleanup()
	return cc
}

// GetOrCreate returns a cached client for the given token, or creates a new one.
func (cc *ClientCache) GetOrCreate(accessToken string) (*webex.WebexClient, error) {
	key := tokenHash(accessToken)

	cc.mu.RLock()
	if entry, ok := cc.entries[key]; ok && time.Now().Before(entry.expiresAt) {
		cc.mu.RUnlock()
		return entry.client, nil
	}
	cc.mu.RUnlock()

	// Create new client
	client, err := webex.NewClient(accessToken, cc.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Webex client: %w", err)
	}

	cc.mu.Lock()
	cc.entries[key] = &cachedClient{
		client:    client,
		expiresAt: time.Now().Add(cc.ttl),
	}
	cc.mu.Unlock()

	return client, nil
}

// Evict removes the cached client for the given token.
func (cc *ClientCache) Evict(accessToken string) {
	key := tokenHash(accessToken)
	cc.mu.Lock()
	delete(cc.entries, key)
	cc.mu.Unlock()
}

// cleanup periodically removes expired entries.
func (cc *ClientCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cc.mu.Lock()
		now := time.Now()
		for k, v := range cc.entries {
			if now.After(v.expiresAt) {
				delete(cc.entries, k)
			}
		}
		cc.mu.Unlock()
	}
}

// tokenHash returns a hex-encoded SHA-256 hash of the token.
func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}

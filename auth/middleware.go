package auth

import (
	"log"
	"net/http"
	"time"
)

// AuthMiddleware validates Bearer tokens on MCP requests and injects
// the corresponding Webex client into the request context.
type AuthMiddleware struct {
	store       *TokenStore
	clientCache *ClientCache
	oauthHandler *OAuthHandler
	serverURL   string
}

// NewAuthMiddleware creates a new auth middleware.
func NewAuthMiddleware(store *TokenStore, clientCache *ClientCache, oauthHandler *OAuthHandler, serverURL string) *AuthMiddleware {
	return &AuthMiddleware{
		store:       store,
		clientCache: clientCache,
		oauthHandler: oauthHandler,
		serverURL:   serverURL,
	}
}

// Wrap returns an http.Handler that enforces Bearer token authentication.
func (am *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			am.sendUnauthorized(w)
			return
		}

		opaqueToken, ok := SplitBearerToken(authHeader)
		if !ok {
			am.sendUnauthorized(w)
			return
		}

		// Look up the opaque token
		record, ok := am.store.LookupToken(opaqueToken)
		if !ok {
			am.sendUnauthorized(w)
			return
		}

		// Check if the Webex access token is expired or near-expiry (5 min buffer)
		webexAccessToken := record.WebexAccessToken
		if time.Now().Add(5 * time.Minute).After(record.ExpiresAt) {
			// Try to refresh
			newToken, err := am.oauthHandler.RefreshWebexTokenForRecord(record)
			if err != nil {
				log.Printf("Failed to refresh Webex token for opaque token %s: %v", opaqueToken[:8], err)
				am.sendUnauthorized(w)
				return
			}
			webexAccessToken = newToken
			// Evict old cached client
			am.clientCache.Evict(record.WebexAccessToken)
		}

		// Get or create a Webex client for this token
		client, err := am.clientCache.GetOrCreate(webexAccessToken)
		if err != nil {
			log.Printf("Failed to create Webex client: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "server_error", "Failed to create API client")
			return
		}

		// Inject the client and token into the context
		ctx := ContextWithWebexClient(r.Context(), client)
		ctx = ContextWithWebexToken(ctx, webexAccessToken)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// sendUnauthorized sends a 401 response with the WWW-Authenticate header.
func (am *AuthMiddleware) sendUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", BuildWWWAuthenticate(am.serverURL))
	writeJSONError(w, http.StatusUnauthorized, "invalid_token", "Bearer token required")
}

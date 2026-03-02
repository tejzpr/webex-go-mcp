package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/WebexCommunity/webex-go-sdk/v2/webexsdk"
)

func TestAuthMiddleware_MissingAuthorization(t *testing.T) {
	store := NewMemoryStore(time.Minute)
	defer store.Close()
	cc := NewClientCache(time.Minute, &webexsdk.Config{BaseURL: "https://example.com"})
	defer cc.Close()
	oauthHandler := NewOAuthHandler(&OAuthConfig{ServerURL: "https://example.com"}, store)

	am := NewAuthMiddleware(store, cc, oauthHandler, "https://example.com")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := am.Wrap(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("WWW-Authenticate header should be set")
	}
}

func TestAuthMiddleware_MalformedBearer(t *testing.T) {
	store := NewMemoryStore(time.Minute)
	defer store.Close()
	cc := NewClientCache(time.Minute, &webexsdk.Config{BaseURL: "https://example.com"})
	defer cc.Close()
	oauthHandler := NewOAuthHandler(&OAuthConfig{ServerURL: "https://example.com"}, store)

	am := NewAuthMiddleware(store, cc, oauthHandler, "https://example.com")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := am.Wrap(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic xxx")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddleware_UnknownToken(t *testing.T) {
	store := NewMemoryStore(time.Minute)
	defer store.Close()
	cc := NewClientCache(time.Minute, &webexsdk.Config{BaseURL: "https://example.com"})
	defer cc.Close()
	oauthHandler := NewOAuthHandler(&OAuthConfig{ServerURL: "https://example.com"}, store)

	am := NewAuthMiddleware(store, cc, oauthHandler, "https://example.com")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := am.Wrap(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-in-store-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

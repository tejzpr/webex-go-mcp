package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tejzpr/webex-go-mcp/auth"
)

func TestUploadBaseURLUsesConfiguredBaseURL(t *testing.T) {
	cfg := &HTTPServerConfig{
		BaseURL:     "https://mcp.example.com/",
		OAuthConfig: &auth.OAuthConfig{ServerURL: "https://fallback.example.com"},
	}

	got := uploadBaseURL(cfg)
	if got != "https://mcp.example.com" {
		t.Fatalf("uploadBaseURL() = %q, want %q", got, "https://mcp.example.com")
	}
}

func TestUploadBaseURLFallsBackToServerURL(t *testing.T) {
	cfg := &HTTPServerConfig{
		OAuthConfig: &auth.OAuthConfig{ServerURL: "https://mcp.example.com/"},
	}

	got := uploadBaseURL(cfg)
	if got != "https://mcp.example.com" {
		t.Fatalf("uploadBaseURL() = %q, want %q", got, "https://mcp.example.com")
	}
}

func TestAPIKeyMiddlewareAllowsMatchingKey(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	handler := apiKeyMiddleware("secret", next)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("X-API-Key", "secret")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
}

func TestAPIKeyMiddlewareRejectsMissingKey(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	})
	handler := apiKeyMiddleware("secret", next)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAPIKeyMiddlewareDisabledWhenUnset(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	handler := apiKeyMiddleware("", next)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
}

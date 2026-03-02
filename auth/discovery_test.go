package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleProtectedResourceMetadata_GET(t *testing.T) {
	cfg := &OAuthConfig{
		ServerURL: "https://example.com",
		Scopes:   "a b c",
	}
	dh := NewDiscoveryHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	dh.HandleProtectedResourceMetadata(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var meta ProtectedResourceMetadata
	if err := json.NewDecoder(rec.Body).Decode(&meta); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if meta.Resource != "https://example.com" {
		t.Errorf("resource = %q, want https://example.com", meta.Resource)
	}
	if len(meta.AuthorizationServers) != 1 || meta.AuthorizationServers[0] != "https://example.com" {
		t.Errorf("authorization_servers = %v", meta.AuthorizationServers)
	}
	if len(meta.BearerMethodsSupported) == 0 || meta.BearerMethodsSupported[0] != "header" {
		t.Errorf("bearer_methods_supported = %v", meta.BearerMethodsSupported)
	}
	if len(meta.ScopesSupported) != 3 {
		t.Errorf("scopes_supported = %v, want 3 elements", meta.ScopesSupported)
	}
}

func TestHandleProtectedResourceMetadata_POST(t *testing.T) {
	cfg := &OAuthConfig{ServerURL: "https://example.com"}
	dh := NewDiscoveryHandler(cfg)

	req := httptest.NewRequest(http.MethodPost, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	dh.HandleProtectedResourceMetadata(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleAuthorizationServerMetadata_GET(t *testing.T) {
	cfg := &OAuthConfig{
		ServerURL: "https://example.com",
		Scopes:   "openid read",
	}
	dh := NewDiscoveryHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()
	dh.HandleAuthorizationServerMetadata(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var meta AuthorizationServerMetadata
	if err := json.NewDecoder(rec.Body).Decode(&meta); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if meta.Issuer != "https://example.com" {
		t.Errorf("issuer = %q", meta.Issuer)
	}
	if meta.AuthorizationEndpoint != "https://example.com/authorize" {
		t.Errorf("authorization_endpoint = %q", meta.AuthorizationEndpoint)
	}
	if meta.TokenEndpoint != "https://example.com/token" {
		t.Errorf("token_endpoint = %q", meta.TokenEndpoint)
	}
	if meta.RegistrationEndpoint != "https://example.com/register" {
		t.Errorf("registration_endpoint = %q", meta.RegistrationEndpoint)
	}
	if len(meta.ScopesSupported) != 2 {
		t.Errorf("scopes_supported = %v", meta.ScopesSupported)
	}
}

func TestHandleAuthorizationServerMetadata_POST(t *testing.T) {
	cfg := &OAuthConfig{ServerURL: "https://example.com"}
	dh := NewDiscoveryHandler(cfg)

	req := httptest.NewRequest(http.MethodPost, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()
	dh.HandleAuthorizationServerMetadata(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

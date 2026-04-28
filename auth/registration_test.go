package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleRegisterCodexCompatibleResponse(t *testing.T) {
	store := NewMemoryStore(time.Hour)
	defer store.Close()

	body := strings.NewReader(`{
		"client_name": "Codex",
		"redirect_uris": ["http://127.0.0.1:65530/callback"],
		"token_endpoint_auth_method": "none",
		"grant_types": ["authorization_code"],
		"response_types": ["code"],
		"scope": "spark:all"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/register", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	HandleRegister(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}

	var resp RegistrationResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ClientID == "" {
		t.Fatal("client_id is empty")
	}
	if resp.ClientName != "Codex" {
		t.Fatalf("client_name = %q, want Codex", resp.ClientName)
	}
	if resp.Scope != "spark:all" {
		t.Fatalf("scope = %q, want spark:all", resp.Scope)
	}
	if resp.TokenEndpointAuthMethod != "none" {
		t.Fatalf("token_endpoint_auth_method = %q, want none", resp.TokenEndpointAuthMethod)
	}
}

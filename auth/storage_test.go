package auth

import (
	"os"
	"testing"
	"time"
)

func getTestStores(t *testing.T) map[string]Store {
	t.Helper()
	stores := map[string]Store{
		"memory": NewMemoryStore(time.Minute),
	}
	sqliteStore, err := NewSQLiteStore(":memory:", time.Minute)
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	stores["sqlite"] = sqliteStore
	if dsn := os.Getenv("POSTGRES_TEST_DSN"); dsn != "" {
		pgStore, err := NewPostgresStore(dsn, time.Minute)
		if err != nil {
			t.Fatalf("failed to create postgres store: %v", err)
		}
		stores["postgres"] = pgStore
	}
	return stores
}

func TestStoreTokenLookupToken(t *testing.T) {
	for name, s := range getTestStores(t) {
		s := s
		defer s.Close()
		t.Run(name+"/StoreToken_LookupToken_lifecycle", func(t *testing.T) {
			opaque, err := s.StoreToken("webex-at", "webex-rt", 3600)
			if err != nil {
				t.Fatalf("StoreToken: %v", err)
			}
			if opaque == "" {
				t.Fatal("StoreToken returned empty opaque token")
			}

			record, ok := s.LookupToken(opaque)
			if !ok {
				t.Fatal("LookupToken: token not found")
			}
			if record.OpaqueToken != opaque {
				t.Errorf("OpaqueToken = %q, want %q", record.OpaqueToken, opaque)
			}
			if record.WebexAccessToken != "webex-at" {
				t.Errorf("WebexAccessToken = %q, want webex-at", record.WebexAccessToken)
			}
			if record.WebexRefreshToken != "webex-rt" {
				t.Errorf("WebexRefreshToken = %q, want webex-rt", record.WebexRefreshToken)
			}
			if record.ExpiresAt.IsZero() {
				t.Error("ExpiresAt is zero")
			}
			if record.CreatedAt.IsZero() {
				t.Error("CreatedAt is zero")
			}
		})
	}
}

func TestUpdateWebexToken(t *testing.T) {
	for name, s := range getTestStores(t) {
		s := s
		defer s.Close()
		t.Run(name+"/UpdateWebexToken", func(t *testing.T) {
			opaque, err := s.StoreToken("old-at", "old-rt", 3600)
			if err != nil {
				t.Fatalf("StoreToken: %v", err)
			}

			err = s.UpdateWebexToken(opaque, "new-at", "new-rt", 7200)
			if err != nil {
				t.Fatalf("UpdateWebexToken: %v", err)
			}

			record, ok := s.LookupToken(opaque)
			if !ok {
				t.Fatal("LookupToken: token not found after update")
			}
			if record.WebexAccessToken != "new-at" {
				t.Errorf("WebexAccessToken = %q, want new-at", record.WebexAccessToken)
			}
			if record.WebexRefreshToken != "new-rt" {
				t.Errorf("WebexRefreshToken = %q, want new-rt", record.WebexRefreshToken)
			}
		})
	}
}

func TestRevokeToken(t *testing.T) {
	for name, s := range getTestStores(t) {
		s := s
		defer s.Close()
		t.Run(name+"/RevokeToken", func(t *testing.T) {
			opaque, err := s.StoreToken("at", "rt", 3600)
			if err != nil {
				t.Fatalf("StoreToken: %v", err)
			}

			s.RevokeToken(opaque)

			_, ok := s.LookupToken(opaque)
			if ok {
				t.Fatal("LookupToken: expected token to be gone after revoke")
			}
		})
	}
}

func TestStoreAuthCodeConsumeAuthCode(t *testing.T) {
	for name, s := range getTestStores(t) {
		s := s
		defer s.Close()
		t.Run(name+"/StoreAuthCode_ConsumeAuthCode_lifecycle", func(t *testing.T) {
			rec := &AuthCodeRecord{
				Code:                "code-abc",
				ClientID:            "client-1",
				RedirectURI:         "https://example.com/cb",
				CodeChallenge:       "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
				CodeChallengeMethod: "S256",
				WebexAccessToken:    "wat",
				WebexRefreshToken:   "wrt",
				WebexExpiresIn:      3600,
				CreatedAt:           time.Now(),
				ExpiresAt:           time.Now().Add(5 * time.Minute),
			}
			if err := s.StoreAuthCode(rec); err != nil {
				t.Fatalf("StoreAuthCode: %v", err)
			}

			consumed, ok := s.ConsumeAuthCode("code-abc")
			if !ok {
				t.Fatal("ConsumeAuthCode: first consume failed")
			}
			if consumed.Code != "code-abc" {
				t.Errorf("Code = %q, want code-abc", consumed.Code)
			}
			if consumed.CodeChallenge != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
				t.Errorf("CodeChallenge = %q, want S256 challenge", consumed.CodeChallenge)
			}
			if consumed.CodeChallengeMethod != "S256" {
				t.Errorf("CodeChallengeMethod = %q, want S256", consumed.CodeChallengeMethod)
			}

			_, ok = s.ConsumeAuthCode("code-abc")
			if ok {
				t.Fatal("ConsumeAuthCode: second consume should return false (one-time use)")
			}
		})
	}
}

func TestAuthCodeExpiry(t *testing.T) {
	for name, s := range getTestStores(t) {
		s := s
		defer s.Close()
		t.Run(name+"/AuthCode_expiry", func(t *testing.T) {
			rec := &AuthCodeRecord{
				Code:              "expired-code",
				ClientID:          "client-1",
				RedirectURI:       "https://example.com/cb",
				WebexAccessToken:  "wat",
				WebexRefreshToken: "wrt",
				WebexExpiresIn:    3600,
				CreatedAt:         time.Now(),
				ExpiresAt:         time.Now().Add(-1 * time.Minute), // past
			}
			if err := s.StoreAuthCode(rec); err != nil {
				t.Fatalf("StoreAuthCode: %v", err)
			}

			_, ok := s.ConsumeAuthCode("expired-code")
			if ok {
				t.Fatal("ConsumeAuthCode: expired code should return false")
			}
		})
	}
}

func TestStorePendingAuthConsumePendingAuth(t *testing.T) {
	for name, s := range getTestStores(t) {
		s := s
		defer s.Close()
		t.Run(name+"/StorePendingAuth_ConsumePendingAuth_lifecycle", func(t *testing.T) {
			pending := &PendingAuth{
				State:             "state-xyz",
				ClientID:          "client-1",
				ClientRedirectURI: "https://example.com/cb",
				WebexCodeVerifier: "verifier-123",
				CreatedAt:         time.Now(),
			}
			if err := s.StorePendingAuth(pending); err != nil {
				t.Fatalf("StorePendingAuth: %v", err)
			}

			consumed, ok := s.ConsumePendingAuth("state-xyz")
			if !ok {
				t.Fatal("ConsumePendingAuth: first consume failed")
			}
			if consumed.State != "state-xyz" {
				t.Errorf("State = %q, want state-xyz", consumed.State)
			}

			_, ok = s.ConsumePendingAuth("state-xyz")
			if ok {
				t.Fatal("ConsumePendingAuth: second consume should return false (one-time use)")
			}
		})
	}
}

func TestPendingAuthExpiry(t *testing.T) {
	for name, s := range getTestStores(t) {
		s := s
		defer s.Close()
		t.Run(name+"/PendingAuth_expiry", func(t *testing.T) {
			pending := &PendingAuth{
				State:             "old-state",
				ClientID:          "client-1",
				ClientRedirectURI: "https://example.com/cb",
				WebexCodeVerifier: "verifier",
				CreatedAt:         time.Now().Add(-15 * time.Minute), // far past
			}
			if err := s.StorePendingAuth(pending); err != nil {
				t.Fatalf("StorePendingAuth: %v", err)
			}

			_, ok := s.ConsumePendingAuth("old-state")
			if ok {
				t.Fatal("ConsumePendingAuth: expired pending auth should return false")
			}
		})
	}
}

func TestRegisterClient(t *testing.T) {
	for name, s := range getTestStores(t) {
		s := s
		defer s.Close()
		t.Run(name+"/RegisterClient_defaults", func(t *testing.T) {
			req := &RegistrationRequest{
				RedirectURIs: []string{"https://example.com/cb"},
			}
			client, err := s.RegisterClient(req)
			if err != nil {
				t.Fatalf("RegisterClient: %v", err)
			}
			if client.ClientID == "" {
				t.Error("ClientID is empty")
			}
			if len(client.RedirectURIs) != 1 || client.RedirectURIs[0] != "https://example.com/cb" {
				t.Errorf("RedirectURIs = %v", client.RedirectURIs)
			}
			if client.TokenEndpointAuthMethod != "none" {
				t.Errorf("TokenEndpointAuthMethod = %q, want none", client.TokenEndpointAuthMethod)
			}

			lookup, ok := s.LookupClient(client.ClientID)
			if !ok {
				t.Fatal("LookupClient: client not found")
			}
			if lookup.ClientID != client.ClientID {
				t.Errorf("LookupClient ClientID = %q, want %q", lookup.ClientID, client.ClientID)
			}
		})
	}
}

func TestRegisterClientWithSecret(t *testing.T) {
	for name, s := range getTestStores(t) {
		s := s
		defer s.Close()
		t.Run(name+"/RegisterClient_secret", func(t *testing.T) {
			req := &RegistrationRequest{
				RedirectURIs:            []string{"https://example.com/cb"},
				TokenEndpointAuthMethod: "client_secret_post",
			}
			client, err := s.RegisterClient(req)
			if err != nil {
				t.Fatalf("RegisterClient: %v", err)
			}
			if client.ClientSecret == "" {
				t.Error("ClientSecret should be non-empty for client_secret_post")
			}
			if client.TokenEndpointAuthMethod != "client_secret_post" {
				t.Errorf("TokenEndpointAuthMethod = %q, want client_secret_post", client.TokenEndpointAuthMethod)
			}
		})
	}
}

func TestRegisterClientWithID(t *testing.T) {
	for name, s := range getTestStores(t) {
		s := s
		defer s.Close()
		t.Run(name+"/RegisterClientWithID", func(t *testing.T) {
			clientID := "my-client-id"
			uri1 := "https://example.com/cb1"
			uri2 := "https://example.com/cb2"

			// Register new client
			if err := s.RegisterClientWithID(clientID, uri1); err != nil {
				t.Fatalf("RegisterClientWithID (new): %v", err)
			}
			c, ok := s.LookupClient(clientID)
			if !ok {
				t.Fatal("LookupClient: client not found after first register")
			}
			if len(c.RedirectURIs) != 1 || c.RedirectURIs[0] != uri1 {
				t.Errorf("RedirectURIs = %v, want [%s]", c.RedirectURIs, uri1)
			}

			// Call again with same clientID and different URI - URI appended
			if err := s.RegisterClientWithID(clientID, uri2); err != nil {
				t.Fatalf("RegisterClientWithID (append): %v", err)
			}
			c, ok = s.LookupClient(clientID)
			if !ok {
				t.Fatal("LookupClient: client not found after append")
			}
			if len(c.RedirectURIs) != 2 {
				t.Errorf("RedirectURIs len = %d, want 2", len(c.RedirectURIs))
			}
			if c.RedirectURIs[0] != uri1 || c.RedirectURIs[1] != uri2 {
				t.Errorf("RedirectURIs = %v", c.RedirectURIs)
			}

			// Call again with same URI - idempotent
			if err := s.RegisterClientWithID(clientID, uri1); err != nil {
				t.Fatalf("RegisterClientWithID (idempotent): %v", err)
			}
			c, ok = s.LookupClient(clientID)
			if !ok {
				t.Fatal("LookupClient: client not found after idempotent call")
			}
			if len(c.RedirectURIs) != 2 {
				t.Errorf("RedirectURIs len = %d after idempotent, want 2", len(c.RedirectURIs))
			}
		})
	}
}

func TestValidateRedirectURI(t *testing.T) {
	for name, s := range getTestStores(t) {
		s := s
		defer s.Close()
		t.Run(name+"/ValidateRedirectURI", func(t *testing.T) {
			if err := s.RegisterClientWithID("client-1", "https://example.com/cb"); err != nil {
				t.Fatalf("RegisterClientWithID: %v", err)
			}

			if !s.ValidateRedirectURI("client-1", "https://example.com/cb") {
				t.Error("ValidateRedirectURI: valid URI should return true")
			}
			if s.ValidateRedirectURI("client-1", "https://evil.com/cb") {
				t.Error("ValidateRedirectURI: invalid URI should return false")
			}
			if s.ValidateRedirectURI("unknown-client", "https://example.com/cb") {
				t.Error("ValidateRedirectURI: unknown client should return false")
			}
		})
	}
}

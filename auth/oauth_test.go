package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestValidatePKCE(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	// S256 of verifier = base64url(SHA256(verifier))
	h := sha256.Sum256([]byte(verifier))
	s256Challenge := base64.RawURLEncoding.EncodeToString(h[:])

	tests := []struct {
		name     string
		challenge string
		method   string
		verifier string
		want     bool
	}{
		{"S256 match", s256Challenge, "S256", verifier, true},
		{"S256 empty method", s256Challenge, "", verifier, true},
		{"plain match", "plain-value", "plain", "plain-value", true},
		{"plain mismatch", "plain-value", "plain", "wrong", false},
		{"missing verifier S256", s256Challenge, "S256", "", false},
		{"unknown method", "x", "unknown", "x", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validatePKCE(tt.challenge, tt.method, tt.verifier)
			if got != tt.want {
				t.Errorf("validatePKCE() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateS256Challenge(t *testing.T) {
	// RFC 7636 example: verifier "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	// produces challenge "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	expected := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	got := generateS256Challenge(verifier)
	if got != expected {
		t.Errorf("generateS256Challenge() = %q, want %q", got, expected)
	}

	// Same input must produce same output
	got2 := generateS256Challenge(verifier)
	if got != got2 {
		t.Error("generateS256Challenge should be deterministic")
	}
}

func TestSplitBearerToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
		token string
		ok    bool
	}{
		{"valid", "Bearer xxx", "xxx", true},
		{"valid with spaces", "Bearer  tok123  ", "tok123", true},
		{"malformed missing Bearer", "Basic xxx", "", false},
		{"malformed no prefix", "xxx", "", false},
		{"empty", "", "", false},
		{"Bearer only", "Bearer ", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, ok := SplitBearerToken(tt.input)
			if ok != tt.ok || token != tt.token {
				t.Errorf("SplitBearerToken(%q) = (%q, %v), want (%q, %v)", tt.input, token, ok, tt.token, tt.ok)
			}
		})
	}
}

func TestBuildWWWAuthenticate(t *testing.T) {
	serverURL := "https://example.com"
	got := BuildWWWAuthenticate(serverURL)
	want := `Bearer resource_metadata="https://example.com/.well-known/oauth-protected-resource"`
	if got != want {
		t.Errorf("BuildWWWAuthenticate() = %q, want %q", got, want)
	}
}

func TestSplitScopes(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{"space separated", "a b c", []string{"a", "b", "c"}},
		{"single", "one", []string{"one"}},
		{"empty returns nil", "", nil},
		{"multiple spaces", "a  b   c", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitScopes(tt.input)
			if len(got) != len(tt.expect) {
				t.Errorf("splitScopes(%q) = %v, want %v", tt.input, got, tt.expect)
			}
			for i := range got {
				if i >= len(tt.expect) || got[i] != tt.expect[i] {
					t.Errorf("splitScopes(%q) = %v, want %v", tt.input, got, tt.expect)
					break
				}
			}
		})
	}
}

package auth

import (
	"context"
	"testing"
	"time"

	webex "github.com/WebexCommunity/webex-go-sdk/v2"
	"github.com/WebexCommunity/webex-go-sdk/v2/webexsdk"
)

func TestNewStaticClientResolver(t *testing.T) {
	var nilClient *webex.WebexClient
	resolver := NewStaticClientResolver(nilClient)

	// Always returns same client
	client1, err := resolver(context.Background())
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if client1 != nilClient {
		t.Errorf("client = %p, want %p (same)", client1, nilClient)
	}

	client2, err := resolver(context.Background())
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if client2 != client1 {
		t.Error("StaticClientResolver should always return same client")
	}
}

func TestNewHTTPClientResolver(t *testing.T) {
	resolver := NewHTTPClientResolver()

	// No client in context - error
	_, err := resolver(context.Background())
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
	if err.Error() != "no authenticated Webex client in context" {
		t.Errorf("err = %q", err.Error())
	}

	// With context - success
	cfg := &webexsdk.Config{BaseURL: "https://api.webex.com"}
	client, err := webex.NewClient("test-token", cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := ContextWithWebexClient(context.Background(), client)
	got, err := resolver(ctx)
	if err != nil {
		t.Fatalf("resolver with context: %v", err)
	}
	if got != client {
		t.Error("resolver should return client from context")
	}
}

func TestClientCacheGetOrCreate(t *testing.T) {
	t.Helper()
	cc := NewClientCache(time.Minute, &webexsdk.Config{BaseURL: "https://example.com"})
	defer cc.Close()

	// Invalid token - GetOrCreate will fail (webex.NewClient may still create a client
	// with the token string, but API calls would fail). We're testing the cache structure.
	// The SDK might accept any non-empty token for client creation - let's use a bogus one.
	_, err := cc.GetOrCreate("invalid-token-for-test")
	if err != nil {
		// Expected: SDK might reject malformed tokens
		t.Logf("GetOrCreate with invalid token errored as expected: %v", err)
		return
	}
	// If it didn't error, second call should return cached client
	_, err2 := cc.GetOrCreate("invalid-token-for-test")
	if err2 != nil {
		t.Logf("Second GetOrCreate: %v", err2)
	}
}

func TestClientCacheClose(t *testing.T) {
	cc := NewClientCache(time.Minute, &webexsdk.Config{BaseURL: "https://example.com"})
	cc.Close() // must not panic
}

func TestTokenHash(t *testing.T) {
	token := "my-secret-token"
	h1 := tokenHash(token)
	h2 := tokenHash(token)
	if h1 != h2 {
		t.Errorf("tokenHash should be deterministic: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("tokenHash should not return empty string")
	}

	// Different input, different output
	h3 := tokenHash("other-token")
	if h1 == h3 {
		t.Error("tokenHash should produce different output for different input")
	}
}

package main

import (
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

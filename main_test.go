package main

import "testing"

func TestNormalizeHTTPBaseURLSupportsLocalhostWithoutScheme(t *testing.T) {
	got, err := normalizeHTTPBaseURL("localhost:8560")
	if err != nil {
		t.Fatalf("normalizeHTTPBaseURL() error = %v", err)
	}
	if got != "http://localhost:8560" {
		t.Fatalf("normalizeHTTPBaseURL() = %q, want %q", got, "http://localhost:8560")
	}
}

func TestNormalizeHTTPBaseURLTrimsTrailingSlash(t *testing.T) {
	got, err := normalizeHTTPBaseURL("https://mcp.example.com/")
	if err != nil {
		t.Fatalf("normalizeHTTPBaseURL() error = %v", err)
	}
	if got != "https://mcp.example.com" {
		t.Fatalf("normalizeHTTPBaseURL() = %q, want %q", got, "https://mcp.example.com")
	}
}

func TestNormalizeHTTPBaseURLRejectsUnsupportedScheme(t *testing.T) {
	if _, err := normalizeHTTPBaseURL("ftp://localhost:8560"); err == nil {
		t.Fatal("normalizeHTTPBaseURL() error = nil, want error")
	}
}

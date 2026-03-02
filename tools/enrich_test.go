package tools

import (
	"testing"
)

func TestIsTextContentType(t *testing.T) {
	tests := []struct {
		ct     string
		expect bool
	}{
		{"text/plain", true},
		{"text/html", true},
		{"application/json", true},
		{"application/xml", true},
		{"image/png", false},
		{"application/octet-stream", false},
		{"video/mp4", false},
	}
	for _, tt := range tests {
		got := isTextContentType(tt.ct)
		if got != tt.expect {
			t.Errorf("isTextContentType(%q) = %v, want %v", tt.ct, got, tt.expect)
		}
	}
}

func TestParseContentDisposition(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{`attachment; filename="test.pdf"`, "test.pdf"},
		{`attachment`, ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseContentDisposition(tt.header)
		if got != tt.want {
			t.Errorf("parseContentDisposition(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestPersonNameCache_NilClient(t *testing.T) {
	cache := NewPersonNameCache(nil)
	got := cache.Resolve("unknown-id")
	if got != "" {
		t.Errorf("Resolve(unknown-id) = %q, want \"\"", got)
	}
}

func TestTeamNameCache_NilClient(t *testing.T) {
	cache := NewTeamNameCache(nil)
	got := cache.Resolve("unknown-id")
	if got != "" {
		t.Errorf("Resolve(unknown-id) = %q, want \"\"", got)
	}
}

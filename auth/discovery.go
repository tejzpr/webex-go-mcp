package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

const mcpResourcePath = "/mcp"
const protectedResourceMetadataPath = "/.well-known/oauth-protected-resource"

// OAuthConfig holds the Webex Integration OAuth configuration.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	Scopes       string
	RedirectURI  string // Our /callback URL registered with Webex
	ServerURL    string // Our server's external base URL (e.g. http://localhost:8080)
}

// ProtectedResourceMetadata is the RFC 9728 metadata document.
type ProtectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
}

// AuthorizationServerMetadata is the RFC 8414 metadata document.
type AuthorizationServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported,omitempty"`
}

// DiscoveryHandler serves the well-known metadata endpoints.
type DiscoveryHandler struct {
	config *OAuthConfig
}

// NewDiscoveryHandler creates a new discovery handler.
func NewDiscoveryHandler(config *OAuthConfig) *DiscoveryHandler {
	return &DiscoveryHandler{config: config}
}

// HandleProtectedResourceMetadata serves GET /.well-known/oauth-protected-resource
func (dh *DiscoveryHandler) HandleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serverURL := canonicalServerURL(dh.config.ServerURL)
	meta := &ProtectedResourceMetadata{
		Resource:               mcpResourceURL(serverURL),
		AuthorizationServers:   []string{serverURL},
		BearerMethodsSupported: []string{"header"},
	}

	if dh.config.Scopes != "" {
		meta.ScopesSupported = splitScopes(dh.config.Scopes)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(meta)
}

// HandleAuthorizationServerMetadata serves GET /.well-known/oauth-authorization-server
func (dh *DiscoveryHandler) HandleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serverURL := canonicalServerURL(dh.config.ServerURL)
	meta := &AuthorizationServerMetadata{
		Issuer:                            serverURL,
		AuthorizationEndpoint:             serverURL + "/authorize",
		TokenEndpoint:                     serverURL + "/token",
		RegistrationEndpoint:              serverURL + "/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code"},
		TokenEndpointAuthMethodsSupported: []string{"none", "client_secret_post", "client_secret_basic"},
		CodeChallengeMethodsSupported:     []string{"S256", "plain"},
	}

	if dh.config.Scopes != "" {
		meta.ScopesSupported = splitScopes(dh.config.Scopes)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(meta)
}

// splitScopes splits a space-separated scope string into a slice.
func splitScopes(scopes string) []string {
	var result []string
	for _, s := range splitNonEmpty(scopes, ' ') {
		result = append(result, s)
	}
	return result
}

func canonicalServerURL(serverURL string) string {
	return strings.TrimRight(serverURL, "/")
}

func mcpResourceURL(serverURL string) string {
	return canonicalServerURL(serverURL) + mcpResourcePath
}

func protectedResourceMetadataURL(serverURL string) string {
	return canonicalServerURL(serverURL) + protectedResourceMetadataPath + mcpResourcePath
}

// splitNonEmpty splits a string by a separator and returns non-empty parts.
func splitNonEmpty(s string, sep byte) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

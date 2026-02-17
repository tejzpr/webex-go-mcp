package auth

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// RegisteredClient represents a dynamically registered OAuth client (RFC 7591).
type RegisteredClient struct {
	ClientID                string    `json:"client_id"`
	ClientSecret            string    `json:"client_secret,omitempty"`
	RedirectURIs            []string  `json:"redirect_uris"`
	ClientName              string    `json:"client_name,omitempty"`
	TokenEndpointAuthMethod string    `json:"token_endpoint_auth_method,omitempty"`
	GrantTypes              []string  `json:"grant_types,omitempty"`
	ResponseTypes           []string  `json:"response_types,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
}

// ClientRegistry manages dynamically registered OAuth clients.
type ClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]*RegisteredClient // client_id -> client
}

// NewClientRegistry creates a new client registry.
func NewClientRegistry() *ClientRegistry {
	return &ClientRegistry{
		clients: make(map[string]*RegisteredClient),
	}
}

// Register creates a new dynamically registered client.
func (cr *ClientRegistry) Register(req *RegistrationRequest) (*RegisteredClient, error) {
	clientID, err := generateSecureToken(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate client_id: %w", err)
	}

	var clientSecret string
	authMethod := req.TokenEndpointAuthMethod
	if authMethod == "" {
		authMethod = "none" // public client by default (MCP clients are typically public)
	}
	if authMethod == "client_secret_post" || authMethod == "client_secret_basic" {
		clientSecret, err = generateSecureToken(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate client_secret: %w", err)
		}
	}

	grantTypes := req.GrantTypes
	if len(grantTypes) == 0 {
		grantTypes = []string{"authorization_code"}
	}

	responseTypes := req.ResponseTypes
	if len(responseTypes) == 0 {
		responseTypes = []string{"code"}
	}

	client := &RegisteredClient{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		RedirectURIs:            req.RedirectURIs,
		ClientName:              req.ClientName,
		TokenEndpointAuthMethod: authMethod,
		GrantTypes:              grantTypes,
		ResponseTypes:           responseTypes,
		CreatedAt:               time.Now(),
	}

	cr.mu.Lock()
	cr.clients[clientID] = client
	cr.mu.Unlock()

	return client, nil
}

// Lookup retrieves a registered client by client_id.
func (cr *ClientRegistry) Lookup(clientID string) (*RegisteredClient, bool) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	client, ok := cr.clients[clientID]
	return client, ok
}

// ValidateRedirectURI checks if the given redirect_uri is allowed for the client.
func (cr *ClientRegistry) ValidateRedirectURI(clientID, redirectURI string) bool {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	client, ok := cr.clients[clientID]
	if !ok {
		return false
	}
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			return true
		}
	}
	return false
}

// RegistrationRequest is the request body for RFC 7591 Dynamic Client Registration.
type RegistrationRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
}

// RegistrationResponse is the response body for RFC 7591 Dynamic Client Registration.
type RegistrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

// HandleRegister handles POST /register for RFC 7591 Dynamic Client Registration.
func (cr *ClientRegistry) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_client_metadata", "Invalid request body")
		return
	}

	if len(req.RedirectURIs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_client_metadata", "redirect_uris is required")
		return
	}

	log.Printf("[DCR] /register: client_name=%s redirect_uris=%v", req.ClientName, req.RedirectURIs)

	client, err := cr.Register(&req)
	if err != nil {
		log.Printf("[DCR] /register: FAILED - %v", err)
		writeJSONError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	log.Printf("[DCR] /register: SUCCESS - client_id=%s", client.ClientID)

	resp := &RegistrationResponse{
		ClientID:                client.ClientID,
		ClientSecret:            client.ClientSecret,
		ClientIDIssuedAt:        client.CreatedAt.Unix(),
		RedirectURIs:            client.RedirectURIs,
		ClientName:              client.ClientName,
		TokenEndpointAuthMethod: client.TokenEndpointAuthMethod,
		GrantTypes:              client.GrantTypes,
		ResponseTypes:           client.ResponseTypes,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// writeJSONError writes an OAuth 2.1 error response.
func writeJSONError(w http.ResponseWriter, status int, errorCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errorCode,
		"error_description": description,
	})
}

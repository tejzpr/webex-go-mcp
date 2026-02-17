package auth

import (
	"encoding/json"
	"log"
	"net/http"
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

// NOTE: The old ClientRegistry struct and its methods have been removed.
// All client registration is now handled through the Store interface (see storage.go).

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

// HandleRegister returns an http.HandlerFunc for POST /register (RFC 7591 Dynamic Client Registration).
func HandleRegister(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		client, err := store.RegisterClient(&req)
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

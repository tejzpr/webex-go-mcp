package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/tejzpr/webex-go-mcp/auth"
	"github.com/tejzpr/webex-go-mcp/tools"

	"github.com/tejzpr/webex-go-sdk/v2/webexsdk"

	"github.com/mark3labs/mcp-go/server"
)

// registerTools creates the MCP server and registers all tool groups with the given resolver.
func registerTools(resolver auth.ClientResolver, include, exclude string, minimal, readonlyMinimal bool) *server.MCPServer {
	s := server.NewMCPServer(
		"webex-mcp",
		version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	// Resolve preset flags into the include list
	include = tools.ResolvePresets(minimal, readonlyMinimal, include)

	// Build the tool registrar — either filtered or direct
	filter := tools.NewToolFilter(include, exclude)
	var registrar tools.ToolRegistrar

	if filter.IsActive() {
		fr := tools.NewFilteredRegistrar(s, filter)
		registrar = fr
		defer func() {
			registered, skipped := fr.Stats()
			log.Printf("Tool filtering active: %d registered, %d skipped", registered, skipped)
		}()
	} else {
		registrar = s
	}

	// Register all tool groups
	tools.RegisterMessageTools(registrar, resolver)
	tools.RegisterRoomTools(registrar, resolver)
	tools.RegisterTeamTools(registrar, resolver)
	tools.RegisterMembershipTools(registrar, resolver)
	tools.RegisterMeetingTools(registrar, resolver)
	tools.RegisterTranscriptTools(registrar, resolver)
	tools.RegisterWebhookTools(registrar, resolver)

	return s
}

// startSTDIOServer starts the MCP server in STDIO mode.
func startSTDIOServer(resolver auth.ClientResolver, include, exclude string, minimal, readonlyMinimal bool) error {
	s := registerTools(resolver, include, exclude, minimal, readonlyMinimal)
	return server.ServeStdio(s)
}

// HTTPServerConfig holds configuration for the HTTP mode.
type HTTPServerConfig struct {
	Host            string
	Port            int
	TLSCert         string
	TLSKey          string
	OAuthConfig     *auth.OAuthConfig
	WebexSDKConfig  *webexsdk.Config
	Include         string
	Exclude         string
	Minimal         bool
	ReadonlyMinimal bool
}

// startHTTPServer starts the MCP server in HTTP mode with OAuth 2.1 support.
func startHTTPServer(cfg *HTTPServerConfig) error {
	// Initialize auth components
	tokenStore := auth.NewTokenStore()
	clientRegistry := auth.NewClientRegistry()
	clientCache := auth.NewClientCache(15*time.Minute, cfg.WebexSDKConfig)

	// Create OAuth handler
	oauthHandler := auth.NewOAuthHandler(cfg.OAuthConfig, tokenStore, clientRegistry)

	// Create discovery handler
	discoveryHandler := auth.NewDiscoveryHandler(cfg.OAuthConfig)

	// Create auth middleware
	authMiddleware := auth.NewAuthMiddleware(tokenStore, clientCache, oauthHandler, cfg.OAuthConfig.ServerURL)

	// Create the HTTP client resolver
	resolver := auth.NewHTTPClientResolver()

	// Register tools with the resolver
	mcpServer := registerTools(resolver, cfg.Include, cfg.Exclude, cfg.Minimal, cfg.ReadonlyMinimal)

	// Create the Streamable HTTP server
	streamableServer := server.NewStreamableHTTPServer(mcpServer)

	// Build the HTTP mux
	mux := http.NewServeMux()

	// Discovery endpoints (unauthenticated)
	mux.HandleFunc("/.well-known/oauth-protected-resource", discoveryHandler.HandleProtectedResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-authorization-server", discoveryHandler.HandleAuthorizationServerMetadata)

	// OAuth endpoints (unauthenticated)
	mux.HandleFunc("/authorize", oauthHandler.HandleAuthorize)
	mux.HandleFunc("/callback", oauthHandler.HandleCallback)
	mux.HandleFunc("/token", oauthHandler.HandleToken)

	// Dynamic Client Registration (unauthenticated)
	mux.HandleFunc("/register", clientRegistry.HandleRegister)

	// MCP endpoint (authenticated)
	mux.Handle("/mcp", authMiddleware.Wrap(streamableServer))

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		log.Printf("Starting Webex MCP Server v%s in HTTP mode (https://%s)", version, addr)
		tlsServer := &http.Server{
			Addr:    addr,
			Handler: mux,
			TLSConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}
		return tlsServer.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
	}

	log.Printf("Starting Webex MCP Server v%s in HTTP mode (http://%s)", version, addr)
	return http.ListenAndServe(addr, mux)
}

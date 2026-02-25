package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/tejzpr/webex-go-mcp/auth"
	"github.com/tejzpr/webex-go-mcp/streaming"
	"github.com/tejzpr/webex-go-mcp/tools"

	"github.com/WebexCommunity/webex-go-sdk/v2/webexsdk"

	"github.com/mark3labs/mcp-go/server"
)

// registerTools creates the MCP server and registers all tool groups with the given resolver.
// If mercuryMgr is non-nil, streaming tools (subscribe, unsubscribe, wait_for_message) are also registered.
func registerTools(resolver auth.ClientResolver, include, exclude string, minimal, readonlyMinimal bool, mercuryMgr *streaming.MercuryManager) *server.MCPServer {
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

	// Register streaming tools only when MercuryManager is available (HTTP mode)
	if mercuryMgr != nil {
		tools.RegisterStreamingTools(registrar, resolver, mercuryMgr)
	}

	return s
}

// startSTDIOServer starts the MCP server in STDIO mode.
func startSTDIOServer(resolver auth.ClientResolver, include, exclude string, minimal, readonlyMinimal bool) error {
	// Create MCPServer first, then wire up MercuryManager for streaming tools
	s := registerTools(resolver, include, exclude, minimal, readonlyMinimal, nil)

	// Create MercuryManager and register streaming tools (works in STDIO too)
	mercuryMgr := streaming.NewMercuryManager(s)
	tools.RegisterStreamingTools(s, resolver, mercuryMgr)

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
	StoreConfig     auth.StoreConfig
	Include         string
	Exclude         string
	Minimal         bool
	ReadonlyMinimal bool
}

// requestLoggingMiddleware logs every incoming HTTP request for debugging.
func requestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[HTTP] %s %s (from %s, Content-Type: %s, Auth: %s)",
			r.Method, r.URL.String(), r.RemoteAddr,
			r.Header.Get("Content-Type"),
			truncateHeader(r.Header.Get("Authorization"), 20))
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers to all responses.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Session-Id")
		w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// truncateHeader truncates a header value for safe logging.
func truncateHeader(value string, maxLen int) string {
	if value == "" {
		return "(none)"
	}
	if len(value) > maxLen {
		return value[:maxLen] + "..."
	}
	return value
}

// startHTTPServer starts the MCP server in HTTP mode with OAuth 2.1 support.
func startHTTPServer(cfg *HTTPServerConfig) error {
	// Initialize store
	store, err := auth.NewStore(cfg.StoreConfig)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	defer store.Close()

	log.Printf("Using %s store", cfg.StoreConfig.Type)

	clientCache := auth.NewClientCache(15*time.Minute, cfg.WebexSDKConfig)

	// Create OAuth handler
	oauthHandler := auth.NewOAuthHandler(cfg.OAuthConfig, store)

	// Create discovery handler
	discoveryHandler := auth.NewDiscoveryHandler(cfg.OAuthConfig)

	// Create auth middleware
	authMiddleware := auth.NewAuthMiddleware(store, clientCache, oauthHandler, cfg.OAuthConfig.ServerURL)

	// Create the HTTP client resolver
	resolver := auth.NewHTTPClientResolver()

	// Register tools with the resolver.
	// MercuryManager needs the MCPServer ref, so we pass nil first, then register streaming tools after.
	mcpServer := registerTools(resolver, cfg.Include, cfg.Exclude, cfg.Minimal, cfg.ReadonlyMinimal, nil)

	// Create MercuryManager for streaming tools (needs MCPServer for notifications)
	mercuryMgr := streaming.NewMercuryManager(mcpServer)

	// Register streaming tools now that we have both the MCPServer and MercuryManager
	tools.RegisterStreamingTools(mcpServer, resolver, mercuryMgr)

	// Create the Streamable HTTP server with context propagation
	// The auth middleware injects the Webex client into the HTTP request context,
	// but mcp-go creates a new context for tool handlers. WithHTTPContextFunc
	// bridges the two by copying our context values into the MCP context.
	streamableServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			// Copy Webex client from HTTP request context to MCP tool handler context
			if client, ok := auth.WebexClientFromContext(r.Context()); ok {
				ctx = auth.ContextWithWebexClient(ctx, client)
			}
			if token, ok := auth.WebexTokenFromContext(r.Context()); ok {
				ctx = auth.ContextWithWebexToken(ctx, token)
			}
			return ctx
		}),
	)

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
	mux.HandleFunc("/register", auth.HandleRegister(store))

	// MCP endpoint (authenticated)
	mux.Handle("/mcp", authMiddleware.Wrap(streamableServer))

	// Wrap with logging and CORS
	handler := requestLoggingMiddleware(corsMiddleware(mux))

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		log.Printf("Starting Webex MCP Server v%s in HTTP mode (https://%s)", version, addr)
		tlsServer := &http.Server{
			Addr:    addr,
			Handler: handler,
			TLSConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}
		return tlsServer.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
	}

	log.Printf("Starting Webex MCP Server v%s in HTTP mode (http://%s)", version, addr)
	return http.ListenAndServe(addr, handler)
}

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tejzpr/webex-go-mcp/auth"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/webexsdk"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version = "0.1.0"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "webex-go-mcp",
		Short:   "Webex MCP Server - STDIO and HTTP MCP server for Webex APIs",
		Version: version,
		RunE:    run,
	}

	// Define flags
	rootCmd.Flags().String("mode", "stdio", "Server mode: 'stdio' (default) or 'http' (env: WEBEX_MODE)")
	rootCmd.Flags().String("access-token", "", "Webex API access token (env: WEBEX_ACCESS_TOKEN). Required for stdio mode.")
	rootCmd.Flags().String("base-url", "https://webexapis.com/v1", "Webex API base URL (env: WEBEX_BASE_URL)")
	rootCmd.Flags().Duration("timeout", 30*time.Second, "HTTP request timeout (env: WEBEX_TIMEOUT)")
	rootCmd.Flags().String("include", "", "Comma-separated list of tools to include (category:action format, e.g. messages:list,meetings:create). Only these tools will be registered. (env: WEBEX_INCLUDE_TOOLS)")
	rootCmd.Flags().String("exclude", "", "Comma-separated list of tools to exclude (category:action format, e.g. messages:delete,rooms:delete). All tools except these will be registered. (env: WEBEX_EXCLUDE_TOOLS)")
	rootCmd.Flags().Bool("minimal", false, "Enable a minimal tool set: messages, rooms, teams, meetings, and transcripts. Adds to --include. (env: WEBEX_MINIMAL)")
	rootCmd.Flags().Bool("readonly-minimal", false, "Enable a readonly minimal tool set: only read/list/get operations for messages, rooms, teams, meetings, and transcripts. Adds to --include. (env: WEBEX_READONLY_MINIMAL)")

	// HTTP mode flags
	rootCmd.Flags().String("host", "localhost", "HTTP server bind host (env: WEBEX_HOST)")
	rootCmd.Flags().Int("port", 8080, "HTTP server port (env: WEBEX_PORT)")
	rootCmd.Flags().String("client-id", "", "Webex Integration Client ID (env: WEBEX_CLIENT_ID). Required for http mode.")
	rootCmd.Flags().String("client-secret", "", "Webex Integration Client Secret (env: WEBEX_CLIENT_SECRET). Required for http mode.")
	rootCmd.Flags().String("oauth-scopes", "spark:all", "Webex OAuth scopes (space-separated) (env: WEBEX_OAUTH_SCOPES)")
	rootCmd.Flags().String("redirect-uri", "", "OAuth redirect URI registered with Webex (env: WEBEX_REDIRECT_URI). Required for http mode.")
	rootCmd.Flags().String("server-url", "", "External base URL of this server (env: WEBEX_SERVER_URL). Required for http mode. Example: http://localhost:8080")
	rootCmd.Flags().String("tls-cert", "", "Path to TLS certificate file (env: WEBEX_TLS_CERT)")
	rootCmd.Flags().String("tls-key", "", "Path to TLS key file (env: WEBEX_TLS_KEY)")

	// Bind flags to viper
	_ = viper.BindPFlag("mode", rootCmd.Flags().Lookup("mode"))
	_ = viper.BindPFlag("access_token", rootCmd.Flags().Lookup("access-token"))
	_ = viper.BindPFlag("base_url", rootCmd.Flags().Lookup("base-url"))
	_ = viper.BindPFlag("timeout", rootCmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("include_tools", rootCmd.Flags().Lookup("include"))
	_ = viper.BindPFlag("exclude_tools", rootCmd.Flags().Lookup("exclude"))
	_ = viper.BindPFlag("minimal", rootCmd.Flags().Lookup("minimal"))
	_ = viper.BindPFlag("readonly_minimal", rootCmd.Flags().Lookup("readonly-minimal"))
	_ = viper.BindPFlag("host", rootCmd.Flags().Lookup("host"))
	_ = viper.BindPFlag("port", rootCmd.Flags().Lookup("port"))
	_ = viper.BindPFlag("client_id", rootCmd.Flags().Lookup("client-id"))
	_ = viper.BindPFlag("client_secret", rootCmd.Flags().Lookup("client-secret"))
	_ = viper.BindPFlag("oauth_scopes", rootCmd.Flags().Lookup("oauth-scopes"))
	_ = viper.BindPFlag("redirect_uri", rootCmd.Flags().Lookup("redirect-uri"))
	_ = viper.BindPFlag("server_url", rootCmd.Flags().Lookup("server-url"))
	_ = viper.BindPFlag("tls_cert", rootCmd.Flags().Lookup("tls-cert"))
	_ = viper.BindPFlag("tls_key", rootCmd.Flags().Lookup("tls-key"))

	// Bind environment variables
	viper.SetEnvPrefix("WEBEX")
	_ = viper.BindEnv("mode", "WEBEX_MODE")
	_ = viper.BindEnv("access_token", "WEBEX_ACCESS_TOKEN")
	_ = viper.BindEnv("base_url", "WEBEX_BASE_URL")
	_ = viper.BindEnv("timeout", "WEBEX_TIMEOUT")
	_ = viper.BindEnv("include_tools", "WEBEX_INCLUDE_TOOLS")
	_ = viper.BindEnv("exclude_tools", "WEBEX_EXCLUDE_TOOLS")
	_ = viper.BindEnv("minimal", "WEBEX_MINIMAL")
	_ = viper.BindEnv("readonly_minimal", "WEBEX_READONLY_MINIMAL")
	_ = viper.BindEnv("host", "WEBEX_HOST")
	_ = viper.BindEnv("port", "WEBEX_PORT")
	_ = viper.BindEnv("client_id", "WEBEX_CLIENT_ID")
	_ = viper.BindEnv("client_secret", "WEBEX_CLIENT_SECRET")
	_ = viper.BindEnv("oauth_scopes", "WEBEX_OAUTH_SCOPES")
	_ = viper.BindEnv("redirect_uri", "WEBEX_REDIRECT_URI")
	_ = viper.BindEnv("server_url", "WEBEX_SERVER_URL")
	_ = viper.BindEnv("tls_cert", "WEBEX_TLS_CERT")
	_ = viper.BindEnv("tls_key", "WEBEX_TLS_KEY")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Redirect log output to stderr so it doesn't interfere with STDIO MCP transport
	log.SetOutput(os.Stderr)

	mode := viper.GetString("mode")
	baseURL := viper.GetString("base_url")
	timeout := viper.GetDuration("timeout")

	// Tool filtering (shared between modes)
	includeTools := viper.GetString("include_tools")
	excludeTools := viper.GetString("exclude_tools")
	minimal := viper.GetBool("minimal")
	readonlyMinimal := viper.GetBool("readonly_minimal")

	sdkConfig := &webexsdk.Config{
		BaseURL: baseURL,
		Timeout: timeout,
	}

	switch mode {
	case "stdio":
		return runSTDIO(sdkConfig, includeTools, excludeTools, minimal, readonlyMinimal)
	case "http":
		return runHTTP(sdkConfig, includeTools, excludeTools, minimal, readonlyMinimal)
	default:
		return fmt.Errorf("invalid mode %q: must be 'stdio' or 'http'", mode)
	}
}

func runSTDIO(sdkConfig *webexsdk.Config, include, exclude string, minimal, readonlyMinimal bool) error {
	accessToken := viper.GetString("access_token")
	if accessToken == "" {
		return fmt.Errorf("WEBEX_ACCESS_TOKEN environment variable or --access-token flag is required in stdio mode")
	}

	webexClient, err := webex.NewClient(accessToken, sdkConfig)
	if err != nil {
		return fmt.Errorf("failed to create Webex client: %w", err)
	}

	resolver := auth.NewStaticClientResolver(webexClient)

	log.Printf("Starting Webex MCP Server v%s in STDIO mode (base_url=%s, timeout=%s)", version, sdkConfig.BaseURL, sdkConfig.Timeout)
	return startSTDIOServer(resolver, include, exclude, minimal, readonlyMinimal)
}

func runHTTP(sdkConfig *webexsdk.Config, include, exclude string, minimal, readonlyMinimal bool) error {
	clientID := viper.GetString("client_id")
	clientSecret := viper.GetString("client_secret")
	oauthScopes := viper.GetString("oauth_scopes")
	redirectURI := viper.GetString("redirect_uri")
	serverURL := viper.GetString("server_url")
	host := viper.GetString("host")
	port := viper.GetInt("port")
	tlsCert := viper.GetString("tls_cert")
	tlsKey := viper.GetString("tls_key")

	// Validate required HTTP mode config
	if clientID == "" {
		return fmt.Errorf("WEBEX_CLIENT_ID or --client-id is required in http mode")
	}
	if clientSecret == "" {
		return fmt.Errorf("WEBEX_CLIENT_SECRET or --client-secret is required in http mode")
	}
	if redirectURI == "" {
		return fmt.Errorf("WEBEX_REDIRECT_URI or --redirect-uri is required in http mode")
	}
	if serverURL == "" {
		// Default to http://host:port
		scheme := "http"
		if tlsCert != "" {
			scheme = "https"
		}
		serverURL = fmt.Sprintf("%s://%s:%d", scheme, host, port)
	}

	log.Printf("Starting Webex MCP Server v%s in HTTP mode (server_url=%s)", version, serverURL)

	return startHTTPServer(&HTTPServerConfig{
		Host:    host,
		Port:    port,
		TLSCert: tlsCert,
		TLSKey:  tlsKey,
		OAuthConfig: &auth.OAuthConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       oauthScopes,
			RedirectURI:  redirectURI,
			ServerURL:    serverURL,
		},
		WebexSDKConfig:  sdkConfig,
		Include:         include,
		Exclude:         exclude,
		Minimal:         minimal,
		ReadonlyMinimal: readonlyMinimal,
	})
}

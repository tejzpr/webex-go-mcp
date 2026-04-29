package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	webex "github.com/WebexCommunity/webex-go-sdk/v2"
	"github.com/WebexCommunity/webex-go-sdk/v2/webexsdk"
	"github.com/tejzpr/webex-go-mcp/auth"

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
	rootCmd.Flags().String("access-token", "", "Webex API access token (env: WEBEX_ACCESS_TOKEN). Required for stdio mode; used for static-token http mode or bot sends in hybrid http mode.")
	rootCmd.Flags().String("webex-api-base-url", "https://webexapis.com/v1", "Webex API base URL (env: WEBEX_API_BASE_URL)")
	rootCmd.Flags().Duration("timeout", 30*time.Second, "HTTP request timeout (env: WEBEX_TIMEOUT)")
	rootCmd.Flags().String("include", "", "Comma-separated list of tools to include (category:action format, e.g. messages:list,meetings:create). Only these tools will be registered. (env: WEBEX_INCLUDE_TOOLS)")
	rootCmd.Flags().String("exclude", "", "Comma-separated list of tools to exclude (category:action format, e.g. messages:delete,rooms:delete). All tools except these will be registered. (env: WEBEX_EXCLUDE_TOOLS)")
	rootCmd.Flags().Bool("minimal", false, "Enable a minimal tool set: messages, rooms, teams, meetings, and transcripts. Adds to --include. (env: WEBEX_MINIMAL)")
	rootCmd.Flags().Bool("readonly-minimal", false, "Enable a readonly minimal tool set: only read/list/get operations for messages, rooms, teams, meetings, and transcripts. Adds to --include. (env: WEBEX_READONLY_MINIMAL)")
	rootCmd.Flags().Bool("shared-env-minimal", false, "Enable a shared-environment-safe minimal tool set: person lookup and outbound message tools only. Adds to --include. (env: WEBEX_SHARED_ENV_MINIMAL)")
	rootCmd.Flags().Bool("enable-mcp-elicitation", false, "Require MCP elicitation approval before mutating Webex tools run. Fails closed when the client does not support elicitation. (env: WEBEX_ENABLE_MCP_ELICITATION)")

	// HTTP mode flags
	rootCmd.Flags().String("host", "localhost", "HTTP server bind host (env: WEBEX_HOST)")
	rootCmd.Flags().Int("port", 8080, "HTTP server port (env: WEBEX_PORT)")
	rootCmd.Flags().String("client-id", "", "Webex Integration Client ID (env: WEBEX_CLIENT_ID). Required for OAuth http mode.")
	rootCmd.Flags().String("client-secret", "", "Webex Integration Client Secret (env: WEBEX_CLIENT_SECRET). Required for OAuth http mode.")
	rootCmd.Flags().String("oauth-scopes", "spark:all", "Webex OAuth scopes (space-separated) (env: WEBEX_OAUTH_SCOPES)")
	rootCmd.Flags().String("redirect-uri", "", "OAuth redirect URI registered with Webex (env: WEBEX_REDIRECT_URI). Required for OAuth http mode.")
	rootCmd.Flags().String("base-url", "", "External base URL of this MCP server (env: WEBEX_BASE_URL). Required for http mode. Example: http://localhost:8080")
	rootCmd.Flags().String("auth-api-key", "", "Optional API key required on HTTP MCP requests via X-API-Key (env: WEBEX_AUTH_API_KEY)")
	rootCmd.Flags().String("tls-cert", "", "Path to TLS certificate file (env: WEBEX_TLS_CERT)")
	rootCmd.Flags().String("tls-key", "", "Path to TLS key file (env: WEBEX_TLS_KEY)")
	rootCmd.Flags().String("store", "memory", "Store backend: 'memory' (default), 'sqlite', or 'postgres' (env: WEBEX_STORE)")
	rootCmd.Flags().String("store-dsn", "", "Store DSN for sqlite/postgres (env: WEBEX_STORE_DSN). SQLite: 'file:data.db', Postgres: 'postgres://user:pass@host:5432/db'")
	rootCmd.Flags().String("cors-origins", "*", "Comma-separated list of allowed CORS origins (env: WEBEX_CORS_ORIGINS). Default '*' allows all.")

	// Bind flags to viper
	_ = viper.BindPFlag("mode", rootCmd.Flags().Lookup("mode"))
	_ = viper.BindPFlag("access_token", rootCmd.Flags().Lookup("access-token"))
	_ = viper.BindPFlag("webex_api_base_url", rootCmd.Flags().Lookup("webex-api-base-url"))
	_ = viper.BindPFlag("timeout", rootCmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("include_tools", rootCmd.Flags().Lookup("include"))
	_ = viper.BindPFlag("exclude_tools", rootCmd.Flags().Lookup("exclude"))
	_ = viper.BindPFlag("minimal", rootCmd.Flags().Lookup("minimal"))
	_ = viper.BindPFlag("readonly_minimal", rootCmd.Flags().Lookup("readonly-minimal"))
	_ = viper.BindPFlag("shared_env_minimal", rootCmd.Flags().Lookup("shared-env-minimal"))
	_ = viper.BindPFlag("enable_mcp_elicitation", rootCmd.Flags().Lookup("enable-mcp-elicitation"))
	_ = viper.BindPFlag("host", rootCmd.Flags().Lookup("host"))
	_ = viper.BindPFlag("port", rootCmd.Flags().Lookup("port"))
	_ = viper.BindPFlag("client_id", rootCmd.Flags().Lookup("client-id"))
	_ = viper.BindPFlag("client_secret", rootCmd.Flags().Lookup("client-secret"))
	_ = viper.BindPFlag("oauth_scopes", rootCmd.Flags().Lookup("oauth-scopes"))
	_ = viper.BindPFlag("redirect_uri", rootCmd.Flags().Lookup("redirect-uri"))
	_ = viper.BindPFlag("base_url", rootCmd.Flags().Lookup("base-url"))
	_ = viper.BindPFlag("auth_api_key", rootCmd.Flags().Lookup("auth-api-key"))
	_ = viper.BindPFlag("tls_cert", rootCmd.Flags().Lookup("tls-cert"))
	_ = viper.BindPFlag("tls_key", rootCmd.Flags().Lookup("tls-key"))
	_ = viper.BindPFlag("store", rootCmd.Flags().Lookup("store"))
	_ = viper.BindPFlag("store_dsn", rootCmd.Flags().Lookup("store-dsn"))
	_ = viper.BindPFlag("cors_origins", rootCmd.Flags().Lookup("cors-origins"))

	// Bind environment variables
	viper.SetEnvPrefix("WEBEX")
	_ = viper.BindEnv("mode", "WEBEX_MODE")
	_ = viper.BindEnv("access_token", "WEBEX_ACCESS_TOKEN")
	_ = viper.BindEnv("webex_api_base_url", "WEBEX_API_BASE_URL")
	_ = viper.BindEnv("timeout", "WEBEX_TIMEOUT")
	_ = viper.BindEnv("include_tools", "WEBEX_INCLUDE_TOOLS")
	_ = viper.BindEnv("exclude_tools", "WEBEX_EXCLUDE_TOOLS")
	_ = viper.BindEnv("minimal", "WEBEX_MINIMAL")
	_ = viper.BindEnv("readonly_minimal", "WEBEX_READONLY_MINIMAL")
	_ = viper.BindEnv("shared_env_minimal", "WEBEX_SHARED_ENV_MINIMAL")
	_ = viper.BindEnv("enable_mcp_elicitation", "WEBEX_ENABLE_MCP_ELICITATION")
	_ = viper.BindEnv("host", "WEBEX_HOST")
	_ = viper.BindEnv("port", "WEBEX_PORT")
	_ = viper.BindEnv("client_id", "WEBEX_CLIENT_ID")
	_ = viper.BindEnv("client_secret", "WEBEX_CLIENT_SECRET")
	_ = viper.BindEnv("oauth_scopes", "WEBEX_OAUTH_SCOPES")
	_ = viper.BindEnv("redirect_uri", "WEBEX_REDIRECT_URI")
	_ = viper.BindEnv("base_url", "WEBEX_BASE_URL")
	_ = viper.BindEnv("auth_api_key", "WEBEX_AUTH_API_KEY")
	_ = viper.BindEnv("tls_cert", "WEBEX_TLS_CERT")
	_ = viper.BindEnv("tls_key", "WEBEX_TLS_KEY")
	_ = viper.BindEnv("store", "WEBEX_STORE")
	_ = viper.BindEnv("store_dsn", "WEBEX_STORE_DSN")
	_ = viper.BindEnv("cors_origins", "WEBEX_CORS_ORIGINS")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Redirect log output to stderr so it doesn't interfere with STDIO MCP transport
	log.SetOutput(os.Stderr)

	mode := viper.GetString("mode")
	webexAPIBaseURL := viper.GetString("webex_api_base_url")
	timeout := viper.GetDuration("timeout")

	// Tool filtering (shared between modes)
	includeTools := viper.GetString("include_tools")
	excludeTools := viper.GetString("exclude_tools")
	minimal := viper.GetBool("minimal")
	readonlyMinimal := viper.GetBool("readonly_minimal")
	sharedEnvMinimal := viper.GetBool("shared_env_minimal")
	enableMCPElicitation := viper.GetBool("enable_mcp_elicitation")

	sdkConfig := &webexsdk.Config{
		BaseURL: webexAPIBaseURL,
		Timeout: timeout,
	}

	switch mode {
	case "stdio":
		return runSTDIO(sdkConfig, includeTools, excludeTools, minimal, readonlyMinimal, sharedEnvMinimal, enableMCPElicitation)
	case "http":
		return runHTTP(sdkConfig, includeTools, excludeTools, minimal, readonlyMinimal, sharedEnvMinimal, enableMCPElicitation)
	default:
		return fmt.Errorf("invalid mode %q: must be 'stdio' or 'http'", mode)
	}
}

func runSTDIO(sdkConfig *webexsdk.Config, include, exclude string, minimal, readonlyMinimal, sharedEnvMinimal, enableMCPElicitation bool) error {
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
	return startSTDIOServer(resolver, include, exclude, minimal, readonlyMinimal, sharedEnvMinimal, enableMCPElicitation)
}

func normalizeHTTPBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("value is empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("scheme must be http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("host is required")
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func runHTTP(sdkConfig *webexsdk.Config, include, exclude string, minimal, readonlyMinimal, sharedEnvMinimal, enableMCPElicitation bool) error {
	accessToken := viper.GetString("access_token")
	clientID := viper.GetString("client_id")
	clientSecret := viper.GetString("client_secret")
	oauthScopes := viper.GetString("oauth_scopes")
	redirectURI := viper.GetString("redirect_uri")
	baseURL := viper.GetString("base_url")
	authAPIKey := viper.GetString("auth_api_key")
	host := viper.GetString("host")
	port := viper.GetInt("port")
	storeType := viper.GetString("store")
	storeDSN := viper.GetString("store_dsn")
	corsOrigins := viper.GetString("cors_origins")
	tlsCert := viper.GetString("tls_cert")
	tlsKey := viper.GetString("tls_key")

	if baseURL == "" {
		return fmt.Errorf("WEBEX_BASE_URL or --base-url is required in http mode (example: http://localhost:%d)", port)
	}
	baseURL, err := normalizeHTTPBaseURL(baseURL)
	if err != nil {
		return fmt.Errorf("invalid WEBEX_BASE_URL or --base-url: %w", err)
	}

	var oauthConfig *auth.OAuthConfig
	var staticResolver auth.ClientResolver
	var botSendResolver auth.ClientResolver
	if clientID != "" || clientSecret != "" {
		if clientID == "" {
			return fmt.Errorf("WEBEX_CLIENT_ID or --client-id is required when using OAuth in http mode")
		}
		if clientSecret == "" {
			return fmt.Errorf("WEBEX_CLIENT_SECRET or --client-secret is required when using OAuth in http mode")
		}
		if redirectURI == "" {
			return fmt.Errorf("WEBEX_REDIRECT_URI or --redirect-uri is required when using OAuth in http mode")
		}
		oauthConfig = &auth.OAuthConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       oauthScopes,
			RedirectURI:  redirectURI,
			ServerURL:    baseURL,
		}
		if accessToken != "" {
			botClient, err := webex.NewClient(accessToken, sdkConfig)
			if err != nil {
				return fmt.Errorf("failed to create bot Webex client from WEBEX_ACCESS_TOKEN for hybrid HTTP mode: %w", err)
			}
			botSendResolver = auth.NewStaticClientResolver(botClient)
		}
	} else {
		if accessToken == "" {
			return fmt.Errorf("either WEBEX_CLIENT_ID + WEBEX_CLIENT_SECRET for OAuth, or WEBEX_ACCESS_TOKEN for static-token HTTP mode, is required")
		}
		webexClient, err := webex.NewClient(accessToken, sdkConfig)
		if err != nil {
			return fmt.Errorf("failed to create Webex client from WEBEX_ACCESS_TOKEN: %w", err)
		}
		staticResolver = auth.NewStaticClientResolver(webexClient)
	}

	if botSendResolver != nil {
		log.Printf("Starting Webex MCP Server v%s in HTTP HYBRID mode (base_url=%s): OAuth user authentication enabled; default message sends use WEBEX_ACCESS_TOKEN bot identity", version, baseURL)
	} else {
		log.Printf("Starting Webex MCP Server v%s in HTTP mode (base_url=%s)", version, baseURL)
	}

	return startHTTPServer(&HTTPServerConfig{
		Host:            host,
		Port:            port,
		TLSCert:         tlsCert,
		TLSKey:          tlsKey,
		BaseURL:         baseURL,
		AuthAPIKey:      authAPIKey,
		OAuthConfig:     oauthConfig,
		StaticResolver:  staticResolver,
		BotSendResolver: botSendResolver,
		WebexSDKConfig:  sdkConfig,
		StoreConfig: auth.StoreConfig{
			Type: storeType,
			DSN:  storeDSN,
		},
		Include:              include,
		Exclude:              exclude,
		Minimal:              minimal,
		ReadonlyMinimal:      readonlyMinimal,
		SharedEnvMinimal:     sharedEnvMinimal,
		EnableMCPElicitation: enableMCPElicitation,
		CORSOrigins:          corsOrigins,
	})
}

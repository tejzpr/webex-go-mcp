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
	"github.com/tejzpr/webex-go-mcp/streaming"
	"github.com/tejzpr/webex-go-mcp/tools"

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
	rootCmd.PersistentFlags().String("mode", "stdio", "Server mode: 'stdio' (default) or 'http' (env: WEBEX_MODE)")
	rootCmd.PersistentFlags().String("access-token", "", "Webex API access token (env: WEBEX_ACCESS_TOKEN). Used for static-token stdio/http mode, or for bot sends in hybrid mode when a Webex Integration is also configured.")
	rootCmd.PersistentFlags().String("webex-api-base-url", "https://webexapis.com/v1", "Webex API base URL (env: WEBEX_API_BASE_URL)")
	rootCmd.PersistentFlags().Duration("timeout", 30*time.Second, "HTTP request timeout (env: WEBEX_TIMEOUT)")
	rootCmd.PersistentFlags().String("include", "", "Comma-separated list of tools to include (category:action format, e.g. messages:list,meetings:create). Only these tools will be registered. (env: WEBEX_INCLUDE_TOOLS)")
	rootCmd.PersistentFlags().String("exclude", "", "Comma-separated list of tools to exclude (category:action format, e.g. messages:delete,rooms:delete). All tools except these will be registered. (env: WEBEX_EXCLUDE_TOOLS)")
	rootCmd.PersistentFlags().Bool("minimal", false, "Enable a minimal tool set: messages, rooms, teams, meetings, and transcripts. Adds to --include. (env: WEBEX_MINIMAL)")
	rootCmd.PersistentFlags().Bool("readonly-minimal", false, "Enable a readonly minimal tool set: only read/list/get operations for messages, rooms, teams, meetings, and transcripts. Adds to --include. (env: WEBEX_READONLY_MINIMAL)")
	rootCmd.PersistentFlags().Bool("shared-env-minimal", false, "Enable a shared-environment-safe minimal tool set: person lookup and outbound message tools only. Adds to --include. (env: WEBEX_SHARED_ENV_MINIMAL)")
	rootCmd.PersistentFlags().Bool("enable-mcp-elicitation", false, "Require MCP elicitation approval before mutating Webex tools run. Fails closed when the client does not support elicitation. (env: WEBEX_ENABLE_MCP_ELICITATION)")
	rootCmd.PersistentFlags().String("streaming-ignore-from-emails", "", "Comma-separated sender email addresses to drop from Mercury streaming notifications. Use this to suppress messages sent by the bot itself. (env: WEBEX_STREAMING_IGNORE_FROM_EMAILS)")

	// HTTP mode flags
	rootCmd.PersistentFlags().String("host", "localhost", "HTTP server bind host (env: WEBEX_HOST)")
	rootCmd.PersistentFlags().Int("port", 8080, "HTTP server port (env: WEBEX_PORT)")
	rootCmd.PersistentFlags().String("client-id", "", "Webex Integration Client ID (env: WEBEX_CLIENT_ID). Enables OAuth in http mode and browser sign-in in stdio mode.")
	rootCmd.PersistentFlags().String("client-secret", "", "Webex Integration Client Secret (env: WEBEX_CLIENT_SECRET). Required with --client-id.")
	rootCmd.PersistentFlags().String("oauth-scopes", "spark:all", "Webex OAuth scopes (space-separated) (env: WEBEX_OAUTH_SCOPES)")
	rootCmd.PersistentFlags().String("redirect-uri", "", "OAuth redirect URI registered with Webex (env: WEBEX_REDIRECT_URI). Required for OAuth http mode. In stdio mode it must be a loopback URL and defaults to "+auth.DefaultLocalRedirectURI+".")
	rootCmd.PersistentFlags().String("base-url", "", "External base URL of this MCP server (env: WEBEX_BASE_URL). Required for http mode. Example: http://localhost:8080")
	rootCmd.PersistentFlags().String("auth-api-key", "", "Optional API key required on HTTP MCP requests via X-API-Key (env: WEBEX_AUTH_API_KEY)")
	rootCmd.PersistentFlags().String("tls-cert", "", "Path to TLS certificate file (env: WEBEX_TLS_CERT)")
	rootCmd.PersistentFlags().String("tls-key", "", "Path to TLS key file (env: WEBEX_TLS_KEY)")
	rootCmd.PersistentFlags().String("store", "memory", "Store backend: 'memory' (default), 'sqlite', or 'postgres' (env: WEBEX_STORE)")
	rootCmd.PersistentFlags().String("store-dsn", "", "Store DSN for sqlite/postgres (env: WEBEX_STORE_DSN). SQLite: 'file:data.db', Postgres: 'postgres://user:pass@host:5432/db'")
	// STDIO OAuth (Webex Integration) flags
	rootCmd.PersistentFlags().String("token-file", "", "Path of the stdio-mode OAuth token file (env: WEBEX_TOKEN_FILE). Default: <user config dir>/webex-go-mcp/oauth-token.json")
	rootCmd.PersistentFlags().Bool("oauth-no-browser", false, "In stdio OAuth mode, do not open a browser automatically; only print/return the sign-in URL (env: WEBEX_OAUTH_NO_BROWSER)")
	rootCmd.PersistentFlags().Duration("oauth-login-wait", 60*time.Second, "In stdio OAuth mode, how long a tool call waits for an interactive browser sign-in before returning the sign-in URL. Negative disables waiting. (env: WEBEX_OAUTH_LOGIN_WAIT)")

	rootCmd.PersistentFlags().String("cors-origins", "*", "Comma-separated list of allowed CORS origins (env: WEBEX_CORS_ORIGINS). Default '*' allows all.")

	// Bind flags to viper
	_ = viper.BindPFlag("mode", rootCmd.PersistentFlags().Lookup("mode"))
	_ = viper.BindPFlag("access_token", rootCmd.PersistentFlags().Lookup("access-token"))
	_ = viper.BindPFlag("webex_api_base_url", rootCmd.PersistentFlags().Lookup("webex-api-base-url"))
	_ = viper.BindPFlag("timeout", rootCmd.PersistentFlags().Lookup("timeout"))
	_ = viper.BindPFlag("include_tools", rootCmd.PersistentFlags().Lookup("include"))
	_ = viper.BindPFlag("exclude_tools", rootCmd.PersistentFlags().Lookup("exclude"))
	_ = viper.BindPFlag("minimal", rootCmd.PersistentFlags().Lookup("minimal"))
	_ = viper.BindPFlag("readonly_minimal", rootCmd.PersistentFlags().Lookup("readonly-minimal"))
	_ = viper.BindPFlag("shared_env_minimal", rootCmd.PersistentFlags().Lookup("shared-env-minimal"))
	_ = viper.BindPFlag("enable_mcp_elicitation", rootCmd.PersistentFlags().Lookup("enable-mcp-elicitation"))
	_ = viper.BindPFlag("streaming_ignore_from_emails", rootCmd.PersistentFlags().Lookup("streaming-ignore-from-emails"))
	_ = viper.BindPFlag("host", rootCmd.PersistentFlags().Lookup("host"))
	_ = viper.BindPFlag("port", rootCmd.PersistentFlags().Lookup("port"))
	_ = viper.BindPFlag("client_id", rootCmd.PersistentFlags().Lookup("client-id"))
	_ = viper.BindPFlag("client_secret", rootCmd.PersistentFlags().Lookup("client-secret"))
	_ = viper.BindPFlag("oauth_scopes", rootCmd.PersistentFlags().Lookup("oauth-scopes"))
	_ = viper.BindPFlag("redirect_uri", rootCmd.PersistentFlags().Lookup("redirect-uri"))
	_ = viper.BindPFlag("base_url", rootCmd.PersistentFlags().Lookup("base-url"))
	_ = viper.BindPFlag("auth_api_key", rootCmd.PersistentFlags().Lookup("auth-api-key"))
	_ = viper.BindPFlag("tls_cert", rootCmd.PersistentFlags().Lookup("tls-cert"))
	_ = viper.BindPFlag("tls_key", rootCmd.PersistentFlags().Lookup("tls-key"))
	_ = viper.BindPFlag("store", rootCmd.PersistentFlags().Lookup("store"))
	_ = viper.BindPFlag("store_dsn", rootCmd.PersistentFlags().Lookup("store-dsn"))
	_ = viper.BindPFlag("cors_origins", rootCmd.PersistentFlags().Lookup("cors-origins"))
	_ = viper.BindPFlag("token_file", rootCmd.PersistentFlags().Lookup("token-file"))
	_ = viper.BindPFlag("oauth_no_browser", rootCmd.PersistentFlags().Lookup("oauth-no-browser"))
	_ = viper.BindPFlag("oauth_login_wait", rootCmd.PersistentFlags().Lookup("oauth-login-wait"))

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
	_ = viper.BindEnv("streaming_ignore_from_emails", "WEBEX_STREAMING_IGNORE_FROM_EMAILS", "WEBEX_STREAMING_IGNORE_FROM_EMAIL")
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
	_ = viper.BindEnv("token_file", "WEBEX_TOKEN_FILE")
	_ = viper.BindEnv("oauth_no_browser", "WEBEX_OAUTH_NO_BROWSER")
	_ = viper.BindEnv("oauth_login_wait", "WEBEX_OAUTH_LOGIN_WAIT")

	rootCmd.AddCommand(newAuthCommand())
	rootCmd.AddCommand(newLoginCommand("login"))
	rootCmd.AddCommand(newLogoutCommand("logout"))

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
	streamingIgnoreFromEmails := streaming.ParseIgnoredSenderEmails(viper.GetString("streaming_ignore_from_emails"))

	sdkConfig := &webexsdk.Config{
		BaseURL: webexAPIBaseURL,
		Timeout: timeout,
	}

	switch mode {
	case "stdio":
		return runSTDIO(sdkConfig, includeTools, excludeTools, minimal, readonlyMinimal, sharedEnvMinimal, enableMCPElicitation, streamingIgnoreFromEmails)
	case "http":
		return runHTTP(sdkConfig, includeTools, excludeTools, minimal, readonlyMinimal, sharedEnvMinimal, enableMCPElicitation, streamingIgnoreFromEmails)
	default:
		return fmt.Errorf("invalid mode %q: must be 'stdio' or 'http'", mode)
	}
}

func runSTDIO(sdkConfig *webexsdk.Config, include, exclude string, minimal, readonlyMinimal, sharedEnvMinimal, enableMCPElicitation bool, streamingIgnoreFromEmails []string) error {
	accessToken := viper.GetString("access_token")
	clientID := viper.GetString("client_id")
	clientSecret := viper.GetString("client_secret")

	// Static access token only (original behavior).
	if clientID == "" && clientSecret == "" {
		if accessToken == "" {
			return fmt.Errorf("stdio mode needs either WEBEX_ACCESS_TOKEN (--access-token), or a Webex Integration via WEBEX_CLIENT_ID + WEBEX_CLIENT_SECRET (--client-id/--client-secret) for browser sign-in")
		}

		webexClient, err := webex.NewClient(accessToken, sdkConfig)
		if err != nil {
			return fmt.Errorf("failed to create Webex client: %w", err)
		}

		resolver := auth.NewStaticClientResolver(webexClient)

		log.Printf("Starting Webex MCP Server v%s in STDIO mode (base_url=%s, timeout=%s)", version, sdkConfig.BaseURL, sdkConfig.Timeout)
		return startSTDIOServer(resolver, include, exclude, minimal, readonlyMinimal, sharedEnvMinimal, enableMCPElicitation, streamingIgnoreFromEmails, nil)
	}

	// Webex Integration (OAuth) via local browser sign-in.
	mgr, err := newLocalOAuthManager(sdkConfig)
	if err != nil {
		return err
	}
	resolver := mgr.Resolver()

	var messageOptions []tools.MessageToolOptions
	if accessToken != "" {
		botClient, err := webex.NewClient(accessToken, sdkConfig)
		if err != nil {
			return fmt.Errorf("failed to create bot Webex client from WEBEX_ACCESS_TOKEN for hybrid STDIO mode: %w", err)
		}
		messageOptions = append(messageOptions, tools.MessageToolOptions{
			AllowLocalFilePath: true,
			SendResolver:       auth.NewStaticClientResolver(botClient),
			LoggedInUserSender: resolver,
		})
		log.Printf("Starting Webex MCP Server v%s in STDIO HYBRID mode (base_url=%s): OAuth user sign-in enabled; default message sends use WEBEX_ACCESS_TOKEN bot identity", version, sdkConfig.BaseURL)
	} else {
		log.Printf("Starting Webex MCP Server v%s in STDIO OAuth mode (base_url=%s, redirect_uri=%s)", version, sdkConfig.BaseURL, mgr.RedirectURI())
	}

	if mgr.HasUsableToken() {
		log.Printf("Using stored Webex OAuth token from %s", mgr.TokenFile())
	} else {
		log.Printf("No stored Webex OAuth token at %s. Sign-in will open in your browser on the first Webex tool call (or run `webex-go-mcp login` beforehand).", mgr.TokenFile())
	}

	return startSTDIOServer(resolver, include, exclude, minimal, readonlyMinimal, sharedEnvMinimal, enableMCPElicitation, streamingIgnoreFromEmails, mgr, messageOptions...)
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

func runHTTP(sdkConfig *webexsdk.Config, include, exclude string, minimal, readonlyMinimal, sharedEnvMinimal, enableMCPElicitation bool, streamingIgnoreFromEmails []string) error {
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
		Include:                   include,
		Exclude:                   exclude,
		Minimal:                   minimal,
		ReadonlyMinimal:           readonlyMinimal,
		SharedEnvMinimal:          sharedEnvMinimal,
		EnableMCPElicitation:      enableMCPElicitation,
		CORSOrigins:               corsOrigins,
		StreamingIgnoreFromEmails: streamingIgnoreFromEmails,
	})
}

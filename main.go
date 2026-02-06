package main

import (
	"fmt"
	"log"
	"os"
	"time"

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
		Short:   "Webex MCP Server - STDIO MCP server for Webex APIs",
		Version: version,
		RunE:    run,
	}

	// Define flags
	rootCmd.Flags().String("access-token", "", "Webex API access token (env: WEBEX_ACCESS_TOKEN)")
	rootCmd.Flags().String("base-url", "https://webexapis.com/v1", "Webex API base URL (env: WEBEX_BASE_URL)")
	rootCmd.Flags().Duration("timeout", 30*time.Second, "HTTP request timeout (env: WEBEX_TIMEOUT)")

	// Bind flags to viper
	_ = viper.BindPFlag("access_token", rootCmd.Flags().Lookup("access-token"))
	_ = viper.BindPFlag("base_url", rootCmd.Flags().Lookup("base-url"))
	_ = viper.BindPFlag("timeout", rootCmd.Flags().Lookup("timeout"))

	// Bind environment variables
	viper.SetEnvPrefix("WEBEX")
	_ = viper.BindEnv("access_token", "WEBEX_ACCESS_TOKEN")
	_ = viper.BindEnv("base_url", "WEBEX_BASE_URL")
	_ = viper.BindEnv("timeout", "WEBEX_TIMEOUT")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	accessToken := viper.GetString("access_token")
	if accessToken == "" {
		return fmt.Errorf("WEBEX_ACCESS_TOKEN environment variable or --access-token flag is required")
	}

	baseURL := viper.GetString("base_url")
	timeout := viper.GetDuration("timeout")

	// Create Webex SDK client
	config := &webexsdk.Config{
		BaseURL: baseURL,
		Timeout: timeout,
	}

	webexClient, err := webex.NewClient(accessToken, config)
	if err != nil {
		return fmt.Errorf("failed to create Webex client: %w", err)
	}

	// Redirect log output to stderr so it doesn't interfere with STDIO MCP transport
	log.SetOutput(os.Stderr)
	log.Printf("Starting Webex MCP Server v%s (base_url=%s, timeout=%s)", version, baseURL, timeout)

	// Start the MCP server
	return startServer(webexClient)
}

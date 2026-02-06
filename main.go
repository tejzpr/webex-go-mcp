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
	rootCmd.Flags().String("include", "", "Comma-separated list of tools to include (category:action format, e.g. messages:list,meetings:create). Only these tools will be registered. (env: WEBEX_INCLUDE_TOOLS)")
	rootCmd.Flags().String("exclude", "", "Comma-separated list of tools to exclude (category:action format, e.g. messages:delete,rooms:delete). All tools except these will be registered. (env: WEBEX_EXCLUDE_TOOLS)")
	rootCmd.Flags().Bool("minimal", false, "Enable a minimal tool set: messages, rooms, teams, meetings, and transcripts. Adds to --include. (env: WEBEX_MINIMAL)")
	rootCmd.Flags().Bool("readonly-minimal", false, "Enable a readonly minimal tool set: only read/list/get operations for messages, rooms, teams, meetings, and transcripts. Adds to --include. (env: WEBEX_READONLY_MINIMAL)")

	// Bind flags to viper
	_ = viper.BindPFlag("access_token", rootCmd.Flags().Lookup("access-token"))
	_ = viper.BindPFlag("base_url", rootCmd.Flags().Lookup("base-url"))
	_ = viper.BindPFlag("timeout", rootCmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("include_tools", rootCmd.Flags().Lookup("include"))
	_ = viper.BindPFlag("exclude_tools", rootCmd.Flags().Lookup("exclude"))
	_ = viper.BindPFlag("minimal", rootCmd.Flags().Lookup("minimal"))
	_ = viper.BindPFlag("readonly_minimal", rootCmd.Flags().Lookup("readonly-minimal"))

	// Bind environment variables
	viper.SetEnvPrefix("WEBEX")
	_ = viper.BindEnv("access_token", "WEBEX_ACCESS_TOKEN")
	_ = viper.BindEnv("base_url", "WEBEX_BASE_URL")
	_ = viper.BindEnv("timeout", "WEBEX_TIMEOUT")
	_ = viper.BindEnv("include_tools", "WEBEX_INCLUDE_TOOLS")
	_ = viper.BindEnv("exclude_tools", "WEBEX_EXCLUDE_TOOLS")
	_ = viper.BindEnv("minimal", "WEBEX_MINIMAL")
	_ = viper.BindEnv("readonly_minimal", "WEBEX_READONLY_MINIMAL")

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

	// Tool filtering
	includeTools := viper.GetString("include_tools")
	excludeTools := viper.GetString("exclude_tools")
	minimal := viper.GetBool("minimal")
	readonlyMinimal := viper.GetBool("readonly_minimal")

	// Start the MCP server
	return startServer(webexClient, includeTools, excludeTools, minimal, readonlyMinimal)
}

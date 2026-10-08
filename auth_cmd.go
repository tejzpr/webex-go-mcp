package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/WebexCommunity/webex-go-sdk/v2/webexsdk"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tejzpr/webex-go-mcp/auth"
)

// sdkConfigFromViper builds the Webex SDK config from flags/env.
func sdkConfigFromViper() *webexsdk.Config {
	return &webexsdk.Config{
		BaseURL: viper.GetString("webex_api_base_url"),
		Timeout: viper.GetDuration("timeout"),
	}
}

// newLocalOAuthManager builds the STDIO-mode Webex Integration sign-in manager
// from flags/env. Requires WEBEX_CLIENT_ID and WEBEX_CLIENT_SECRET.
func newLocalOAuthManager(sdkConfig *webexsdk.Config) (*auth.LocalOAuthManager, error) {
	clientID := viper.GetString("client_id")
	clientSecret := viper.GetString("client_secret")
	if clientID == "" {
		return nil, fmt.Errorf("WEBEX_CLIENT_ID or --client-id is required for Webex Integration sign-in")
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("WEBEX_CLIENT_SECRET or --client-secret is required for Webex Integration sign-in")
	}

	redirectURI := viper.GetString("redirect_uri")
	if redirectURI == "" {
		redirectURI = auth.DefaultLocalRedirectURI
	}

	mgr, err := auth.NewLocalOAuthManager(auth.LocalOAuthConfig{
		OAuth: &auth.OAuthConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       viper.GetString("oauth_scopes"),
			RedirectURI:  redirectURI,
		},
		TokenFile:   viper.GetString("token_file"),
		SDKConfig:   sdkConfig,
		OpenBrowser: !viper.GetBool("oauth_no_browser"),
		LoginWait:   viper.GetDuration("oauth_login_wait"),
	})
	if err != nil {
		return nil, fmt.Errorf("invalid stdio OAuth configuration: %w", err)
	}
	return mgr, nil
}

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Webex Integration sign-in for stdio mode",
		Long: "Manage the locally stored Webex OAuth token used by stdio mode when a Webex Integration " +
			"(WEBEX_CLIENT_ID + WEBEX_CLIENT_SECRET) is configured.\n\n" +
			"The Integration's redirect URI must include the loopback URL used here " +
			"(default " + auth.DefaultLocalRedirectURI + ").",
	}
	cmd.AddCommand(newLoginCommand("login"))
	cmd.AddCommand(newLogoutCommand("logout"))
	cmd.AddCommand(newAuthStatusCommand())
	return cmd
}

func newLoginCommand(use string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: "Sign in to Webex in your browser and store the token for stdio mode",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetOutput(os.Stderr)
			mgr, err := newLocalOAuthManager(sdkConfigFromViper())
			if err != nil {
				return err
			}
			defer mgr.Close()

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			if err := mgr.Login(ctx, cmd.ErrOrStderr()); err != nil {
				return fmt.Errorf("sign-in failed: %w", err)
			}

			if client, err := mgr.ClientIfAuthenticated(); err == nil {
				if me, err := client.People().GetMe(); err == nil && me != nil {
					email := ""
					if len(me.Emails) > 0 {
						email = me.Emails[0]
					}
					fmt.Fprintf(cmd.OutOrStdout(), "Signed in as %s <%s>\n", me.DisplayName, email)
				}
			}
			// Give the browser a moment to receive the success page before exiting.
			time.Sleep(500 * time.Millisecond)
			return nil
		},
	}
}

func newLogoutCommand(use string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: "Delete the stored stdio-mode Webex OAuth token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetOutput(os.Stderr)
			mgr, err := newLocalOAuthManager(sdkConfigFromViper())
			if err != nil {
				return err
			}
			if err := mgr.Logout(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Signed out. Removed %s\n", mgr.TokenFile())
			return nil
		},
	}
}

func newAuthStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the stored stdio-mode Webex OAuth token status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetOutput(os.Stderr)
			mgr, err := newLocalOAuthManager(sdkConfigFromViper())
			if err != nil {
				return err
			}
			result := map[string]interface{}{"status": mgr.Status()}
			if mgr.HasUsableToken() {
				if client, err := mgr.ClientIfAuthenticated(); err == nil {
					if me, err := client.People().GetMe(); err == nil && me != nil {
						result["user"] = map[string]interface{}{
							"id":          me.ID,
							"displayName": me.DisplayName,
							"emails":      me.Emails,
						}
					} else if err != nil {
						result["userLookupError"] = err.Error()
					}
				} else {
					result["userLookupError"] = err.Error()
				}
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}
}

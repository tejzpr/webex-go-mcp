package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
)

const maxAuthLoginWaitSeconds = 300

// RegisterLocalAuthTools registers STDIO-mode Webex Integration sign-in tools.
// They are only registered when STDIO mode is configured with a Webex
// Integration (client ID + secret) and are not subject to tool filtering,
// because without them the user has no way to recover from a missing token.
func RegisterLocalAuthTools(s ToolRegistrar, mgr *auth.LocalOAuthManager) {
	if mgr == nil {
		return
	}

	s.AddTool(
		mcp.NewTool("webex_auth_status",
			mcp.WithDescription("Show the Webex sign-in status for this MCP server (STDIO + Webex Integration mode): whether a user is signed in, who it is, token expiry, and any sign-in link that is waiting to be completed.\n"+
				"\n"+
				"Use this when a Webex tool reports 'Webex sign-in required', or after the user says they finished signing in."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			st := mgr.Status()
			result := map[string]interface{}{"status": st}
			if st.Authenticated {
				// Never triggers a browser sign-in; only uses an existing/refreshable token.
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
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("webex_auth_login",
			mcp.WithDescription("Start a Webex sign-in in the user's browser (STDIO + Webex Integration mode).\n"+
				"\n"+
				"The server opens the browser to the Webex consent page and listens on its local redirect URI. The tool waits up to waitSeconds for the user to finish; if they haven't, it returns a loginUrl. Show that URL to the user, ask them to complete sign-in, then call webex_auth_status or retry the original request.\n"+
				"\n"+
				"If a user is already signed in this returns their status unless force=true (use force to switch accounts or re-consent to new scopes)."),
			mcp.WithBoolean("force", mcp.Description("Start a new sign-in even if a valid token is already stored. Default false.")),
			mcp.WithNumber("waitSeconds", mcp.Description(fmt.Sprintf("How long to wait for the user to finish signing in before returning (0-%d). Default 60.", maxAuthLoginWaitSeconds))),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			force := req.GetBool("force", false)
			wait := req.GetInt("waitSeconds", 60)
			if wait < 0 {
				wait = 0
			}
			if wait > maxAuthLoginWaitSeconds {
				wait = maxAuthLoginWaitSeconds
			}

			if !force && mgr.HasUsableToken() {
				data, _ := json.MarshalIndent(map[string]interface{}{
					"message": "Already signed in to Webex. Pass force=true to sign in again.",
					"status":  mgr.Status(),
				}, "", "  ")
				return mcp.NewToolResultText(string(data)), nil
			}

			loginURL, _, err := mgr.StartLogin()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to start Webex sign-in: %v", err)), nil
			}

			if wait > 0 {
				if err := mgr.WaitForLogin(ctx, time.Duration(wait)*time.Second); err == nil {
					data, _ := json.MarshalIndent(map[string]interface{}{
						"message": "Signed in to Webex.",
						"status":  mgr.Status(),
					}, "", "  ")
					return mcp.NewToolResultText(string(data)), nil
				} else if ctx.Err() == nil && !isLoginStillPending(mgr) {
					return mcp.NewToolResultError(fmt.Sprintf("Webex sign-in did not complete: %v. Call webex_auth_login again to retry.", err)), nil
				}
			}

			data, _ := json.MarshalIndent(map[string]interface{}{
				"message":  "Waiting for the user to sign in. A browser window should have opened; if not, ask the user to open loginUrl. After they finish, call webex_auth_status or retry the original request.",
				"loginUrl": loginURL,
				"status":   mgr.Status(),
			}, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("webex_auth_logout",
			mcp.WithDescription("Sign out of Webex for this MCP server (STDIO + Webex Integration mode): deletes the locally stored OAuth token and cancels any pending sign-in. The next Webex tool call will require signing in again. Confirm with the user before calling."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if err := mgr.Logout(); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to sign out: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Signed out of Webex. Removed stored token at %s.", mgr.TokenFile())), nil
		},
	)
}

func isLoginStillPending(mgr *auth.LocalOAuthManager) bool {
	return mgr.Status().LoginInProgress
}

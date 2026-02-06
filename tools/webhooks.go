package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/webhooks"
)

// RegisterWebhookTools registers all webhook-related MCP tools.
func RegisterWebhookTools(s *server.MCPServer, client *webex.WebexClient) {
	// webex_webhooks_list
	s.AddTool(
		mcp.NewTool("webex_webhooks_list",
			mcp.WithDescription("List all Webex webhooks registered by the authenticated user."),
			mcp.WithNumber("max", mcp.Description("Maximum number of webhooks to return")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			opts := &webhooks.ListOptions{}

			if v := req.GetInt("max", 0); v > 0 {
				opts.Max = v
			}

			page, err := client.Webhooks().List(opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list webhooks: %v", err)), nil
			}

			data, _ := json.MarshalIndent(page.Items, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_webhooks_create
	s.AddTool(
		mcp.NewTool("webex_webhooks_create",
			mcp.WithDescription("Create a new Webex webhook to receive real-time notifications."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Name of the webhook")),
			mcp.WithString("targetUrl", mcp.Required(), mcp.Description("URL to receive webhook notifications")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("Resource to watch: 'messages', 'memberships', 'rooms', 'meetings', 'recordings', 'meetingParticipants', 'meetingTranscripts', 'attachmentActions'")),
			mcp.WithString("event", mcp.Required(), mcp.Description("Event to watch: 'created', 'updated', 'deleted', 'started', 'ended', 'joined', 'left'")),
			mcp.WithString("filter", mcp.Description("Filter expression (e.g., 'roomId=xxx' or 'personEmail=xxx')")),
			mcp.WithString("secret", mcp.Description("Secret used to generate webhook payload signature for verification")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			targetURL, err := req.RequireString("targetUrl")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			resource, err := req.RequireString("resource")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			event, err := req.RequireString("event")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			webhook := &webhooks.Webhook{
				Name:      name,
				TargetURL: targetURL,
				Resource:  resource,
				Event:     event,
				Filter:    req.GetString("filter", ""),
				Secret:    req.GetString("secret", ""),
			}

			result, err := client.Webhooks().Create(webhook)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create webhook: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_webhooks_get
	s.AddTool(
		mcp.NewTool("webex_webhooks_get",
			mcp.WithDescription("Get details of a specific Webex webhook by its ID."),
			mcp.WithString("webhookId", mcp.Required(), mcp.Description("The ID of the webhook to retrieve")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			webhookID, err := req.RequireString("webhookId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			result, err := client.Webhooks().Get(webhookID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get webhook: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_webhooks_update
	s.AddTool(
		mcp.NewTool("webex_webhooks_update",
			mcp.WithDescription("Update an existing Webex webhook."),
			mcp.WithString("webhookId", mcp.Required(), mcp.Description("The ID of the webhook to update")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Updated name of the webhook")),
			mcp.WithString("targetUrl", mcp.Required(), mcp.Description("Updated target URL")),
			mcp.WithString("secret", mcp.Description("Updated secret for payload verification")),
			mcp.WithString("status", mcp.Description("Webhook status: 'active' or 'inactive'")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			webhookID, err := req.RequireString("webhookId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			targetURL, err := req.RequireString("targetUrl")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			webhook := webhooks.NewUpdateWebhook(
				name,
				targetURL,
				req.GetString("secret", ""),
				req.GetString("status", ""),
			)

			result, err := client.Webhooks().Update(webhookID, webhook)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to update webhook: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_webhooks_delete
	s.AddTool(
		mcp.NewTool("webex_webhooks_delete",
			mcp.WithDescription("Delete a Webex webhook by its ID."),
			mcp.WithString("webhookId", mcp.Required(), mcp.Description("The ID of the webhook to delete")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			webhookID, err := req.RequireString("webhookId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			err = client.Webhooks().Delete(webhookID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to delete webhook: %v", err)), nil
			}

			return mcp.NewToolResultText("Webhook deleted successfully"), nil
		},
	)
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
	"github.com/WebexCommunity/webex-go-sdk/v2/webhooks"
)

// RegisterWebhookTools registers all webhook-related MCP tools.
func RegisterWebhookTools(s ToolRegistrar, resolver auth.ClientResolver) {
	// webex_webhooks_list
	s.AddTool(
		mcp.NewTool("webex_webhooks_list",
			mcp.WithDescription("List all Webex webhooks registered by the authenticated user. A webhook is a callback URL that Webex notifies when specific events happen (e.g. new message, meeting started, membership changed).\n"+
				"\n"+
				"RESPONSE: Each webhook shows its name, targetUrl, resource, event, filter, status (active/inactive), and creation date."),
			mcp.WithNumber("max", mcp.Description("Maximum number of webhooks to return.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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
			mcp.WithDescription("Create a new Webex webhook to receive real-time event notifications at a URL. Webex will POST a JSON payload to targetUrl whenever the specified event occurs on the specified resource.\n"+
				"\n"+
				"COMMON COMBINATIONS:\n"+
				"- New messages in a room: resource='messages', event='created', filter='roomId=ROOM_ID'\n"+
				"- New messages mentioning me: resource='messages', event='created', filter='mentionedPeople=me'\n"+
				"- All new messages: resource='messages', event='created' (no filter)\n"+
				"- Someone joins a room: resource='memberships', event='created', filter='roomId=ROOM_ID'\n"+
				"- Meeting started: resource='meetings', event='started'\n"+
				"- Meeting ended: resource='meetings', event='ended'\n"+
				"- New recording available: resource='recordings', event='created'\n"+
				"- New transcript available: resource='meetingTranscripts', event='created'\n"+
				"\n"+
				"IMPORTANT: The targetUrl must be a publicly accessible HTTPS URL that can receive POST requests."),
			mcp.WithString("name", mcp.Required(), mcp.Description("A friendly name for this webhook (e.g. 'New messages in Project Alpha', 'Meeting notifications').")),
			mcp.WithString("targetUrl", mcp.Required(), mcp.Description("The HTTPS URL where Webex will POST event notifications. Must be publicly accessible.")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("The Webex resource to monitor. Options: 'messages', 'memberships', 'rooms', 'meetings', 'recordings', 'meetingParticipants', 'meetingTranscripts', 'attachmentActions'.")),
			mcp.WithString("event", mcp.Required(), mcp.Description("The event type to trigger on. Options depend on resource: 'created', 'updated', 'deleted' (for messages/memberships/rooms), 'started', 'ended' (for meetings), 'joined', 'left' (for meetingParticipants).")),
			mcp.WithString("filter", mcp.Description("Optional filter to narrow events. Examples: 'roomId=ROOM_ID' (only events in that room), 'mentionedPeople=me' (only messages mentioning you), 'personEmail=alice@example.com' (only events involving that person).")),
			mcp.WithString("secret", mcp.Description("Optional secret string. Webex uses it to sign the webhook payload (HMAC-SHA1 in X-Spark-Signature header) so your server can verify the request is authentic.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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
			mcp.WithDescription("Get full details of a specific webhook by its ID. Returns name, targetUrl, resource, event, filter, status, and creation/update timestamps."),
			mcp.WithString("webhookId", mcp.Required(), mcp.Description("The ID of the webhook. Get this from webex_webhooks_list.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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
			mcp.WithDescription("Update an existing webhook -- change its name, target URL, secret, or enable/disable it.\n"+
				"\n"+
				"TIP: Set status='inactive' to temporarily pause a webhook without deleting it. Set status='active' to re-enable."),
			mcp.WithString("webhookId", mcp.Required(), mcp.Description("The ID of the webhook to update. Get this from webex_webhooks_list.")),
			mcp.WithString("name", mcp.Required(), mcp.Description("The webhook name (pass existing name if not changing).")),
			mcp.WithString("targetUrl", mcp.Required(), mcp.Description("The target URL (pass existing URL if not changing). Must be HTTPS.")),
			mcp.WithString("secret", mcp.Description("Updated secret for payload signature verification. Pass empty string to remove the secret.")),
			mcp.WithString("status", mcp.Description("Set to 'active' to enable or 'inactive' to disable the webhook without deleting it.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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
			mcp.WithDescription("Permanently delete a webhook. Webex will stop sending notifications to the target URL immediately.\n"+
				"\n"+
				"TIP: If you just want to temporarily stop notifications, use webex_webhooks_update with status='inactive' instead.\n"+
				"\n"+
				"IMPORTANT: Confirm with the user before deleting."),
			mcp.WithString("webhookId", mcp.Required(), mcp.Description("The ID of the webhook to delete. Get this from webex_webhooks_list.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tejzpr/webex-go-mcp/auth"
	"github.com/tejzpr/webex-go-mcp/streaming"
)

// RegisterStreamingTools registers Mercury-based streaming MCP tools.
func RegisterStreamingTools(s ToolRegistrar, resolver auth.ClientResolver, manager *streaming.MercuryManager) {
	// subscribe_room_messages — opens a Mercury listener for a room
	s.AddTool(
		mcp.NewTool("webex_subscribe_room_messages",
			mcp.WithDescription("Subscribe to real-time messages in a Webex room via Mercury WebSocket. "+
				"Returns immediately with a subscriptionId. Events are streamed as MCP notifications. "+
				"Use webex_unsubscribe to stop. Requires HTTP mode with OAuth authentication."),
			mcp.WithString("roomId",
				mcp.Required(),
				mcp.Description("The ID of the room to subscribe to. Messages in this room will be streamed as notifications.")),
			mcp.WithString("eventTypes",
				mcp.Description("Comma-separated event types to listen for. Default: 'post,share'. "+
					"Options: post, share, acknowledge.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			roomID := req.GetString("roomId", "")
			if roomID == "" {
				return mcp.NewToolResultError("roomId is required"), nil
			}

			// Parse event types
			eventTypesStr := req.GetString("eventTypes", "post,share")
			eventTypes := parseCSV(eventTypesStr)

			// Get the access token from context (HTTP mode) or from the client (STDIO mode)
			accessToken, ok := auth.WebexTokenFromContext(ctx)
			if !ok || accessToken == "" {
				accessToken = client.Core().GetAccessToken()
			}
			if accessToken == "" {
				return mcp.NewToolResultError("No access token available for Mercury connection."), nil
			}

			sub, err := manager.Subscribe(ctx, client, accessToken, roomID, eventTypes)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to subscribe: %v", err)), nil
			}

			result := map[string]interface{}{
				"subscriptionId": sub.ID,
				"roomId":         sub.RoomID,
				"status":         "listening",
				"message":        "Subscription active. Events will be streamed as MCP notifications. Use webex_unsubscribe to stop.",
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// unsubscribe — cancels a Mercury subscription
	s.AddTool(
		mcp.NewTool("webex_unsubscribe",
			mcp.WithDescription("Cancel a Mercury event subscription created by webex_subscribe_room_messages. "+
				"Stops streaming events for the given subscription."),
			mcp.WithString("subscriptionId",
				mcp.Required(),
				mcp.Description("The subscription ID returned by webex_subscribe_room_messages.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			subID := req.GetString("subscriptionId", "")
			if subID == "" {
				return mcp.NewToolResultError("subscriptionId is required"), nil
			}

			if err := manager.Unsubscribe(subID); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to unsubscribe: %v", err)), nil
			}

			result := map[string]interface{}{
				"subscriptionId": subID,
				"status":         "cancelled",
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// wait_for_next_message — blocks until a message arrives or timeout
	s.AddTool(
		mcp.NewTool("webex_wait_for_message",
			mcp.WithDescription("Wait for the next message in a Webex room. Blocks until a message arrives or timeout. "+
				"Simpler alternative to subscribe_room_messages for one-shot use cases. "+
				"Requires HTTP mode with OAuth authentication."),
			mcp.WithString("roomId",
				mcp.Required(),
				mcp.Description("The ID of the room to wait for a message in.")),
			mcp.WithNumber("timeoutSeconds",
				mcp.Description("Maximum time to wait in seconds. Default: 60. Max: 300.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			roomID := req.GetString("roomId", "")
			if roomID == "" {
				return mcp.NewToolResultError("roomId is required"), nil
			}

			timeoutSec := req.GetInt("timeoutSeconds", 60)
			if timeoutSec > 300 {
				timeoutSec = 300
			}
			if timeoutSec < 1 {
				timeoutSec = 1
			}
			timeout := time.Duration(timeoutSec) * time.Second

			// Get the access token from context (HTTP mode) or from the client (STDIO mode)
			accessToken, ok := auth.WebexTokenFromContext(ctx)
			if !ok || accessToken == "" {
				accessToken = client.Core().GetAccessToken()
			}
			if accessToken == "" {
				return mcp.NewToolResultError("No access token available for Mercury connection."), nil
			}

			msg, err := manager.WaitForMessage(ctx, client, accessToken, roomID, timeout)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Error waiting for message: %v", err)), nil
			}

			data, _ := json.MarshalIndent(msg, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// list_subscriptions — lists active subscriptions
	s.AddTool(
		mcp.NewTool("webex_list_subscriptions",
			mcp.WithDescription("List all active Mercury event subscriptions for the current session."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Try to get session ID for filtering
			sessionID := ""
			if session := extractSessionID(ctx); session != "" {
				sessionID = session
			}

			subs := manager.ListSubscriptions(sessionID)

			items := make([]map[string]interface{}, 0, len(subs))
			for _, sub := range subs {
				items = append(items, map[string]interface{}{
					"subscriptionId": sub.ID,
					"roomId":         sub.RoomID,
					"createdAt":      sub.CreatedAt.Format(time.RFC3339),
				})
			}

			result := map[string]interface{}{
				"subscriptions": items,
				"count":         len(items),
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

// parseCSV splits a comma-separated string into trimmed non-empty parts.
func parseCSV(s string) []string {
	var result []string
	for _, part := range splitAndTrim(s, ',') {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// splitAndTrim splits a string by sep and trims whitespace from each part.
func splitAndTrim(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			part := trimSpace(s[start:i])
			parts = append(parts, part)
			start = i + 1
		}
	}
	part := trimSpace(s[start:])
	parts = append(parts, part)
	return parts
}

// trimSpace trims leading and trailing whitespace.
func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// extractSessionID tries to get the MCP session ID from context.
func extractSessionID(ctx context.Context) string {
	if session := mcpserver.ClientSessionFromContext(ctx); session != nil {
		return session.SessionID()
	}
	return ""
}

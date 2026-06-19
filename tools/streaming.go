package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	webex "github.com/WebexCommunity/webex-go-sdk/v2"
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
			accessToken, ok := accessTokenFromCtxOrClient(ctx, client)
			if !ok {
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

	// subscribe_mentions — opens a Mercury listener for @mentions and direct messages
	s.AddTool(
		mcp.NewTool("webex_subscribe_mentions",
			mcp.WithDescription("Subscribe to real-time messages that @mention a specific email address or are sent as direct messages (1:1) to that user. "+
				"Listens to all Mercury events across all rooms and filters for:\n"+
				"1. Messages containing @mentions via Webex mention syntax (<@personEmail:user@example.com|Name>)\n"+
				"2. Messages containing @mentions via person ID (<@personId:...|Name>)\n"+
				"3. Messages containing @all mentions (notifies everyone in the room)\n"+
				"4. Direct messages (1:1 rooms) sent to the target user (when includeDirect is true)\n\n"+
				"Returns immediately with a subscriptionId. Matching events are streamed as MCP notifications with matchType='mention', 'mention_all', or 'direct_message'. "+
				"Use webex_unsubscribe to stop. Requires HTTP mode with OAuth authentication."),
			mcp.WithString("email",
				mcp.Required(),
				mcp.Description("The email address to monitor for @mentions and direct messages. "+
					"The tool will resolve this to a Webex person ID for matching.")),
			mcp.WithBoolean("includeDirect",
				mcp.Description("Whether to also include direct messages (1:1) sent to the target user. Default: true.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			email := req.GetString("email", "")
			if email == "" {
				return mcp.NewToolResultError("email is required"), nil
			}

			includeDirect := req.GetBool("includeDirect", true)

			// Get the access token from context (HTTP mode) or from the client (STDIO mode)
			accessToken, ok := accessTokenFromCtxOrClient(ctx, client)
			if !ok {
				return mcp.NewToolResultError("No access token available for Mercury connection."), nil
			}

			sub, err := manager.SubscribeMentions(ctx, client, accessToken, email, includeDirect)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to subscribe: %v", err)), nil
			}

			result := map[string]interface{}{
				"subscriptionId": sub.ID,
				"email":          sub.Email,
				"personId":       sub.PersonID,
				"includeDirect":  includeDirect,
				"status":         "listening",
				"message":        "Mention subscription active. Events will be streamed as MCP notifications with matchType='mention' or 'direct_message'. Use webex_unsubscribe to stop.",
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// subscribe_direct_messages — opens a Mercury listener for 1:1 DMs only
	s.AddTool(
		mcp.NewTool("webex_subscribe_direct_messages",
			mcp.WithDescription("Subscribe to real-time direct messages (1:1 conversations) sent to a specific email address. "+
				"This is the simplest way for an AI to have a live conversation with a user via Webex DMs.\n\n"+
				"Unlike webex_subscribe_mentions, this tool ONLY delivers messages from 1:1 rooms — "+
				"no @mentions, no @all, no group room messages. It filters purely for direct conversations.\n\n"+
				"Returns immediately with a subscriptionId. Incoming DMs are streamed as MCP notifications with matchType='direct_message'. "+
				"The AI can then respond using webex_messages_create with toPersonEmail.\n\n"+
				"Use webex_unsubscribe to stop. Requires HTTP mode with OAuth authentication."),
			mcp.WithString("email",
				mcp.Required(),
				mcp.Description("The email address to listen for direct messages. Messages from 1:1 conversations with this user will be streamed.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			email := req.GetString("email", "")
			if email == "" {
				return mcp.NewToolResultError("email is required"), nil
			}

			// Get the access token from context (HTTP mode) or from the client (STDIO mode)
			accessToken, ok := accessTokenFromCtxOrClient(ctx, client)
			if !ok {
				return mcp.NewToolResultError("No access token available for Mercury connection."), nil
			}

			sub, err := manager.SubscribeDirectMessages(ctx, client, accessToken, email)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to subscribe: %v", err)), nil
			}

			result := map[string]interface{}{
				"subscriptionId": sub.ID,
				"email":          sub.Email,
				"status":         "listening",
				"message":        "DM subscription active. Direct messages will be streamed as MCP notifications. Respond using webex_messages_create with toPersonEmail. Use webex_unsubscribe to stop.",
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// unsubscribe — cancels a Mercury subscription
	s.AddTool(
		mcp.NewTool("webex_unsubscribe",
			mcp.WithDescription("Cancel a Mercury event subscription created by webex_subscribe_room_messages, webex_subscribe_mentions, or webex_subscribe_direct_messages. "+
				"Stops streaming events for the given subscription."),
			mcp.WithString("subscriptionId",
				mcp.Required(),
				mcp.Description("The subscription ID returned by webex_subscribe_room_messages or webex_subscribe_mentions.")),
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
			accessToken, ok := accessTokenFromCtxOrClient(ctx, client)
			if !ok {
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

	// subscribe_messages_from_person — streams messages sent BY a given person, scoped by rooms + mentions
	s.AddTool(
		mcp.NewTool("webex_subscribe_messages_from_person",
			mcp.WithDescription("Subscribe to real-time messages SENT BY a specific person (by email), with optional room scoping and a mentions filter. "+
				"personEmail is always the sender. Behavior by parameters:\n"+
				"- rooms empty, mentionsOnly false: direct (1:1) messages from the person.\n"+
				"- rooms empty, mentionsOnly true: messages from the person in ANY room that mention you (the authenticated user) or @all.\n"+
				"- rooms set, mentionsOnly false: messages from the person in any of those rooms.\n"+
				"- rooms set, mentionsOnly true: messages from the person in those rooms that mention you or @all.\n\n"+
				"Matching events are streamed as MCP notifications with matchType='from_person', 'from_person_direct', 'from_person_mention', or 'from_person_mention_all'. "+
				"Use webex_unsubscribe to stop. Requires HTTP mode with OAuth authentication."),
			mcp.WithString("personEmail",
				mcp.Required(),
				mcp.Description("The email address of the sender to listen for.")),
			mcp.WithArray("rooms",
				mcp.WithStringItems(),
				mcp.Description("Room IDs to scope to. If empty, listens to direct (1:1) messages only — unless mentionsOnly is true, in which case it listens across all rooms.")),
			mcp.WithBoolean("mentionsOnly",
				mcp.Description("When true, only deliver messages that mention you (the authenticated user) or @all. Default: false.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			personEmail := req.GetString("personEmail", "")
			if personEmail == "" {
				return mcp.NewToolResultError("personEmail is required"), nil
			}
			rooms := req.GetStringSlice("rooms", nil)
			mentionsOnly := req.GetBool("mentionsOnly", false)

			accessToken, ok := accessTokenFromCtxOrClient(ctx, client)
			if !ok {
				return mcp.NewToolResultError("No access token available for Mercury connection."), nil
			}

			sub, err := manager.SubscribeMessagesFromPerson(ctx, client, accessToken, personEmail, rooms, mentionsOnly)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to subscribe: %v", err)), nil
			}

			result := map[string]interface{}{
				"subscriptionId": sub.ID,
				"personEmail":    sub.Email,
				"rooms":          sub.Rooms,
				"mentionsOnly":   sub.MentionsOnly,
				"status":         "listening",
				"message":        "From-person subscription active. Events will be streamed as MCP notifications. Use webex_unsubscribe to stop.",
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// wait_for_message_in_room_from_person — blocks until a message from a given person arrives or timeout
	s.AddTool(
		mcp.NewTool("webex_wait_for_message_in_room_from_person",
			mcp.WithDescription("Wait for the next message SENT BY a specific person (by email), with optional room scoping and a mentions filter. "+
				"Blocks until a matching message arrives or timeout. One-shot alternative to webex_subscribe_messages_from_person. "+
				"personEmail is always the sender. Behavior by parameters:\n"+
				"- rooms empty, mentionsOnly false: direct (1:1) messages from the person.\n"+
				"- rooms empty, mentionsOnly true: messages from the person in ANY room that mention you (the authenticated user) or @all.\n"+
				"- rooms set, mentionsOnly false: messages from the person in any of those rooms.\n"+
				"- rooms set, mentionsOnly true: messages from the person in those rooms that mention you or @all.\n\n"+
				"Requires HTTP mode with OAuth authentication."),
			mcp.WithString("personEmail",
				mcp.Required(),
				mcp.Description("The email address of the sender to wait for.")),
			mcp.WithArray("rooms",
				mcp.WithStringItems(),
				mcp.Description("Room IDs to scope to. If empty, waits for direct (1:1) messages only — unless mentionsOnly is true, in which case it waits across all rooms.")),
			mcp.WithBoolean("mentionsOnly",
				mcp.Description("When true, only match messages that mention you (the authenticated user) or @all. Default: false.")),
			mcp.WithNumber("timeoutSeconds",
				mcp.Description("Maximum time to wait in seconds. Default: 60. Max: 300.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			personEmail := req.GetString("personEmail", "")
			if personEmail == "" {
				return mcp.NewToolResultError("personEmail is required"), nil
			}
			rooms := req.GetStringSlice("rooms", nil)
			mentionsOnly := req.GetBool("mentionsOnly", false)

			timeoutSec := req.GetInt("timeoutSeconds", 60)
			if timeoutSec > 300 {
				timeoutSec = 300
			}
			if timeoutSec < 1 {
				timeoutSec = 1
			}
			timeout := time.Duration(timeoutSec) * time.Second

			accessToken, ok := accessTokenFromCtxOrClient(ctx, client)
			if !ok {
				return mcp.NewToolResultError("No access token available for Mercury connection."), nil
			}

			msg, err := manager.WaitForMessageFromPerson(ctx, client, accessToken, personEmail, rooms, mentionsOnly, timeout)
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
				item := map[string]interface{}{
					"subscriptionId": sub.ID,
					"createdAt":      sub.CreatedAt.Format(time.RFC3339),
				}
				if strings.HasPrefix(sub.ID, "fpsub_") {
					item["type"] = "from_person"
					item["personEmail"] = sub.Email
					item["rooms"] = sub.Rooms
					item["mentionsOnly"] = sub.MentionsOnly
				} else if strings.HasPrefix(sub.ID, "dmsub_") {
					item["type"] = "direct_messages"
					item["email"] = sub.Email
				} else if sub.Email != "" {
					item["type"] = "mentions"
					item["email"] = sub.Email
					item["personId"] = sub.PersonID
				} else {
					item["type"] = "room"
					item["roomId"] = sub.RoomID
				}
				items = append(items, item)
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

// accessTokenFromCtxOrClient returns the Webex access token for a Mercury
// connection: the per-request token from context (HTTP mode), falling back to
// the client's configured token (STDIO mode). The bool is false when neither
// yields a non-empty token.
func accessTokenFromCtxOrClient(ctx context.Context, client *webex.WebexClient) (string, bool) {
	if token, ok := auth.WebexTokenFromContext(ctx); ok && token != "" {
		return token, true
	}
	if token := client.Core().GetAccessToken(); token != "" {
		return token, true
	}
	return "", false
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

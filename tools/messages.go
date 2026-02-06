package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/messages"
)

// RegisterMessageTools registers all message-related MCP tools.
func RegisterMessageTools(s *server.MCPServer, client *webex.WebexClient) {
	// webex_messages_list
	s.AddTool(
		mcp.NewTool("webex_messages_list",
			mcp.WithDescription("List messages in a Webex room/space. Returns the latest 20 messages by default. Use 'beforeMessage' with the ID of the oldest message from the previous result to paginate further back."),
			mcp.WithString("roomId", mcp.Required(), mcp.Description("The ID of the room to list messages from")),
			mcp.WithString("mentionedPeople", mcp.Description("Filter by mentioned people (use 'me' for current user)")),
			mcp.WithString("before", mcp.Description("List messages before this date/time (ISO 8601)")),
			mcp.WithString("beforeMessage", mcp.Description("List messages before this message ID")),
			mcp.WithNumber("max", mcp.Description("Maximum number of messages to return (default 50, max 1000)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			roomID, err := req.RequireString("roomId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &messages.ListOptions{
				RoomID: roomID,
			}

			if v := req.GetString("mentionedPeople", ""); v != "" {
				opts.MentionedPeople = v
			}
			if v := req.GetString("before", ""); v != "" {
				opts.Before = v
			}
			if v := req.GetString("beforeMessage", ""); v != "" {
				opts.BeforeMessage = v
			}
			opts.Max = req.GetInt("max", 20)

			page, err := client.Messages().List(opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list messages: %v", err)), nil
			}

			data, _ := json.MarshalIndent(page.Items, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_messages_create
	s.AddTool(
		mcp.NewTool("webex_messages_create",
			mcp.WithDescription("Send a message to a Webex room/space or person. Provide roomId, or toPersonId, or toPersonEmail. Always confirm with the user before sending a message, unless the user has explicitly requested to not confirm."),
			mcp.WithString("roomId", mcp.Description("The room ID to send the message to")),
			mcp.WithString("toPersonId", mcp.Description("The person ID to send a 1:1 message to")),
			mcp.WithString("toPersonEmail", mcp.Description("The email to send a 1:1 message to")),
			mcp.WithString("text", mcp.Description("Plain text message content")),
			mcp.WithString("markdown", mcp.Description("Markdown formatted message content")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msg := &messages.Message{
				RoomID:        req.GetString("roomId", ""),
				ToPersonID:    req.GetString("toPersonId", ""),
				ToPersonEmail: req.GetString("toPersonEmail", ""),
				Text:          req.GetString("text", ""),
				Markdown:      req.GetString("markdown", ""),
			}

			if msg.RoomID == "" && msg.ToPersonID == "" && msg.ToPersonEmail == "" {
				return mcp.NewToolResultError("One of roomId, toPersonId, or toPersonEmail is required"), nil
			}
			if msg.Text == "" && msg.Markdown == "" {
				return mcp.NewToolResultError("Either text or markdown content is required"), nil
			}

			result, err := client.Messages().Create(msg)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create message: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_messages_get
	s.AddTool(
		mcp.NewTool("webex_messages_get",
			mcp.WithDescription("Get details of a specific Webex message by its ID."),
			mcp.WithString("messageId", mcp.Required(), mcp.Description("The ID of the message to retrieve")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			messageID, err := req.RequireString("messageId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			result, err := client.Messages().Get(messageID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get message: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_messages_delete
	s.AddTool(
		mcp.NewTool("webex_messages_delete",
			mcp.WithDescription("Delete a Webex message by its ID. Always confirm with the user before deleting a message, unless the user has explicitly requested to not confirm."),
			mcp.WithString("messageId", mcp.Required(), mcp.Description("The ID of the message to delete")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			messageID, err := req.RequireString("messageId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			err = client.Messages().Delete(messageID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to delete message: %v", err)), nil
			}

			return mcp.NewToolResultText("Message deleted successfully"), nil
		},
	)
}

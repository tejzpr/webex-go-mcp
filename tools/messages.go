package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/messages"
)

// RegisterMessageTools registers all message-related MCP tools.
func RegisterMessageTools(s ToolRegistrar, client *webex.WebexClient) {
	// webex_messages_list
	s.AddTool(
		mcp.NewTool("webex_messages_list",
			mcp.WithDescription("List messages in a Webex room/space. Returns the latest 20 messages by default. Use 'beforeMessage' with the ID of the oldest message from the previous result to paginate further back. Response is enriched with room title, sender display names, and file attachment metadata."),
			mcp.WithString("roomId", mcp.Required(), mcp.Description("The ID of the room to list messages from")),
			mcp.WithString("mentionedPeople", mcp.Description("Filter by mentioned people (use 'me' for current user)")),
			mcp.WithString("before", mcp.Description("List messages before this date/time (ISO 8601)")),
			mcp.WithString("beforeMessage", mcp.Description("List messages before this message ID")),
			mcp.WithNumber("max", mcp.Description("Maximum number of messages to return (default 20, max 1000)")),
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

			// Build enriched response
			response := make(map[string]interface{})

			// Enrich: room context
			if roomInfo := resolveRoomInfo(client, roomID); roomInfo != nil {
				response["room"] = roomInfo
			}

			// Enrich: sender names (deduplicated) + file metadata per message
			nameCache := NewPersonNameCache(client)
			enrichedMessages := make([]map[string]interface{}, 0, len(page.Items))
			for _, msg := range page.Items {
				em := map[string]interface{}{
					"id":          msg.ID,
					"roomId":      msg.RoomID,
					"text":        msg.Text,
					"personId":    msg.PersonID,
					"personEmail": msg.PersonEmail,
					"created":     msg.Created,
				}

				if msg.Markdown != "" {
					em["markdown"] = msg.Markdown
				}
				if msg.HTML != "" {
					em["html"] = msg.HTML
				}
				if msg.ParentID != "" {
					em["parentId"] = msg.ParentID
				}
				if msg.Updated != nil {
					em["updated"] = msg.Updated
				}
				if len(msg.MentionedPeople) > 0 {
					em["mentionedPeople"] = msg.MentionedPeople
				}
				if len(msg.MentionedGroups) > 0 {
					em["mentionedGroups"] = msg.MentionedGroups
				}

				// Enrich: sender display name
				if name := nameCache.Resolve(msg.PersonID); name != "" {
					em["senderName"] = name
				}

				// Enrich: file metadata (HEAD only for list, no content download)
				if len(msg.Files) > 0 {
					fileInfos := make([]*FileInfo, 0, len(msg.Files))
					for _, fileURL := range msg.Files {
						if fi := resolveFileMetadata(client, fileURL); fi != nil {
							fileInfos = append(fileInfos, fi)
						}
					}
					if len(fileInfos) > 0 {
						em["files"] = fileInfos
					}
				}

				enrichedMessages = append(enrichedMessages, em)
			}
			response["messages"] = enrichedMessages

			data, _ := json.MarshalIndent(response, "", "  ")
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
			mcp.WithDescription("Get details of a specific Webex message by its ID. Response is enriched with sender display name, room title, and file attachment content (text files included inline, binary files with metadata)."),
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

			// Build enriched response
			response := map[string]interface{}{
				"message": result,
			}

			// Enrich: sender
			if result.PersonID != "" {
				if person, pErr := client.People().Get(result.PersonID); pErr == nil {
					response["sender"] = map[string]interface{}{
						"id":          person.ID,
						"displayName": person.DisplayName,
						"emails":      person.Emails,
					}
				}
			}

			// Enrich: room
			if roomInfo := resolveRoomInfo(client, result.RoomID); roomInfo != nil {
				response["room"] = roomInfo
			}

			// Enrich: files with content for text, metadata for binary
			if len(result.Files) > 0 {
				fileInfos := make([]*FileInfo, 0, len(result.Files))
				for _, fileURL := range result.Files {
					if fi := resolveFileContent(client, fileURL); fi != nil {
						fileInfos = append(fileInfos, fi)
					}
				}
				if len(fileInfos) > 0 {
					response["files"] = fileInfos
				}
			}

			data, _ := json.MarshalIndent(response, "", "  ")
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

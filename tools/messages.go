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
			mcp.WithDescription("List messages in a Webex room/space. Requires a roomId.\n"+
				"\n"+
				"HOW TO GET A ROOM ID:\n"+
				"- To read messages from a group space: use webex_rooms_list to find it by name.\n"+
				"- To read a 1:1 conversation with someone: use webex_rooms_list with type='direct' to list all 1:1 rooms. The room title for 1:1 rooms is the other person's display name.\n"+
				"- If you already have a roomId from a previous response, use it directly.\n"+
				"\n"+
				"PAGINATION: Returns latest 20 messages by default (newest first). To load older messages, pass the ID of the oldest message from your last result as 'beforeMessage'. Keep paginating until you get fewer results than 'max' or an empty list.\n"+
				"\n"+
				"RESPONSE: Enriched with room title, sender display names (resolved from IDs), and file attachment metadata (filename, size, content-type) for each message."),
			mcp.WithString("roomId", mcp.Required(), mcp.Description("The ID of the room/space to list messages from. Get this from webex_rooms_list, or from a previous API response.")),
			mcp.WithString("mentionedPeople", mcp.Description("Filter to only messages that mention specific people. Use the special value 'me' to find messages that mention the authenticated user. Otherwise pass a personId.")),
			mcp.WithString("before", mcp.Description("List messages sent before this date/time (ISO 8601 format, e.g. '2026-02-01T00:00:00Z'). Useful for searching messages in a date range.")),
			mcp.WithString("beforeMessage", mcp.Description("List messages sent before this message ID. Use this for pagination: pass the ID of the oldest message from the previous page to load the next page of older messages.")),
			mcp.WithNumber("max", mcp.Description("Maximum number of messages to return (default 20, max 1000). Start with a small number like 20 and paginate if you need more.")),
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
			mcp.WithDescription("Send a message to a Webex room/space or directly to a person.\n"+
				"\n"+
				"TO DM SOMEONE: Just pass their email address as 'toPersonEmail' -- this is the easiest way. You do NOT need to look up their personId or find a roomId first. Webex automatically creates or reuses the 1:1 room.\n"+
				"\n"+
				"TO MESSAGE A GROUP SPACE: Use 'roomId'. Get the roomId from webex_rooms_list.\n"+
				"\n"+
				"DESTINATION (exactly one required):\n"+
				"- toPersonEmail: The simplest way to DM someone. Just pass their email (e.g. 'alice@example.com').\n"+
				"- toPersonId: DM someone by their Webex person ID (use if you already have it from another API response).\n"+
				"- roomId: Send to a specific room/space (group rooms or existing 1:1 rooms).\n"+
				"\n"+
				"CONTENT (at least one required):\n"+
				"- text: Plain text message.\n"+
				"- markdown: Rich message with Webex-flavored markdown (supports bold, italic, links, mentions, code blocks, lists).\n"+
				"\n"+
				"IMPORTANT: Always confirm with the user before sending a message, unless the user has explicitly asked you not to confirm."),
			mcp.WithString("roomId", mcp.Description("The room/space ID to send the message to. Use this for group spaces. Get the ID from webex_rooms_list or a previous API response.")),
			mcp.WithString("toPersonId", mcp.Description("The person ID to send a direct 1:1 message to. Use only if you already have their personId from another API response. Otherwise prefer toPersonEmail.")),
			mcp.WithString("toPersonEmail", mcp.Description("The email address to send a direct 1:1 message to (e.g. 'alice@example.com'). This is the EASIEST way to DM someone -- no room lookup or person lookup needed. Webex automatically creates or reuses the 1:1 room.")),
			mcp.WithString("text", mcp.Description("Plain text message content. At least one of 'text' or 'markdown' is required.")),
			mcp.WithString("markdown", mcp.Description("Rich text message content using Webex-flavored markdown. Supports **bold**, *italic*, [links](url), @mentions, `code`, code blocks, and lists. Preferred over 'text' when formatting is desired.")),
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
			mcp.WithDescription("Get a specific Webex message by its ID. Use this when you have a messageId from a previous response (e.g. from webex_messages_list or a webhook notification) and need the full details.\n"+
				"\n"+
				"RESPONSE: Enriched with:\n"+
				"- sender: Display name and email of who sent the message.\n"+
				"- room: Title and type of the room the message is in.\n"+
				"- files: For any file attachments -- text-based files (txt, json, xml, csv, etc.) have their content included inline (up to 100KB). Binary files (pdf, images, etc.) include metadata (filename, size, content-type) so you can describe them.\n"+
				"\n"+
				"TIP: If the user asks 'what did someone send me' or 'what files were shared', use webex_messages_list first to find recent messages, then use this tool on specific messages that have attachments to get the file contents."),
			mcp.WithString("messageId", mcp.Required(), mcp.Description("The ID of the message to retrieve. Get this from webex_messages_list results or from webhook notification data.")),
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
			mcp.WithDescription("Delete a Webex message by its ID. You can only delete messages sent by the authenticated user (not other people's messages, unless the user is an admin/compliance officer).\n"+
				"\n"+
				"IMPORTANT: Always confirm with the user before deleting a message, unless the user has explicitly requested to not confirm. Deletion is permanent and cannot be undone."),
			mcp.WithString("messageId", mcp.Required(), mcp.Description("The ID of the message to delete. Get this from webex_messages_list or webex_messages_get results.")),
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

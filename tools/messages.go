package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
	"github.com/WebexCommunity/webex-go-sdk/v2/messages"
)

// RegisterMessageTools registers all message-related MCP tools.
func RegisterMessageTools(s ToolRegistrar, resolver auth.ClientResolver) {
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
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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
			mcp.WithDescription("Send a text message to a person or room in Webex.\n"+
				"\n"+
				"WHEN THE USER SAYS 'send a message to <email>' or 'message <name>@<domain>' or 'DM <email>':\n"+
				"→ Use toPersonEmail with their email address. That's it. ONE call. Do NOT look up rooms, people, or IDs first.\n"+
				"\n"+
				"WHEN THE USER SAYS 'send a message to <room name>' or 'post in <space name>':\n"+
				"→ Use webex_rooms_list to find the roomId, then use roomId here.\n"+
				"\n"+
				"QUICK REFERENCE:\n"+
				"- Have an email? → toPersonEmail (direct DM, no lookup needed)\n"+
				"- Have a room/space name? → find roomId via webex_rooms_list first, then roomId\n"+
				"- Have a personId from a previous call? → toPersonId\n"+
				"- Have a roomId from a previous call? → roomId\n"+
				"\n"+
				"To send files/attachments, use webex_messages_send_attachment instead.\n"+
				"\n"+
				"IMPORTANT: Always confirm with the user before sending, unless they explicitly said not to."),
			mcp.WithString("roomId", mcp.Description("Room/space ID. Use ONLY when sending to a group space or when you already have a roomId. Do NOT look up a room just to DM someone -- use toPersonEmail instead.")),
			mcp.WithString("toPersonId", mcp.Description("Person ID for a direct 1:1 message. Use only if you already have it from a previous API response.")),
			mcp.WithString("toPersonEmail", mcp.Description("Email address for a direct 1:1 message (e.g. 'alice@example.com'). USE THIS when the user provides an email. No room lookup or person lookup needed -- Webex handles everything.")),
			mcp.WithString("text", mcp.Description("Plain text message content.")),
			mcp.WithString("markdown", mcp.Description("Rich text using Webex markdown (bold, italic, links, code blocks, lists). Use this when formatting is desired.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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

	// webex_messages_send_attachment
	s.AddTool(
		mcp.NewTool("webex_messages_send_attachment",
			mcp.WithDescription("Send a message with a file attachment to a person or room in Webex.\n"+
				"\n"+
				"DESTINATION: Same as webex_messages_create -- use toPersonEmail for DMs (email is enough, no lookup needed), or roomId for group spaces.\n"+
				"\n"+
				"HOW TO ATTACH A FILE (provide exactly one approach):\n"+
				"\n"+
				"★ BEST: localFilePath -- Absolute path to a file on the local filesystem. The MCP server reads the file and uploads it directly. Use this when the file already exists on disk (e.g. saved charts, downloaded files, generated reports). This avoids base64 encoding overhead and LLM output token limits.\n"+
				"\n"+
				"★ PREFERRED: fileBase64 + fileName -- Base64-encode the file content and provide a filename. Use this for small generated content that isn't saved to disk. Be aware that very large base64 strings may be truncated by LLM output limits.\n"+
				"\n"+
				"⚠ FALLBACK ONLY: fileUrl -- A publicly accessible URL. Use this ONLY if you have a confirmed publicly reachable URL. Most URLs (internal, auth-gated, VPN-only, localhost) will FAIL because Webex servers must be able to download the file directly. When in doubt, use localFilePath or fileBase64 instead.\n"+
				"\n"+
				"You can optionally include a text or markdown message along with the file.\n"+
				"\n"+
				"LIMITATIONS:\n"+
				"- One file per message.\n"+
				"- Max file size: 100MB.\n"+
				"\n"+
				"IMPORTANT: Always confirm with the user before sending."),
			mcp.WithString("roomId", mcp.Description("Room/space ID. Use when sending to a group space or when you already have a roomId.")),
			mcp.WithString("toPersonId", mcp.Description("Person ID for a direct 1:1 message. Use only if you already have it.")),
			mcp.WithString("toPersonEmail", mcp.Description("Email address for a direct 1:1 message (e.g. 'alice@example.com'). No lookup needed.")),
			mcp.WithString("localFilePath", mcp.Description("BEST option. Absolute path to a file on the local filesystem (e.g. '/tmp/report.pdf', '/Users/me/chart.png'). The MCP server reads the file and uploads it directly to Webex. Use this when the file exists on disk — it avoids base64 encoding and LLM token limits. Provide ONLY this, OR fileBase64+fileName, OR fileUrl.")),
			mcp.WithString("fileBase64", mcp.Description("PREFERRED for in-memory content. Base64-encoded file content. Use with 'fileName' to upload directly. Works regardless of URL accessibility but large files may hit LLM output token limits — prefer localFilePath for large files. Provide ONLY this+fileName, OR localFilePath, OR fileUrl.")),
			mcp.WithString("fileName", mcp.Description("Filename for the upload (e.g. 'report.pdf', 'data.csv'). Required when using fileBase64. Optional with localFilePath (defaults to the file's actual name).")),
			mcp.WithString("fileUrl", mcp.Description("FALLBACK ONLY. A publicly accessible URL of the file to attach. Use ONLY if you have a confirmed publicly reachable URL (no auth, no VPN, no internal network). Most URLs will fail. Prefer localFilePath or fileBase64+fileName instead. Provide ONLY this, OR localFilePath, OR fileBase64+fileName.")),
			mcp.WithString("text", mcp.Description("Optional plain text message to include with the file.")),
			mcp.WithString("markdown", mcp.Description("Optional rich text message (Webex markdown) to include with the file.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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

			localFilePath := req.GetString("localFilePath", "")
			fileBase64 := req.GetString("fileBase64", "")
			fileName := req.GetString("fileName", "")
			fileURL := req.GetString("fileUrl", "")

			// Count how many file source approaches were provided
			sourceCount := 0
			if localFilePath != "" {
				sourceCount++
			}
			if fileBase64 != "" {
				sourceCount++
			}
			if fileURL != "" {
				sourceCount++
			}

			if sourceCount == 0 {
				return mcp.NewToolResultError("One of 'localFilePath', 'fileBase64' + 'fileName', or 'fileUrl' is required"), nil
			}
			if sourceCount > 1 {
				return mcp.NewToolResultError("Provide exactly one of 'localFilePath', 'fileBase64', or 'fileUrl' -- not multiple"), nil
			}

			var result *messages.Message

			if localFilePath != "" {
				// Local file upload: read from disk and send via multipart
				fileBytes, readErr := os.ReadFile(localFilePath)
				if readErr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to read local file '%s': %v", localFilePath, readErr)), nil
				}
				if fileName == "" {
					fileName = filepath.Base(localFilePath)
				}
				result, err = client.Messages().CreateWithAttachment(msg, &messages.FileUpload{
					FileName:  fileName,
					FileBytes: fileBytes,
				})
			} else if fileBase64 != "" {
				// Base64 upload via multipart form
				if fileName == "" {
					return mcp.NewToolResultError("'fileName' is required when using 'fileBase64' (e.g. 'report.pdf')"), nil
				}
				result, err = client.Messages().CreateWithBase64File(msg, fileName, fileBase64)
			} else {
				// URL-based attachment
				msg.Files = []string{fileURL}
				result, err = client.Messages().Create(msg)
			}

			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to send attachment: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_messages_send_adaptive_card
	s.AddTool(
		mcp.NewTool("webex_messages_send_adaptive_card",
			mcp.WithDescription("Send an Adaptive Card message to a person or room in Webex.\n"+
				"\n"+
				"Adaptive Cards are rich, interactive UI cards that can contain text, images, buttons, inputs, and more. "+
				"They are rendered natively in Webex clients.\n"+
				"\n"+
				"DESTINATION: Same as webex_messages_create -- use toPersonEmail for DMs (email is enough, no lookup needed), or roomId for group spaces.\n"+
				"\n"+
				"CARD FORMAT: Provide the card body as a JSON string in 'cardJson'. The JSON must follow the Adaptive Card schema "+
				"(see https://adaptivecards.io/explorer/). At minimum it should have:\n"+
				"  {\"type\": \"AdaptiveCard\", \"version\": \"1.3\", \"body\": [...]}\n"+
				"\n"+
				"IMAGES AND MEDIA IN CARDS:\n"+
				"For Image elements and any 'url' fields in the card, you can use:\n"+
				"\u2605 BEST: Local file path (e.g. \"url\": \"/tmp/chart.png\") -- The MCP server automatically reads the file and converts it to an embedded base64 data URI. Use this for generated charts, saved images, etc.\n"+
				"\u2605 OK: Public URL (e.g. \"url\": \"https://example.com/img.png\") -- Must be publicly accessible.\n"+
				"\u2605 OK: data: URI (e.g. \"url\": \"data:image/png;base64,...\") -- Already embedded, passed through as-is.\n"+
				"\n"+
				"EXAMPLES of body elements:\n"+
				"- TextBlock: {\"type\": \"TextBlock\", \"text\": \"Hello!\", \"size\": \"large\", \"weight\": \"bolder\"}\n"+
				"- Image: {\"type\": \"Image\", \"url\": \"/tmp/chart.png\"} (local path auto-converted)\n"+
				"- ColumnSet, FactSet, ActionSet, Input.Text, Action.Submit, etc.\n"+
				"\n"+
				"IMPORTANT: Always confirm with the user before sending."),
			mcp.WithString("roomId", mcp.Description("Room/space ID. Use when sending to a group space or when you already have a roomId.")),
			mcp.WithString("toPersonId", mcp.Description("Person ID for a direct 1:1 message. Use only if you already have it.")),
			mcp.WithString("toPersonEmail", mcp.Description("Email address for a direct 1:1 message (e.g. 'alice@example.com'). No lookup needed.")),
			mcp.WithString("cardJson", mcp.Required(), mcp.Description("The Adaptive Card body as a JSON string. Must be a valid Adaptive Card object with at least {\"type\": \"AdaptiveCard\", \"version\": \"1.3\", \"body\": [...]}. See https://adaptivecards.io/explorer/ for the full schema.")),
			mcp.WithString("fallbackText", mcp.Description("Plain text fallback displayed on clients that cannot render Adaptive Cards. If omitted, defaults to 'Adaptive Card'.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			msg := &messages.Message{
				RoomID:        req.GetString("roomId", ""),
				ToPersonID:    req.GetString("toPersonId", ""),
				ToPersonEmail: req.GetString("toPersonEmail", ""),
			}

			if msg.RoomID == "" && msg.ToPersonID == "" && msg.ToPersonEmail == "" {
				return mcp.NewToolResultError("One of roomId, toPersonId, or toPersonEmail is required"), nil
			}

			cardJSON, err := req.RequireString("cardJson")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			var cardBody interface{}
			if err := json.Unmarshal([]byte(cardJSON), &cardBody); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid cardJson: %v", err)), nil
			}

			// Resolve any local file paths in url fields to base64 data URIs
			if err := resolveLocalFileURLs(cardBody); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve local file paths in card: %v", err)), nil
			}

			card := messages.NewAdaptiveCard(cardBody)
			fallbackText := req.GetString("fallbackText", "")

			result, err := client.Messages().CreateWithAdaptiveCard(msg, card, fallbackText)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to send adaptive card: %v", err)), nil
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
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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

// resolveLocalFileURLs recursively walks a parsed JSON tree (from an Adaptive Card)
// and replaces any "url" values that are local file paths with base64 data URIs.
// Local paths start with "/" or "~/". HTTP(S) URLs and data: URIs are left as-is.
func resolveLocalFileURLs(node interface{}) error {
	switch v := node.(type) {
	case map[string]interface{}:
		// Check if this map has a "url" key with a local file path
		if urlVal, ok := v["url"]; ok {
			if urlStr, ok := urlVal.(string); ok {
				resolved, err := maybeResolveLocalPath(urlStr)
				if err != nil {
					return err
				}
				v["url"] = resolved
			}
		}
		// Recurse into all values
		for _, val := range v {
			if err := resolveLocalFileURLs(val); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, item := range v {
			if err := resolveLocalFileURLs(item); err != nil {
				return err
			}
		}
	}
	return nil
}

// maybeResolveLocalPath checks if a URL string is a local file path and, if so,
// reads the file and returns a base64 data URI. Otherwise returns the original string.
func maybeResolveLocalPath(urlStr string) (string, error) {
	// Skip URLs, data URIs, and empty strings
	if urlStr == "" ||
		strings.HasPrefix(urlStr, "http://") ||
		strings.HasPrefix(urlStr, "https://") ||
		strings.HasPrefix(urlStr, "data:") {
		return urlStr, nil
	}

	// Expand ~ to home directory
	path := urlStr
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand ~ in path %q: %w", urlStr, err)
		}
		path = filepath.Join(home, path[2:])
	}

	// Only treat absolute paths as local files
	if !filepath.IsAbs(path) {
		return urlStr, nil
	}

	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read local file %q: %w", path, err)
	}

	// Infer MIME type from extension
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	encoded := base64.StdEncoding.EncodeToString(fileBytes)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded), nil
}

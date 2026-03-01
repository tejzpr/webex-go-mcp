package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/WebexCommunity/webex-go-sdk/v2/memberships"
	"github.com/WebexCommunity/webex-go-sdk/v2/messages"
	"github.com/WebexCommunity/webex-go-sdk/v2/rooms"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
)

// RegisterRoomTools registers all room/space-related MCP tools.
func RegisterRoomTools(s ToolRegistrar, resolver auth.ClientResolver) {
	// webex_rooms_list
	s.AddTool(
		mcp.NewTool("webex_rooms_list",
			mcp.WithDescription("List Webex rooms/spaces the authenticated user belongs to. In Webex, a 'room' can be either a group space or a 1:1 direct conversation.\n"+
				"\n"+
				"ROOM TYPES:\n"+
				"- 'direct': A 1:1 conversation with another person. The room title is the other person's display name. Every pair of people has exactly one 1:1 room.\n"+
				"- 'group': A named space with multiple members (like a channel or project room).\n"+
				"\n"+
				"COMMON TASKS:\n"+
				"- Find a 1:1 chat with someone: Use type='direct' and sortBy='lastactivity' to list 1:1 rooms. Find the one whose title matches the person's name.\n"+
				"- Find a group space by name: Use type='group' and sortBy='lastactivity'. The room title is the space name.\n"+
				"- Find recently active conversations: Use sortBy='lastactivity' (no type filter) to get the most recent rooms of any type.\n"+
				"- List rooms in a team: Use teamId to filter by team.\n"+
				"\n"+
				"TIPS:\n"+
				"- You do NOT need to find a room to DM someone. Use webex_messages_create with 'toPersonEmail' directly -- it's much simpler.\n"+
				"- You only need a roomId when you want to read messages from a conversation (webex_messages_list requires it).\n"+
				"\n"+
				"RESPONSE: Enriched with team name, member count, and last message preview per room."+
				PaginationDescription),
			mcp.WithString("teamId", mcp.Description("Filter to only rooms that belong to this team. Get a teamId from webex_teams_list.")),
			mcp.WithString("type", mcp.Description("Filter by room type. 'direct' = 1:1 conversations (room title is the other person's name). 'group' = named multi-person spaces. Omit to get both types.")),
			mcp.WithString("sortBy", mcp.Description("Sort order: 'lastactivity' (most recently active first -- RECOMMENDED for finding recent conversations), 'created' (newest first), or 'id' (default, by room ID).")),
			mcp.WithString("nextPageUrl", mcp.Description(NextPageUrlParamDescription)),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			nextPageUrl := req.GetString("nextPageUrl", "")

			var roomItems []rooms.Room
			var hasNextPage bool
			var nextURL string

			if nextPageUrl != "" {
				// Direct next-page navigation — O(1) API call
				page, pErr := FetchPage(client, nextPageUrl)
				if pErr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch next page: %v", pErr)), nil
				}
				roomItems, err = UnmarshalPageItems[rooms.Room](page)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to parse rooms: %v", err)), nil
				}
				hasNextPage = page.HasNext
				nextURL = page.NextPage
			} else {
				// First page
				opts := &rooms.ListOptions{Max: PageSize}

				if v := req.GetString("teamId", ""); v != "" {
					opts.TeamID = v
				}
				if v := req.GetString("type", ""); v != "" {
					opts.Type = v
				}
				if v := req.GetString("sortBy", ""); v != "" {
					opts.SortBy = v
				}

				page, pErr := client.Rooms().List(opts)
				if pErr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to list rooms: %v", pErr)), nil
				}
				roomItems = page.Items
				hasNextPage = page.HasNext
				nextURL = page.NextPage
			}

			// Enrich each room
			teamCache := NewTeamNameCache(client)
			enrichedRooms := make([]map[string]interface{}, 0, len(roomItems))

			for _, room := range roomItems {
				er := map[string]interface{}{
					"room": room,
				}

				// Enrich: team name
				if room.TeamID != "" {
					if name := teamCache.Resolve(room.TeamID); name != "" {
						er["teamName"] = name
					}
				}

				// Enrich: member count
				if memberPage, mErr := client.Memberships().List(&memberships.ListOptions{
					RoomID: room.ID,
				}); mErr == nil {
					er["memberCount"] = len(memberPage.Items)
				}

				// Enrich: last message preview
				if msgPage, mErr := client.Messages().List(&messages.ListOptions{
					RoomID: room.ID,
					Max:    1,
				}); mErr == nil && len(msgPage.Items) > 0 {
					lastMsg := msgPage.Items[0]
					preview := lastMsg.Text
					if len(preview) > 200 {
						preview = preview[:200] + "..."
					}
					senderName := lastMsg.PersonEmail
					if senderName == "" {
						senderName = lastMsg.PersonID
					}
					er["lastMessagePreview"] = fmt.Sprintf("%s: %s", senderName, preview)
				}

				enrichedRooms = append(enrichedRooms, er)
			}

			result, fErr := FormatPaginatedResponse(enrichedRooms, hasNextPage, nextURL)
			if fErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", fErr)), nil
			}
			return mcp.NewToolResultText(result), nil
		},
	)

	// webex_rooms_create
	s.AddTool(
		mcp.NewTool("webex_rooms_create",
			mcp.WithDescription("Create a new Webex group room/space. This creates a multi-person space that others can be added to.\n"+
				"\n"+
				"NOTE: You do NOT need to create a room to send a 1:1 message. Use webex_messages_create with 'toPersonEmail' instead -- Webex auto-creates the 1:1 room.\n"+
				"\n"+
				"After creating a group room, use webex_memberships_create to add people to it."),
			mcp.WithString("title", mcp.Required(), mcp.Description("The name/title for the new room. Choose something descriptive (e.g. 'Project Alpha Discussion', 'Q1 Planning').")),
			mcp.WithString("teamId", mcp.Description("Optional team ID to associate this room with. The room will appear under that team. Get a teamId from webex_teams_list.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			title, err := req.RequireString("title")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			room := &rooms.Room{
				Title:  title,
				TeamID: req.GetString("teamId", ""),
			}

			result, err := client.Rooms().Create(room)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create room: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_rooms_get
	s.AddTool(
		mcp.NewTool("webex_rooms_get",
			mcp.WithDescription("Get full details of a specific Webex room/space by its ID. Use this when you have a roomId and need comprehensive info about the room.\n"+
				"\n"+
				"RESPONSE: Heavily enriched with:\n"+
				"- room: Full room details (title, type, created date, lastActivity, etc.).\n"+
				"- team: Team name and ID if the room belongs to a team.\n"+
				"- creator: Display name of who created the room.\n"+
				"- members: Full list of everyone in the room with their display names, emails, and moderator status.\n"+
				"- memberCount: Total number of members.\n"+
				"- recentMessages: The 5 most recent messages with sender names -- gives a snapshot of the current conversation.\n"+
				"\n"+
				"This is the best tool to use when the user asks 'who is in this room?' or 'what's happening in this space?' -- one call gets everything."),
			mcp.WithString("roomId", mcp.Required(), mcp.Description("The ID of the room to retrieve. Get this from webex_rooms_list or from any API response that includes a roomId.")),
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

			result, err := client.Rooms().Get(roomID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get room: %v", err)), nil
			}

			// Build enriched response
			response := map[string]interface{}{
				"room": result,
			}

			// Enrich: team
			if result.TeamID != "" {
				if team, tErr := client.Teams().Get(result.TeamID); tErr == nil {
					response["team"] = map[string]interface{}{
						"id":   team.ID,
						"name": team.Name,
					}
				}
			}

			// Enrich: creator
			if name := resolvePersonName(client, result.CreatorID); name != "" {
				response["creator"] = map[string]interface{}{
					"id":          result.CreatorID,
					"displayName": name,
				}
			}

			// Enrich: members (already includes personDisplayName)
			if memberPage, mErr := client.Memberships().List(&memberships.ListOptions{
				RoomID: roomID,
			}); mErr == nil {
				response["members"] = memberPage.Items
				response["memberCount"] = len(memberPage.Items)
			}

			// Enrich: recent messages with sender names
			if msgPage, mErr := client.Messages().List(&messages.ListOptions{
				RoomID: roomID,
				Max:    5,
			}); mErr == nil && len(msgPage.Items) > 0 {
				nameCache := NewPersonNameCache(client)
				recentMsgs := make([]map[string]interface{}, 0, len(msgPage.Items))
				for _, msg := range msgPage.Items {
					rm := map[string]interface{}{
						"id":          msg.ID,
						"text":        msg.Text,
						"personEmail": msg.PersonEmail,
						"created":     msg.Created,
					}
					if name := nameCache.Resolve(msg.PersonID); name != "" {
						rm["senderName"] = name
					}
					if len(msg.Files) > 0 {
						rm["fileCount"] = len(msg.Files)
					}
					recentMsgs = append(recentMsgs, rm)
				}
				response["recentMessages"] = recentMsgs
			}

			data, _ := json.MarshalIndent(response, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_rooms_update
	s.AddTool(
		mcp.NewTool("webex_rooms_update",
			mcp.WithDescription("Rename a Webex group room/space. Only works on group rooms -- 1:1 direct rooms cannot be renamed (their title is always the other person's name).\n"+
				"\n"+
				"IMPORTANT: Confirm with the user before renaming a room -- all members will see the name change."),
			mcp.WithString("roomId", mcp.Required(), mcp.Description("The ID of the group room to rename. Get this from webex_rooms_list.")),
			mcp.WithString("title", mcp.Required(), mcp.Description("The new title/name for the room.")),
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
			title, err := req.RequireString("title")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			room := &rooms.Room{
				Title: title,
			}

			result, err := client.Rooms().Update(roomID, room)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to update room: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_rooms_delete
	s.AddTool(
		mcp.NewTool("webex_rooms_delete",
			mcp.WithDescription("Permanently delete a Webex room/space and all its messages. This action is IRREVERSIBLE -- all messages, files, and membership history in the room will be lost.\n"+
				"\n"+
				"IMPORTANT: Always confirm with the user before deleting. The user must be a moderator of the room or the last person in it to delete."),
			mcp.WithString("roomId", mcp.Required(), mcp.Description("The ID of the room to permanently delete. Get this from webex_rooms_list.")),
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

			err = client.Rooms().Delete(roomID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to delete room: %v", err)), nil
			}

			return mcp.NewToolResultText("Room deleted successfully"), nil
		},
	)
}

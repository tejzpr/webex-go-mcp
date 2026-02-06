package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/memberships"
	"github.com/tejzpr/webex-go-sdk/v2/messages"
	"github.com/tejzpr/webex-go-sdk/v2/rooms"
)

// RegisterRoomTools registers all room/space-related MCP tools.
func RegisterRoomTools(s ToolRegistrar, client *webex.WebexClient) {
	// webex_rooms_list
	s.AddTool(
		mcp.NewTool("webex_rooms_list",
			mcp.WithDescription("List Webex rooms/spaces the authenticated user belongs to. Response is enriched with team name, member count, and last message preview per room."),
			mcp.WithString("teamId", mcp.Description("Filter rooms by team ID")),
			mcp.WithString("type", mcp.Description("Filter by room type: 'direct' (1:1) or 'group'")),
			mcp.WithString("sortBy", mcp.Description("Sort by: 'id', 'lastactivity', or 'created'")),
			mcp.WithNumber("max", mcp.Description("Maximum number of rooms to return")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			opts := &rooms.ListOptions{}

			if v := req.GetString("teamId", ""); v != "" {
				opts.TeamID = v
			}
			if v := req.GetString("type", ""); v != "" {
				opts.Type = v
			}
			if v := req.GetString("sortBy", ""); v != "" {
				opts.SortBy = v
			}
			if v := req.GetInt("max", 0); v > 0 {
				opts.Max = v
			}

			page, err := client.Rooms().List(opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list rooms: %v", err)), nil
			}

			// Enrich each room
			teamCache := NewTeamNameCache(client)
			enrichedRooms := make([]map[string]interface{}, 0, len(page.Items))

			for _, room := range page.Items {
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

			data, _ := json.MarshalIndent(enrichedRooms, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_rooms_create
	s.AddTool(
		mcp.NewTool("webex_rooms_create",
			mcp.WithDescription("Create a new Webex room/space."),
			mcp.WithString("title", mcp.Required(), mcp.Description("Title of the room")),
			mcp.WithString("teamId", mcp.Description("Team ID to associate the room with")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			mcp.WithDescription("Get details of a specific Webex room/space by its ID. Response is enriched with team info, creator name, full member list, and recent messages with sender names."),
			mcp.WithString("roomId", mcp.Required(), mcp.Description("The ID of the room to retrieve")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			mcp.WithDescription("Update a Webex room/space (e.g., change its title)."),
			mcp.WithString("roomId", mcp.Required(), mcp.Description("The ID of the room to update")),
			mcp.WithString("title", mcp.Required(), mcp.Description("New title for the room")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			mcp.WithDescription("Delete a Webex room/space by its ID."),
			mcp.WithString("roomId", mcp.Required(), mcp.Description("The ID of the room to delete")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

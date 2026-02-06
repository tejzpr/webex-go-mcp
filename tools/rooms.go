package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/rooms"
)

// RegisterRoomTools registers all room/space-related MCP tools.
func RegisterRoomTools(s ToolRegistrar, client *webex.WebexClient) {
	// webex_rooms_list
	s.AddTool(
		mcp.NewTool("webex_rooms_list",
			mcp.WithDescription("List Webex rooms/spaces the authenticated user belongs to."),
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

			data, _ := json.MarshalIndent(page.Items, "", "  ")
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
			mcp.WithDescription("Get details of a specific Webex room/space by its ID."),
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

			data, _ := json.MarshalIndent(result, "", "  ")
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

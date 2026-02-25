package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
	"github.com/WebexCommunity/webex-go-sdk/v2/memberships"
)

// RegisterMembershipTools registers all membership-related MCP tools.
func RegisterMembershipTools(s ToolRegistrar, resolver auth.ClientResolver) {
	// webex_memberships_list
	s.AddTool(
		mcp.NewTool("webex_memberships_list",
			mcp.WithDescription("List room memberships -- i.e., which people are in which rooms. A membership represents one person being in one room.\n"+
				"\n"+
				"COMMON TASKS:\n"+
				"- 'Who is in room X?' → Pass roomId. Response includes every member with their display name, email, and moderator status.\n"+
				"- 'What rooms is person X in?' → Pass personEmail (e.g. 'alice@example.com'). Returns all rooms that person is a member of.\n"+
				"- 'Is person X in room Y?' → Pass both roomId and personEmail. If results are empty, they're not in the room.\n"+
				"\n"+
				"TIP: You usually don't need this tool to find out who is in a room. webex_rooms_get already includes the full member list in its enriched response. Use this tool when you need to search across rooms by person.\n"+
				"\n"+
				"RESPONSE: Each membership includes personDisplayName and personEmail. When filtered by roomId, the response is enriched with the room title."),
			mcp.WithString("roomId", mcp.Description("Filter to members of this specific room. Returns all people in the room with display names and emails.")),
			mcp.WithString("personId", mcp.Description("Filter to memberships for this specific person ID. Returns all rooms this person is in.")),
			mcp.WithString("personEmail", mcp.Description("Filter to memberships for this person by email (e.g. 'alice@example.com'). Returns all rooms this person is in. This is the easiest way to find what rooms someone belongs to.")),
			mcp.WithNumber("max", mcp.Description("Maximum number of memberships to return.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			opts := &memberships.ListOptions{}

			roomID := req.GetString("roomId", "")
			if roomID != "" {
				opts.RoomID = roomID
			}
			if v := req.GetString("personId", ""); v != "" {
				opts.PersonID = v
			}
			if v := req.GetString("personEmail", ""); v != "" {
				opts.PersonEmail = v
			}
			if v := req.GetInt("max", 0); v > 0 {
				opts.Max = v
			}

			page, lErr := client.Memberships().List(opts)
			if lErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list memberships: %v", lErr)), nil
			}

			// Build enriched response
			response := map[string]interface{}{
				"memberships": page.Items,
			}

			// Enrich: room title when roomId is provided
			if roomID != "" {
				if roomInfo := resolveRoomInfo(client, roomID); roomInfo != nil {
					response["room"] = roomInfo
				}
			}

			data, _ := json.MarshalIndent(response, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_memberships_create
	s.AddTool(
		mcp.NewTool("webex_memberships_create",
			mcp.WithDescription("Add a person to a Webex room/space. The simplest way is to pass the roomId and the person's email address.\n"+
				"\n"+
				"EXAMPLE: To add alice@example.com to a room, just pass roomId + personEmail='alice@example.com'. That's it.\n"+
				"\n"+
				"IMPORTANT: Confirm with the user before adding someone to a room."),
			mcp.WithString("roomId", mcp.Required(), mcp.Description("The ID of the room to add the person to. Get this from webex_rooms_list or webex_rooms_create.")),
			mcp.WithString("personId", mcp.Description("The person ID to add. Use only if you already have it from another API response. Otherwise prefer personEmail.")),
			mcp.WithString("personEmail", mcp.Description("The email address of the person to add (e.g. 'alice@example.com'). This is the EASIEST way -- no person lookup needed.")),
			mcp.WithBoolean("isModerator", mcp.Description("Set to true to make this person a moderator of the room. Moderators can manage membership and room settings. Default: false.")),
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

			m := &memberships.Membership{
				RoomID:      roomID,
				PersonID:    req.GetString("personId", ""),
				PersonEmail: req.GetString("personEmail", ""),
				IsModerator: req.GetBool("isModerator", false),
			}

			if m.PersonID == "" && m.PersonEmail == "" {
				return mcp.NewToolResultError("Either personId or personEmail is required"), nil
			}

			result, err := client.Memberships().Create(m)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create membership: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_memberships_update
	s.AddTool(
		mcp.NewTool("webex_memberships_update",
			mcp.WithDescription("Update a room membership -- currently this means promoting or demoting someone as a moderator. Get the membershipId from webex_memberships_list or webex_rooms_get (which includes members in its enriched response).\n"+
				"\n"+
				"IMPORTANT: Confirm with the user before changing moderator status."),
			mcp.WithString("membershipId", mcp.Required(), mcp.Description("The ID of the membership to update. This is NOT the person ID or room ID -- it's the membership object ID from webex_memberships_list.")),
			mcp.WithBoolean("isModerator", mcp.Required(), mcp.Description("Set to true to make this person a moderator, false to remove moderator status.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			membershipID, err := req.RequireString("membershipId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			isModerator := req.GetBool("isModerator", false)

			m := &memberships.Membership{
				IsModerator: isModerator,
			}

			result, err := client.Memberships().Update(membershipID, m)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to update membership: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_memberships_delete
	s.AddTool(
		mcp.NewTool("webex_memberships_delete",
			mcp.WithDescription("Remove a person from a Webex room/space by deleting their membership. The person will lose access to the room and its message history.\n"+
				"\n"+
				"To find the membershipId: use webex_memberships_list with roomId and personEmail to find the specific membership, then use its ID here.\n"+
				"\n"+
				"IMPORTANT: Always confirm with the user before removing someone from a room."),
			mcp.WithString("membershipId", mcp.Required(), mcp.Description("The ID of the membership to delete. This is NOT the person ID or room ID -- get it from webex_memberships_list.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			membershipID, err := req.RequireString("membershipId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			err = client.Memberships().Delete(membershipID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to delete membership: %v", err)), nil
			}

			return mcp.NewToolResultText("Membership deleted successfully"), nil
		},
	)
}

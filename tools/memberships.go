package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/memberships"
)

// RegisterMembershipTools registers all membership-related MCP tools.
func RegisterMembershipTools(s ToolRegistrar, client *webex.WebexClient) {
	// webex_memberships_list
	s.AddTool(
		mcp.NewTool("webex_memberships_list",
			mcp.WithDescription("List memberships (people in rooms). Filter by roomId or personEmail."),
			mcp.WithString("roomId", mcp.Description("Filter by room ID")),
			mcp.WithString("personId", mcp.Description("Filter by person ID")),
			mcp.WithString("personEmail", mcp.Description("Filter by person email")),
			mcp.WithNumber("max", mcp.Description("Maximum number of memberships to return")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			opts := &memberships.ListOptions{}

			if v := req.GetString("roomId", ""); v != "" {
				opts.RoomID = v
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

			page, err := client.Memberships().List(opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list memberships: %v", err)), nil
			}

			data, _ := json.MarshalIndent(page.Items, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_memberships_create
	s.AddTool(
		mcp.NewTool("webex_memberships_create",
			mcp.WithDescription("Add a person to a Webex room/space. Provide roomId and either personId or personEmail."),
			mcp.WithString("roomId", mcp.Required(), mcp.Description("The room ID to add the person to")),
			mcp.WithString("personId", mcp.Description("The person ID to add")),
			mcp.WithString("personEmail", mcp.Description("The email of the person to add")),
			mcp.WithBoolean("isModerator", mcp.Description("Whether the person should be a moderator")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			mcp.WithDescription("Update a membership (e.g., promote/demote moderator)."),
			mcp.WithString("membershipId", mcp.Required(), mcp.Description("The ID of the membership to update")),
			mcp.WithBoolean("isModerator", mcp.Required(), mcp.Description("Whether the person should be a moderator")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			mcp.WithDescription("Remove a person from a Webex room/space by deleting their membership."),
			mcp.WithString("membershipId", mcp.Required(), mcp.Description("The ID of the membership to delete")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

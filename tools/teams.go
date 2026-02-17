package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
	"github.com/tejzpr/webex-go-sdk/v2/rooms"
	"github.com/tejzpr/webex-go-sdk/v2/teammemberships"
	"github.com/tejzpr/webex-go-sdk/v2/teams"
)

// RegisterTeamTools registers all team-related MCP tools.
func RegisterTeamTools(s ToolRegistrar, resolver auth.ClientResolver) {
	// webex_teams_list
	s.AddTool(
		mcp.NewTool("webex_teams_list",
			mcp.WithDescription("List Webex teams the authenticated user belongs to. A team is an organizational container that groups related rooms/spaces together (like a department, project, or squad).\n"+
				"\n"+
				"WEBEX HIERARCHY: Teams contain Rooms. Rooms contain Messages. Members belong to both Teams and Rooms.\n"+
				"\n"+
				"COMMON TASKS:\n"+
				"- 'What teams am I on?' -- Call this with no filters.\n"+
				"- 'What rooms are in team X?' -- Use the teamId from this response with webex_rooms_list.\n"+
				"\n"+
				"RESPONSE: Enriched with creator name, room count, and a list of rooms (with titles) for each team -- so you don't need a follow-up call to see what's inside."),
			mcp.WithNumber("max", mcp.Description("Maximum number of teams to return. Most users belong to a small number of teams.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			opts := &teams.ListOptions{}

			if v := req.GetInt("max", 0); v > 0 {
				opts.Max = v
			}

			page, err := client.Teams().List(opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list teams: %v", err)), nil
			}

			// Enrich each team
			nameCache := NewPersonNameCache(client)
			enrichedTeams := make([]map[string]interface{}, 0, len(page.Items))

			for _, team := range page.Items {
				et := map[string]interface{}{
					"team": team,
				}

				// Enrich: creator name
				if name := nameCache.Resolve(team.CreatorID); name != "" {
					et["creatorName"] = name
				}

				// Enrich: rooms for this team
				if roomPage, rErr := client.Rooms().List(&rooms.ListOptions{
					TeamID: team.ID,
				}); rErr == nil {
					et["roomCount"] = len(roomPage.Items)
					roomSummaries := make([]map[string]interface{}, 0, len(roomPage.Items))
					for _, r := range roomPage.Items {
						roomSummaries = append(roomSummaries, map[string]interface{}{
							"id":    r.ID,
							"title": r.Title,
						})
					}
					et["rooms"] = roomSummaries
				}

				enrichedTeams = append(enrichedTeams, et)
			}

			data, _ := json.MarshalIndent(enrichedTeams, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_teams_create
	s.AddTool(
		mcp.NewTool("webex_teams_create",
			mcp.WithDescription("Create a new Webex team. A team is a container for related rooms/spaces. When you create a team, Webex automatically creates a 'General' room inside it.\n"+
				"\n"+
				"After creating a team, use webex_rooms_create with the teamId to add more rooms, and webex_memberships_create to add people to the team's rooms."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Name for the new team (e.g. 'Engineering', 'Project Alpha', 'Q1 Sprint Team').")),
			mcp.WithString("description", mcp.Description("Optional description of the team's purpose.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			team := &teams.Team{
				Name:        name,
				Description: req.GetString("description", ""),
			}

			result, err := client.Teams().Create(team)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create team: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_teams_get
	s.AddTool(
		mcp.NewTool("webex_teams_get",
			mcp.WithDescription("Get full details of a specific Webex team by its ID.\n"+
				"\n"+
				"RESPONSE: Heavily enriched with:\n"+
				"- team: Full team details (name, description, creation date).\n"+
				"- creator: Display name and email of who created the team.\n"+
				"- rooms: All rooms/spaces in this team with their titles.\n"+
				"- roomCount: Total number of rooms.\n"+
				"- members: All team members with display names, emails, and moderator status.\n"+
				"- memberCount: Total number of members.\n"+
				"\n"+
				"This is the best tool when the user asks 'tell me about team X' or 'who is on team X?' -- one call gets everything."),
			mcp.WithString("teamId", mcp.Required(), mcp.Description("The ID of the team to retrieve. Get this from webex_teams_list.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			teamID, err := req.RequireString("teamId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			result, err := client.Teams().Get(teamID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get team: %v", err)), nil
			}

			// Build enriched response
			response := map[string]interface{}{
				"team": result,
			}

			// Enrich: creator
			if result.CreatorID != "" {
				if person, pErr := client.People().Get(result.CreatorID); pErr == nil {
					response["creator"] = map[string]interface{}{
						"id":          person.ID,
						"displayName": person.DisplayName,
						"emails":      person.Emails,
					}
				}
			}

			// Enrich: rooms for this team
			if roomPage, rErr := client.Rooms().List(&rooms.ListOptions{
				TeamID: teamID,
			}); rErr == nil {
				response["rooms"] = roomPage.Items
				response["roomCount"] = len(roomPage.Items)
			}

			// Enrich: team members
			if memberPage, mErr := client.TeamMemberships().List(&teammemberships.ListOptions{
				TeamID: teamID,
			}); mErr == nil {
				response["members"] = memberPage.Items
				response["memberCount"] = len(memberPage.Items)
			}

			data, _ := json.MarshalIndent(response, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_teams_update
	s.AddTool(
		mcp.NewTool("webex_teams_update",
			mcp.WithDescription("Rename a Webex team or update its description. All team members will see the change.\n"+
				"\n"+
				"IMPORTANT: Confirm with the user before renaming a team."),
			mcp.WithString("teamId", mcp.Required(), mcp.Description("The ID of the team to update. Get this from webex_teams_list.")),
			mcp.WithString("name", mcp.Required(), mcp.Description("The new name for the team.")),
			mcp.WithString("description", mcp.Description("Optional new description for the team.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			teamID, err := req.RequireString("teamId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			team := &teams.Team{
				Name:        name,
				Description: req.GetString("description", ""),
			}

			result, err := client.Teams().Update(teamID, team)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to update team: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

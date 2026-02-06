package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/rooms"
	"github.com/tejzpr/webex-go-sdk/v2/teammemberships"
	"github.com/tejzpr/webex-go-sdk/v2/teams"
)

// RegisterTeamTools registers all team-related MCP tools.
func RegisterTeamTools(s ToolRegistrar, client *webex.WebexClient) {
	// webex_teams_list
	s.AddTool(
		mcp.NewTool("webex_teams_list",
			mcp.WithDescription("List Webex teams the authenticated user belongs to. Response is enriched with creator name, room count, and room names per team."),
			mcp.WithNumber("max", mcp.Description("Maximum number of teams to return")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			mcp.WithDescription("Create a new Webex team."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Name of the team")),
			mcp.WithString("description", mcp.Description("Description of the team")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			mcp.WithDescription("Get details of a specific Webex team by its ID. Response is enriched with creator info, full rooms list with member counts, and team members."),
			mcp.WithString("teamId", mcp.Required(), mcp.Description("The ID of the team to retrieve")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			mcp.WithDescription("Update a Webex team (e.g., change its name)."),
			mcp.WithString("teamId", mcp.Required(), mcp.Description("The ID of the team to update")),
			mcp.WithString("name", mcp.Required(), mcp.Description("New name for the team")),
			mcp.WithString("description", mcp.Description("New description for the team")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

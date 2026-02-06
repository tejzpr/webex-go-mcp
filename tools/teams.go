package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/teams"
)

// RegisterTeamTools registers all team-related MCP tools.
func RegisterTeamTools(s *server.MCPServer, client *webex.WebexClient) {
	// webex_teams_list
	s.AddTool(
		mcp.NewTool("webex_teams_list",
			mcp.WithDescription("List Webex teams the authenticated user belongs to."),
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

			data, _ := json.MarshalIndent(page.Items, "", "  ")
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
			mcp.WithDescription("Get details of a specific Webex team by its ID."),
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

			data, _ := json.MarshalIndent(result, "", "  ")
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

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/transcripts"
)

// RegisterTranscriptTools registers all transcript-related MCP tools.
func RegisterTranscriptTools(s *server.MCPServer, client *webex.WebexClient) {
	// webex_transcripts_list
	s.AddTool(
		mcp.NewTool("webex_transcripts_list",
			mcp.WithDescription("List Webex meeting transcripts. Transcripts are AI-powered text records of meetings."),
			mcp.WithString("meetingId", mcp.Description("Filter by meeting ID")),
			mcp.WithString("hostEmail", mcp.Description("Filter by host email")),
			mcp.WithString("siteUrl", mcp.Description("Filter by Webex site URL")),
			mcp.WithString("from", mcp.Description("Start date/time filter (ISO 8601)")),
			mcp.WithString("to", mcp.Description("End date/time filter (ISO 8601)")),
			mcp.WithNumber("max", mcp.Description("Maximum number of transcripts to return")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			opts := &transcripts.ListOptions{}

			if v := req.GetString("meetingId", ""); v != "" {
				opts.MeetingID = v
			}
			if v := req.GetString("hostEmail", ""); v != "" {
				opts.HostEmail = v
			}
			if v := req.GetString("siteUrl", ""); v != "" {
				opts.SiteURL = v
			}
			if v := req.GetString("from", ""); v != "" {
				opts.From = v
			}
			if v := req.GetString("to", ""); v != "" {
				opts.To = v
			}
			if v := req.GetInt("max", 0); v > 0 {
				opts.Max = v
			}

			page, err := client.Transcripts().List(opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list transcripts: %v", err)), nil
			}

			data, _ := json.MarshalIndent(page.Items, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_transcripts_download
	s.AddTool(
		mcp.NewTool("webex_transcripts_download",
			mcp.WithDescription("Download the full content of a Webex meeting transcript in plain text or VTT format."),
			mcp.WithString("transcriptId", mcp.Required(), mcp.Description("The ID of the transcript to download")),
			mcp.WithString("format", mcp.Description("Download format: 'txt' (plain text) or 'vtt' (WebVTT with timestamps). Default: 'txt'")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			transcriptID, err := req.RequireString("transcriptId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			format := req.GetString("format", "txt")

			content, err := client.Transcripts().Download(transcriptID, format)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to download transcript: %v", err)), nil
			}

			return mcp.NewToolResultText(content), nil
		},
	)

	// webex_transcripts_list_snippets
	s.AddTool(
		mcp.NewTool("webex_transcripts_list_snippets",
			mcp.WithDescription("List individual spoken segments (snippets) from a Webex meeting transcript. Each snippet includes the speaker, text, and timing."),
			mcp.WithString("transcriptId", mcp.Required(), mcp.Description("The ID of the transcript")),
			mcp.WithNumber("max", mcp.Description("Maximum number of snippets to return")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			transcriptID, err := req.RequireString("transcriptId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &transcripts.SnippetListOptions{}
			if v := req.GetInt("max", 0); v > 0 {
				opts.Max = v
			}

			page, err := client.Transcripts().ListSnippets(transcriptID, opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list snippets: %v", err)), nil
			}

			data, _ := json.MarshalIndent(page.Items, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_transcripts_get_snippet
	s.AddTool(
		mcp.NewTool("webex_transcripts_get_snippet",
			mcp.WithDescription("Get a specific transcript snippet by its ID. A snippet is a short spoken segment by a participant."),
			mcp.WithString("transcriptId", mcp.Required(), mcp.Description("The ID of the transcript")),
			mcp.WithString("snippetId", mcp.Required(), mcp.Description("The ID of the snippet to retrieve")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			transcriptID, err := req.RequireString("transcriptId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			snippetID, err := req.RequireString("snippetId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			result, err := client.Transcripts().GetSnippet(transcriptID, snippetID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get snippet: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

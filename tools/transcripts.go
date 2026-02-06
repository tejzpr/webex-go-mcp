package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/transcripts"
)

// RegisterTranscriptTools registers all transcript-related MCP tools.
func RegisterTranscriptTools(s ToolRegistrar, client *webex.WebexClient) {
	// webex_transcripts_list
	s.AddTool(
		mcp.NewTool("webex_transcripts_list",
			mcp.WithDescription("List Webex meeting transcripts. Transcripts are AI-powered text records of meetings. If 'from' and 'to' are not specified (and no 'meetingId' is provided), defaults to the last 30 days. The date range between 'from' and 'to' must be within 30 days.\n"+
				"Response is enriched: each transcript includes the meeting title and a preview of the first 3 spoken snippets (who said what), giving enough context to understand the discussion without downloading the full transcript."),
			mcp.WithString("meetingId", mcp.Description("Filter by meeting ID")),
			mcp.WithString("hostEmail", mcp.Description("Filter by host email")),
			mcp.WithString("siteUrl", mcp.Description("Filter by Webex site URL")),
			mcp.WithString("from", mcp.Description("Optional start date/time filter (ISO 8601). Defaults to 30 days ago if not provided.")),
			mcp.WithString("to", mcp.Description("Optional end date/time filter (ISO 8601). Defaults to now if not provided.")),
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

			// Enrich each transcript with meeting title and snippet preview
			enrichedTranscripts := make([]map[string]interface{}, 0, len(page.Items))
			for _, t := range page.Items {
				et := map[string]interface{}{
					"transcript": t,
				}

				// Enrich: meeting title (meetingTopic is in transcript, but also fetch meeting for richer info)
				if t.MeetingID != "" {
					if meeting, mErr := client.Meetings().Get(t.MeetingID); mErr == nil {
						et["meetingTitle"] = meeting.Title
					} else {
						log.Printf("Enrichment: failed to get meeting %s for transcript %s: %v", t.MeetingID, t.ID, mErr)
						// Fall back to meetingTopic from the transcript itself
						if t.MeetingTopic != "" {
							et["meetingTitle"] = t.MeetingTopic
						}
					}
				}

				// Enrich: first 3 snippets as a preview
				if snippetPage, sErr := client.Transcripts().ListSnippets(t.ID, &transcripts.SnippetListOptions{
					Max: 3,
				}); sErr == nil && len(snippetPage.Items) > 0 {
					snippetPreviews := make([]map[string]interface{}, 0, len(snippetPage.Items))
					for _, s := range snippetPage.Items {
						preview := map[string]interface{}{
							"personName": s.PersonName,
							"text":       s.Text,
						}
						if s.StartTime != "" {
							preview["startTime"] = s.StartTime
						}
						snippetPreviews = append(snippetPreviews, preview)
					}
					et["snippetPreview"] = snippetPreviews
				} else if sErr != nil {
					log.Printf("Enrichment: failed to list snippets for transcript %s: %v", t.ID, sErr)
				}

				enrichedTranscripts = append(enrichedTranscripts, et)
			}

			data, _ := json.MarshalIndent(enrichedTranscripts, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_transcripts_download
	s.AddTool(
		mcp.NewTool("webex_transcripts_download",
			mcp.WithDescription("Download the full content of a Webex meeting transcript in plain text or VTT format. Both transcriptId and meetingId are required -- get them from webex_transcripts_list which returns both fields for each transcript."),
			mcp.WithString("transcriptId", mcp.Required(), mcp.Description("The ID of the transcript to download")),
			mcp.WithString("meetingId", mcp.Required(), mcp.Description("The meeting instance ID (returned as 'meetingId' in webex_transcripts_list results)")),
			mcp.WithString("format", mcp.Description("Download format: 'txt' (plain text) or 'vtt' (WebVTT with timestamps). Default: 'txt'")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			transcriptID, err := req.RequireString("transcriptId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			meetingID, err := req.RequireString("meetingId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			format := req.GetString("format", "txt")

			content, err := client.Transcripts().Download(transcriptID, format, &transcripts.DownloadOptions{MeetingID: meetingID})
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

	// webex_transcripts_update_snippet
	s.AddTool(
		mcp.NewTool("webex_transcripts_update_snippet",
			mcp.WithDescription("Update the text of a transcript snippet. Use this to correct transcription errors. Note: Transcripts generated by Cisco AI Assistant cannot be updated via API."),
			mcp.WithString("transcriptId", mcp.Required(), mcp.Description("The ID of the transcript")),
			mcp.WithString("snippetId", mcp.Required(), mcp.Description("The ID of the snippet to update")),
			mcp.WithString("text", mcp.Required(), mcp.Description("The corrected text for the snippet")),
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
			text, err := req.RequireString("text")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			snippet := &transcripts.Snippet{
				Text: text,
			}

			result, err := client.Transcripts().UpdateSnippet(transcriptID, snippetID, snippet)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to update snippet: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

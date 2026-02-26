package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/WebexCommunity/webex-go-sdk/v2/transcripts"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
)

// RegisterTranscriptTools registers all transcript-related MCP tools.
func RegisterTranscriptTools(s ToolRegistrar, resolver auth.ClientResolver) {
	// webex_transcripts_list
	s.AddTool(
		mcp.NewTool("webex_transcripts_list",
			mcp.WithDescription("List Webex meeting transcripts. Transcripts are text records of what was said during meetings.\n"+
				"\n"+
				"COMMON TASKS:\n"+
				"- 'What transcripts are available?' → Call with no filters (defaults to last 30 days).\n"+
				"- 'Get transcript for meeting X' → Pass meetingId from webex_meetings_list results.\n"+
				"- 'What was discussed in recent meetings?' → Call with no filters. The enriched snippet preview shows who said what.\n"+
				"\n"+
				"DATE RANGE: If 'from' and 'to' are not specified (and no meetingId), defaults to the last 30 days. The range between 'from' and 'to' must not exceed 30 days.\n"+
				"\n"+
				"RESPONSE: Enriched with:\n"+
				"- meetingTitle: The meeting name.\n"+
				"- snippetPreview: First 3 spoken snippets showing who said what -- enough to understand the discussion topic without downloading the full transcript.\n"+
				"- transcript: Includes both 'id' (transcriptId) and 'meetingId' -- you need BOTH to download the full transcript with webex_transcripts_download.\n"+
				"\n"+
				"WORKFLOW: List transcripts → pick one → use its transcriptId + meetingId to call webex_transcripts_download."),
			mcp.WithString("meetingId", mcp.Description("Filter to transcripts for a specific meeting. Get the meetingId from webex_meetings_list (look for meetings where hasTranscription=true) or from webex_meetings_get.")),
			mcp.WithString("hostEmail", mcp.Description("Filter to transcripts from meetings hosted by this email address.")),
			mcp.WithString("siteUrl", mcp.Description("Filter by Webex site URL. Usually not needed unless the user has multiple Webex sites.")),
			mcp.WithString("from", mcp.Description("Start of date range (UTC format: '2026-01-01T00:00:00Z'). Defaults to 30 days ago. The from-to range must be within 30 days.")),
			mcp.WithString("to", mcp.Description("End of date range (UTC format: '2026-02-06T23:59:59Z'). Defaults to now. The from-to range must be within 30 days.")),
			mcp.WithNumber("max", mcp.Description("Maximum number of transcripts to return.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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
				convertedFrom, err := validateAndConvertISO8601(v, "from")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				opts.From = convertedFrom
			}
			if v := req.GetString("to", ""); v != "" {
				convertedTo, err := validateAndConvertISO8601(v, "to")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				opts.To = convertedTo
			}
			if v := req.GetInt("max", 0); v > 0 {
				opts.Max = v
			}

			page, lErr := client.Transcripts().List(opts)
			if lErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list transcripts: %v", lErr)), nil
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
			mcp.WithDescription("Download the full text content of a Webex meeting transcript. Returns the complete conversation as text.\n"+
				"\n"+
				"REQUIRES BOTH transcriptId AND meetingId. Get both from webex_transcripts_list -- each transcript in the response has an 'id' (= transcriptId) and 'meetingId'.\n"+
				"\n"+
				"WORKFLOW:\n"+
				"1. Call webex_transcripts_list to find the transcript.\n"+
				"2. From the result, take the transcript 'id' and 'meetingId'.\n"+
				"3. Pass both to this tool.\n"+
				"\n"+
				"FORMATS:\n"+
				"- 'txt' (default): Plain text, easy to read. Best for summarizing or searching.\n"+
				"- 'vtt': WebVTT format with timestamps for each spoken segment. Use if the user needs timing info.\n"+
				"\n"+
				"TIP: The enriched webex_transcripts_list already includes a snippet preview (first 3 utterances). If that's enough to answer the user's question, you may not need to download the full transcript."),
			mcp.WithString("transcriptId", mcp.Required(), mcp.Description("The transcript ID to download. This is the 'id' field from webex_transcripts_list results.")),
			mcp.WithString("meetingId", mcp.Required(), mcp.Description("The meeting instance ID. This is the 'meetingId' field from the SAME transcript object in webex_transcripts_list results. MUST match the transcript.")),
			mcp.WithString("format", mcp.Description("Download format: 'txt' (plain text, default) or 'vtt' (WebVTT with timestamps). Use 'txt' unless the user specifically needs timestamps.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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
			mcp.WithDescription("List individual spoken segments (snippets) from a transcript. Each snippet is one utterance by one speaker, with their name, the text they said, and start/end timestamps.\n"+
				"\n"+
				"USE CASES:\n"+
				"- Browse through a transcript segment by segment.\n"+
				"- Find what a specific person said (filter client-side by personName).\n"+
				"- Get more granular data than the full download (which is just plain text).\n"+
				"\n"+
				"TIP: webex_transcripts_list already includes the first 3 snippets as a preview. Use this tool only if you need more snippets or the full conversation in structured form. For the complete transcript as plain text, use webex_transcripts_download instead."),
			mcp.WithString("transcriptId", mcp.Required(), mcp.Description("The transcript ID. Get this from webex_transcripts_list ('id' field in each transcript).")),
			mcp.WithNumber("max", mcp.Description("Maximum number of snippets to return. Each snippet is one spoken utterance. A 30-minute meeting might have 50-200 snippets. Start with 20-50 for an overview.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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
			mcp.WithDescription("Get a specific transcript snippet by ID. Returns the full details of one spoken segment (speaker, text, timestamps, duration, language).\n"+
				"\n"+
				"Rarely needed directly -- use webex_transcripts_list_snippets to browse snippets, or webex_transcripts_download for the full transcript text."),
			mcp.WithString("transcriptId", mcp.Required(), mcp.Description("The transcript ID containing this snippet.")),
			mcp.WithString("snippetId", mcp.Required(), mcp.Description("The specific snippet ID. Get this from webex_transcripts_list_snippets results.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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
			mcp.WithDescription("Correct the text of a transcript snippet. Use this when the transcription got a word or phrase wrong and the user wants to fix it.\n"+
				"\n"+
				"LIMITATION: Transcripts generated by Cisco AI Assistant cannot be updated via this API -- only standard Webex transcripts.\n"+
				"\n"+
				"WORKFLOW: Find the snippet via webex_transcripts_list_snippets → identify the one to correct → pass its snippetId and the corrected text here.\n"+
				"\n"+
				"IMPORTANT: Confirm the correction with the user before updating."),
			mcp.WithString("transcriptId", mcp.Required(), mcp.Description("The transcript ID containing the snippet to correct.")),
			mcp.WithString("snippetId", mcp.Required(), mcp.Description("The ID of the specific snippet to update. Get this from webex_transcripts_list_snippets.")),
			mcp.WithString("text", mcp.Required(), mcp.Description("The corrected text to replace the existing snippet text.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

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

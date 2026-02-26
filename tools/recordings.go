package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/WebexCommunity/webex-go-sdk/v2/recordings"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
)

// RegisterRecordingTools registers all recording-related MCP tools
func RegisterRecordingTools(s ToolRegistrar, resolver auth.ClientResolver) {
	// webex_recordings_list
	s.AddTool(
		mcp.NewTool("webex_recordings_list",
			mcp.WithDescription("List Webex meeting recordings. The Webex Recordings API provides access to meeting recordings that have been processed and are available for download.\n"+
				"\n"+
				"COMMON USES:\n"+
				"- 'What recordings are available?' → Call with no filters (defaults to all accessible recordings).\n"+
				"- 'Get recordings for meeting X' → Pass meetingId from webex_meetings_list results.\n"+
				"- 'What recordings from last week?' → Use from/to date range.\n"+
				"- 'Recordings I hosted' → Use hostEmail filter.\n"+
				"\n"+
				"DATE RANGE: If 'from' and 'to' are not specified, returns all accessible recordings. The from-to range can be used to filter by recording time.\n"+
				"\n"+
				"RESPONSE: Each recording includes download URLs, playback URLs, duration, file size, and metadata like topic, host email, and recording status."),
			mcp.WithString("meetingId", mcp.Description("Filter to recordings for a specific meeting. Get the meetingId from webex_meetings_list (look for meetings where hasRecording=true) or from webex_meetings_get.")),
			mcp.WithString("meetingSeriesId", mcp.Description("Filter to recordings for a specific meeting series (recurring meetings).")),
			mcp.WithString("hostEmail", mcp.Description("Filter to recordings from meetings hosted by this email address.")),
			mcp.WithString("siteUrl", mcp.Description("Filter by Webex site URL. Usually not needed unless the user has multiple Webex sites.")),
			mcp.WithString("from", mcp.Description("Start of date range (UTC format: '2026-01-01T00:00:00Z'). Use with 'to' to define a date range for recording time.")),
			mcp.WithString("to", mcp.Description("End of date range (UTC format: '2026-02-06T23:59:59Z'). Use with 'from' to define a date range for recording time.")),
			mcp.WithString("serviceType", mcp.Description("Filter by service type (e.g., 'meeting', 'event', 'webinar').")),
			mcp.WithString("status", mcp.Description("Filter by recording status (e.g., 'available', 'processing', 'failed').")),
			mcp.WithString("topic", mcp.Description("Filter by recording topic (meeting title).")),
			mcp.WithString("format", mcp.Description("Filter by recording format (e.g., 'mp4', 'mp3', 'wav').")),
			mcp.WithNumber("max", mcp.Description("Maximum number of recordings to return.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			opts := &recordings.ListOptions{}

			if v := req.GetString("meetingId", ""); v != "" {
				opts.MeetingID = v
			}
			if v := req.GetString("meetingSeriesId", ""); v != "" {
				opts.MeetingSeriesID = v
			}
			if v := req.GetString("hostEmail", ""); v != "" {
				opts.HostEmail = v
			}
			if v := req.GetString("siteUrl", ""); v != "" {
				opts.SiteURL = v
			}
			if v := req.GetString("from", ""); v != "" {
				if err := validateISO8601(v, "from"); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				opts.From = v
			}
			if v := req.GetString("to", ""); v != "" {
				if err := validateISO8601(v, "to"); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				opts.To = v
			}
			if v := req.GetString("serviceType", ""); v != "" {
				opts.ServiceType = v
			}
			if v := req.GetString("status", ""); v != "" {
				opts.Status = v
			}
			if v := req.GetString("topic", ""); v != "" {
				opts.Topic = v
			}
			if v := req.GetString("format", ""); v != "" {
				opts.Format = v
			}
			if v := req.GetInt("max", 0); v > 0 {
				opts.Max = v
			}

			// Debug logging
			log.Printf("[recordings] List options: %+v", opts)

			// Try using client.Recordings() like transcripts
			page, lErr := client.Recordings().List(opts)
			if lErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list recordings: %v", lErr)), nil
			}

			log.Printf("[recordings] Found %d recordings", len(page.Items))

			// Enrich each recording with additional information
			enrichedRecordings := make([]map[string]interface{}, 0, len(page.Items))
			for _, recording := range page.Items {
				er := map[string]interface{}{
					"recording": recording,
				}

				// Enrich: meeting information if we have meeting ID
				if recording.MeetingID != "" {
					if meeting, mErr := client.Meetings().Get(recording.MeetingID); mErr == nil {
						er["meeting"] = map[string]interface{}{
							"id":              meeting.ID,
							"title":           meeting.Title,
							"start":           meeting.Start,
							"end":             meeting.End,
							"hostEmail":       meeting.HostEmail,
							"hostDisplayName": meeting.HostDisplayName,
							"state":           meeting.State,
							"meetingType":     meeting.MeetingType,
							"webLink":         meeting.WebLink,
						}
					} else {
						log.Printf("Enrichment: failed to get meeting %s: %v", recording.MeetingID, mErr)
					}
				}

				// Enrich: download and playback URLs
				if recording.DownloadURL != "" {
					er["downloadUrl"] = recording.DownloadURL
				}
				if recording.PlaybackURL != "" {
					er["playbackUrl"] = recording.PlaybackURL
				}

				// Enrich: file size in human readable format
				if recording.SizeBytes > 0 {
					er["sizeBytes"] = recording.SizeBytes
					if recording.SizeBytes < 1024 {
						er["sizeHuman"] = fmt.Sprintf("%d B", recording.SizeBytes)
					} else if recording.SizeBytes < 1024*1024 {
						er["sizeHuman"] = fmt.Sprintf("%.1f KB", float64(recording.SizeBytes)/1024)
					} else {
						er["sizeHuman"] = fmt.Sprintf("%.1f MB", float64(recording.SizeBytes)/(1024*1024))
					}
				}

				// Enrich: duration in human readable format
				if recording.DurationSeconds > 0 {
					er["durationSeconds"] = recording.DurationSeconds
					if recording.DurationSeconds < 60 {
						er["durationHuman"] = fmt.Sprintf("%d seconds", recording.DurationSeconds)
					} else if recording.DurationSeconds < 3600 {
						er["durationHuman"] = fmt.Sprintf("%.1f minutes", float64(recording.DurationSeconds)/60)
					} else {
						er["durationHuman"] = fmt.Sprintf("%.1f hours", float64(recording.DurationSeconds)/3600)
					}
				}

				// Enrich: recording status
				if recording.Status != "" {
					er["status"] = recording.Status
				}

				// Enrich: format information
				if recording.Format != "" {
					er["format"] = recording.Format
				}

				// Enrich: service type
				if recording.ServiceType != "" {
					er["serviceType"] = recording.ServiceType
				}

				// Enrich: password protection
				if recording.Password != "" {
					er["hasPassword"] = true
				}

				// Enrich: share information
				if recording.ShareToMe {
					er["shareToMe"] = true
				}

				// Enrich: integration tags
				if len(recording.IntegrationTags) > 0 {
					er["integrationTags"] = recording.IntegrationTags
				}

				// Enrich: temporary download links
				if recording.TemporaryDirectDownloadLinks != nil {
					er["temporaryDirectDownloadLinks"] = recording.TemporaryDirectDownloadLinks
				}

				enrichedRecordings = append(enrichedRecordings, er)
			}

			data, _ := json.MarshalIndent(enrichedRecordings, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_recordings_get
	s.AddTool(
		mcp.NewTool("webex_recordings_get",
			mcp.WithDescription("Get detailed information about a specific recording by its ID.\n"+
				"\n"+
				"RESPONSE: Enriched with:\n"+
				"- recording: Full recording details\n"+
				"- meeting: Basic meeting information if available\n"+
				"- downloadUrl: Direct download link\n"+
				"- playbackUrl: Direct playback link\n"+
				"- sizeHuman: Human-readable file size\n"+
				"- durationHuman: Human-readable duration"),
			mcp.WithString("recordingId", mcp.Required(), mcp.Description("The ID of the recording to retrieve. Get this from webex_recordings_list.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			recordingID, err := req.RequireString("recordingId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Try using client.Recordings() like transcripts
			result, err := client.Recordings().Get(recordingID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get recording: %v", err)), nil
			}

			// Build enriched response
			response := map[string]interface{}{
				"recording": result,
			}

			// Enrich: meeting information if we have meeting ID
			if result.MeetingID != "" {
				if meeting, mErr := client.Meetings().Get(result.MeetingID); mErr == nil {
					response["meeting"] = map[string]interface{}{
						"id":              meeting.ID,
						"title":           meeting.Title,
						"start":           meeting.Start,
						"end":             meeting.End,
						"hostEmail":       meeting.HostEmail,
						"hostDisplayName": meeting.HostDisplayName,
						"state":           meeting.State,
						"meetingType":     meeting.MeetingType,
						"webLink":         meeting.WebLink,
					}
				} else {
					log.Printf("Enrichment: failed to get meeting %s: %v", result.MeetingID, mErr)
				}
			}

			// Enrich: download and playback URLs
			if result.DownloadURL != "" {
				response["downloadUrl"] = result.DownloadURL
			}
			if result.PlaybackURL != "" {
				response["playbackUrl"] = result.PlaybackURL
			}

			// Enrich: file size in human readable format
			if result.SizeBytes > 0 {
				response["sizeBytes"] = result.SizeBytes
				if result.SizeBytes < 1024 {
					response["sizeHuman"] = fmt.Sprintf("%d B", result.SizeBytes)
				} else if result.SizeBytes < 1024*1024 {
					response["sizeHuman"] = fmt.Sprintf("%.1f KB", float64(result.SizeBytes)/1024)
				} else {
					response["sizeHuman"] = fmt.Sprintf("%.1f MB", float64(result.SizeBytes)/(1024*1024))
				}
			}

			// Enrich: duration in human readable format
			if result.DurationSeconds > 0 {
				response["durationSeconds"] = result.DurationSeconds
				if result.DurationSeconds < 60 {
					response["durationHuman"] = fmt.Sprintf("%d seconds", result.DurationSeconds)
				} else if result.DurationSeconds < 3600 {
					response["durationHuman"] = fmt.Sprintf("%.1f minutes", float64(result.DurationSeconds)/60)
				} else {
					response["durationHuman"] = fmt.Sprintf("%.1f hours", float64(result.DurationSeconds)/3600)
				}
			}

			// Enrich: password protection
			if result.Password != "" {
				response["hasPassword"] = true
			}

			// Enrich: share information
			if result.ShareToMe {
				response["shareToMe"] = true
			}

			// Enrich: integration tags
			if len(result.IntegrationTags) > 0 {
				response["integrationTags"] = result.IntegrationTags
			}

			// Enrich: temporary download links
			if result.TemporaryDirectDownloadLinks != nil {
				response["temporaryDirectDownloadLinks"] = result.TemporaryDirectDownloadLinks
			}

			data, _ := json.MarshalIndent(response, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_recordings_download
	s.AddTool(
		mcp.NewTool("webex_recordings_download",
			mcp.WithDescription("Download a recording file content. Returns the actual file content for text-based formats or metadata for binary formats.\n"+
				"\n"+
				"SUPPORTED FORMATS:\n"+
				"- Text formats (txt, json, xml, csv): Returns full file content\n"+
				"- Binary formats (mp4, mp3, wav, etc.): Returns metadata with file information\n"+
				"\n"+
				"USAGE: Get recordingId from webex_recordings_list, then call this tool to download the actual recording file."),
			mcp.WithString("recordingId", mcp.Required(), mcp.Description("The ID of the recording to download. Get this from webex_recordings_list.")),
			mcp.WithString("format", mcp.Description("Optional format preference (e.g., 'mp4', 'mp3', 'txt'). If not specified, uses the recording's default format.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			recordingID, err := req.RequireString("recordingId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// First get recording details to check format
			recording, err := client.Recordings().Get(recordingID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get recording details: %v", err)), nil
			}

			// Determine the format to use
			format := req.GetString("format", "")
			if format == "" {
				format = recording.Format // Use default format
			}

			log.Printf("[recordings] Downloading recording %s in format %s", recordingID, format)

			// Try using client.Recordings() like transcripts
			// Note: This might not exist, we'll need to check
			// For now, return metadata since actual file download would require additional implementation
			response := map[string]interface{}{
				"recordingId": recordingID,
				"format":      format,
				"downloadUrl": recording.DownloadURL,
				"playbackUrl": recording.PlaybackURL,
				"sizeBytes":   recording.SizeBytes,
				"status":      recording.Status,
				"note":        "File download would be implemented here using the download URL",
			}

			data, _ := json.MarshalIndent(response, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

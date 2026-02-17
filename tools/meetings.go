package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
	"github.com/tejzpr/webex-go-sdk/v2/meetings"
	"github.com/tejzpr/webex-go-sdk/v2/transcripts"
)

// RegisterMeetingTools registers all meeting-related MCP tools.
func RegisterMeetingTools(s ToolRegistrar, resolver auth.ClientResolver) {
	// webex_meetings_list
	s.AddTool(
		mcp.NewTool("webex_meetings_list",
			mcp.WithDescription("List Webex meetings. The Webex Meetings API uses three distinct object types controlled by 'meetingType'. Understanding this is CRITICAL:\n"+
				"\n"+
				"MEETING TYPES:\n"+
				"- meetingSeries: The recurring definition/template (e.g. 'Weekly Standup every Monday'). This is the DEFAULT if you omit meetingType. Not useful for finding specific past/future meetings.\n"+
				"- scheduledMeeting: An upcoming scheduled occurrence that hasn't happened yet. USE THIS for 'what meetings do I have today/this week/tomorrow'.\n"+
				"- meeting: An actual instance that has started, is in progress, or has ended. USE THIS for 'what meetings happened last week/yesterday'.\n"+
				"\n"+
				"COMMON TASKS:\n"+
				"- 'What meetings do I have today?' → meetingType='scheduledMeeting', from='2026-02-06T00:00:00Z', to='2026-02-06T23:59:59Z'\n"+
				"- 'What meetings happened last week?' → meetingType='meeting', state='ended', from='2026-01-27T00:00:00Z', to='2026-02-02T23:59:59Z'\n"+
				"- 'List all my recurring meetings' → No meetingType (or meetingType='meetingSeries')\n"+
				"- 'Find meetings with transcripts' → Use meetingType='meeting' with state='ended'. Look for hasTranscription=true in results.\n"+
				"\n"+
				"IMPORTANT RULES:\n"+
				"- 'state' requires 'meetingType' to be set.\n"+
				"- 'from' and 'to' define the time window. Always use ISO 8601 format.\n"+
				"\n"+
				"RESPONSE: Enriched -- for each meeting with hasTranscription=true, the response includes transcript IDs and meetingIds so you can download transcripts directly with webex_transcripts_download. No extra calls needed.\n"+
				"\n"+
				"RESPONSE FIELDS: Each meeting includes title, start, end, meetingType, state, hostDisplayName, hostEmail, webLink (join URL), hasTranscription, hasRecording, and more."),
			mcp.WithString("meetingType", mcp.Description("CRITICAL parameter that controls what type of meeting objects are returned:\n"+
				"- 'scheduledMeeting': Upcoming scheduled occurrences. Use for 'meetings today', 'meetings this week', 'next meeting'.\n"+
				"- 'meeting': Actual instances that started/ended. Use for 'past meetings', 'meetings last week', 'meetings with recordings'.\n"+
				"- 'meetingSeries': Recurring definitions/templates. Use for 'what recurring meetings do I have'.\n"+
				"- Omit: Defaults to meetingSeries. NOT useful for finding specific day's meetings.")),
			mcp.WithString("state", mcp.Description("Filter meetings by state. REQUIRES meetingType to be set.\n"+
				"Common values: 'scheduled' (upcoming), 'ended' (finished, use with meetingType='meeting'), 'active' (in progress), 'missed' (not attended).\n"+
				"All values: 'active', 'scheduled', 'ready', 'lobby', 'connected', 'started', 'ended', 'missed', 'expired'.")),
			mcp.WithString("scheduledType", mcp.Description("Filter by the type of scheduled event: 'meeting' (standard meeting), 'webinar' (Webex webinar), 'personalRoomMeeting' (personal room). Usually not needed.")),
			mcp.WithString("from", mcp.Description("Start of time window (ISO 8601, e.g. '2026-02-06T00:00:00Z'). Use with 'to' to define a date range. For today's meetings, use today's date at 00:00:00.")),
			mcp.WithString("to", mcp.Description("End of time window (ISO 8601, e.g. '2026-02-06T23:59:59Z'). Use with 'from' to define a date range. For today's meetings, use today's date at 23:59:59.")),
			mcp.WithString("hostEmail", mcp.Description("Filter by meeting host email. Only works for admin users -- regular users can only see their own meetings.")),
			mcp.WithString("meetingNumber", mcp.Description("Filter by the Webex meeting number (the numeric code used to join). Useful when the user provides a specific meeting number.")),
			mcp.WithNumber("max", mcp.Description("Maximum number of meetings to return. Default varies by Webex API. Use 10-20 for searching, higher for comprehensive listing.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			opts := &meetings.ListOptions{}

			if v := req.GetString("meetingType", ""); v != "" {
				opts.MeetingType = v
			}
			if v := req.GetString("state", ""); v != "" {
				opts.State = v
			}
			if v := req.GetString("scheduledType", ""); v != "" {
				opts.ScheduledType = v
			}
			if v := req.GetString("from", ""); v != "" {
				opts.From = v
			}
			if v := req.GetString("to", ""); v != "" {
				opts.To = v
			}
			if v := req.GetString("hostEmail", ""); v != "" {
				opts.HostEmail = v
			}
			if v := req.GetString("meetingNumber", ""); v != "" {
				opts.MeetingNumber = v
			}
			if v := req.GetInt("max", 0); v > 0 {
				opts.Max = v
			}

			page, lErr := client.Meetings().List(opts)
			if lErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list meetings: %v", lErr)), nil
			}

			// Enrich each meeting with transcript info if hasTranscription
			enrichedMeetings := make([]map[string]interface{}, 0, len(page.Items))
			for _, meeting := range page.Items {
				em := map[string]interface{}{
					"meeting": meeting,
				}

				// Enrich: transcripts for meetings that have them
				if meeting.HasTranscription {
					if tPage, tErr := client.Transcripts().List(&transcripts.ListOptions{
						MeetingID: meeting.ID,
					}); tErr == nil && len(tPage.Items) > 0 {
						transcriptSummaries := make([]map[string]interface{}, 0, len(tPage.Items))
						for _, t := range tPage.Items {
							transcriptSummaries = append(transcriptSummaries, map[string]interface{}{
								"transcriptId": t.ID,
								"meetingId":    t.MeetingID,
								"status":       t.Status,
							})
						}
						em["transcripts"] = transcriptSummaries
					} else if tErr != nil {
						log.Printf("Enrichment: failed to list transcripts for meeting %s: %v", meeting.ID, tErr)
					}
				}

				enrichedMeetings = append(enrichedMeetings, em)
			}

			data, _ := json.MarshalIndent(enrichedMeetings, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_meetings_create
	s.AddTool(
		mcp.NewTool("webex_meetings_create",
			mcp.WithDescription("Schedule a new Webex meeting with optional invitees. Creates a meeting that participants can join via the webLink in the response.\n"+
				"\n"+
				"INVITEES: Pass a comma-separated list of email addresses to automatically invite people. They receive a Webex meeting invite. Example: 'alice@example.com,bob@example.com'\n"+
				"\n"+
				"IMPORTANT: Always confirm the meeting details (title, time, timezone, invitees) with the user before creating.\n"+
				"\n"+
				"TIPS:\n"+
				"- Always specify a timezone if the user mentions one (e.g. 'America/New_York', 'Asia/Kolkata', 'Europe/London'). If no timezone is mentioned, ask the user or default to UTC.\n"+
				"- For a 30-minute meeting at 2pm ET: start='2026-02-06T14:00:00', end='2026-02-06T14:30:00', timezone='America/New_York'\n"+
				"- The response includes the webLink (join URL) and meetingNumber that participants need to join."),
			mcp.WithString("title", mcp.Required(), mcp.Description("Title of the meeting (e.g. 'Weekly Team Sync', '1:1 with Alice').")),
			mcp.WithString("start", mcp.Required(), mcp.Description("Start time in ISO 8601 format (e.g. '2026-02-06T14:00:00Z' for UTC, or '2026-02-06T14:00:00' if using timezone parameter). Always clarify the timezone with the user.")),
			mcp.WithString("end", mcp.Required(), mcp.Description("End time in ISO 8601 format. Must be after start. Common durations: 30 min, 1 hour. Example: if start is 14:00, end for a 1-hour meeting is 15:00.")),
			mcp.WithString("invitees", mcp.Description("Comma-separated email addresses to invite to the meeting (e.g. 'alice@example.com,bob@example.com,charlie@example.com'). Each person receives a Webex meeting invite.")),
			mcp.WithString("timezone", mcp.Description("IANA timezone name (e.g. 'America/New_York', 'Asia/Kolkata', 'Europe/London', 'US/Pacific'). If omitted, times are treated as UTC. ALWAYS set this when the user mentions a timezone or location.")),
			mcp.WithString("agenda", mcp.Description("Optional meeting agenda or description. Appears in the meeting invite.")),
			mcp.WithString("password", mcp.Description("Optional meeting password. If omitted, Webex generates one automatically.")),
			mcp.WithString("recurrence", mcp.Description("Optional recurrence rule in RFC 2445 / iCal RRULE format. Examples: 'FREQ=WEEKLY;BYDAY=MO' (every Monday), 'FREQ=DAILY;COUNT=5' (next 5 days), 'FREQ=WEEKLY;INTERVAL=2;BYDAY=TU,TH' (every other Tue/Thu).")),
			mcp.WithBoolean("enabledAutoRecordMeeting", mcp.Description("Set to true to automatically record the meeting when it starts. Default: false.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			title, err := req.RequireString("title")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			start, err := req.RequireString("start")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			end, err := req.RequireString("end")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			meeting := &meetings.Meeting{
				Title:                    title,
				Start:                    start,
				End:                      end,
				Timezone:                 req.GetString("timezone", ""),
				Agenda:                   req.GetString("agenda", ""),
				Password:                 req.GetString("password", ""),
				Recurrence:               req.GetString("recurrence", ""),
				EnabledAutoRecordMeeting: req.GetBool("enabledAutoRecordMeeting", false),
			}

			// Parse invitees from comma-separated emails
			if inviteesStr := req.GetString("invitees", ""); inviteesStr != "" {
				emails := strings.Split(inviteesStr, ",")
				invitees := make([]meetings.Invitee, 0, len(emails))
				for _, email := range emails {
					email = strings.TrimSpace(email)
					if email != "" {
						invitees = append(invitees, meetings.Invitee{Email: email})
					}
				}
				if len(invitees) > 0 {
					meeting.Invitees = invitees
				}
			}

			result, err := client.Meetings().Create(meeting)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create meeting: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_meetings_get
	s.AddTool(
		mcp.NewTool("webex_meetings_get",
			mcp.WithDescription("Get full details of a specific Webex meeting by its ID.\n"+
				"\n"+
				"RESPONSE: Enriched with:\n"+
				"- meeting: Full meeting details (title, start, end, state, webLink, meetingNumber, etc.).\n"+
				"- hostName: Display name of the meeting host.\n"+
				"- transcripts: If the meeting has transcripts (hasTranscription=true), includes transcript IDs and meetingIds ready for webex_transcripts_download.\n"+
				"\n"+
				"COMMON USE: After finding a meeting via webex_meetings_list, use this tool if you need the full details, host name, or transcript IDs."),
			mcp.WithString("meetingId", mcp.Required(), mcp.Description("The ID of the meeting to retrieve. Get this from webex_meetings_list results.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			meetingID, err := req.RequireString("meetingId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			result, err := client.Meetings().Get(meetingID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get meeting: %v", err)), nil
			}

			// Build enriched response
			response := map[string]interface{}{
				"meeting": result,
			}

			// Enrich: host name
			if result.HostUserID != "" {
				if name := resolvePersonName(client, result.HostUserID); name != "" {
					response["hostName"] = name
				}
			}

			// Enrich: transcripts
			if result.HasTranscription {
				if tPage, tErr := client.Transcripts().List(&transcripts.ListOptions{
					MeetingID: result.ID,
				}); tErr == nil && len(tPage.Items) > 0 {
					transcriptSummaries := make([]map[string]interface{}, 0, len(tPage.Items))
					for _, t := range tPage.Items {
						transcriptSummaries = append(transcriptSummaries, map[string]interface{}{
							"transcriptId": t.ID,
							"meetingId":    t.MeetingID,
							"status":       t.Status,
						})
					}
					response["transcripts"] = transcriptSummaries
				} else if tErr != nil {
					log.Printf("Enrichment: failed to list transcripts for meeting %s: %v", result.ID, tErr)
				}
			}

			data, _ := json.MarshalIndent(response, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_meetings_update
	s.AddTool(
		mcp.NewTool("webex_meetings_update",
			mcp.WithDescription("Update an existing Webex meeting (reschedule, rename, change agenda, etc.).\n"+
				"\n"+
				"NOTE: For recurring meetings, updating the meetingSeries ID changes ALL occurrences. To change a single occurrence, update the specific scheduledMeeting ID instead.\n"+
				"\n"+
				"IMPORTANT: Confirm changes with the user before updating. Participants will be notified of the change."),
			mcp.WithString("meetingId", mcp.Required(), mcp.Description("The ID of the meeting to update. Get this from webex_meetings_list. For recurring meetings, use the series ID to update all, or a specific occurrence ID to update just one.")),
			mcp.WithString("title", mcp.Required(), mcp.Description("The meeting title (pass existing title if not changing).")),
			mcp.WithString("start", mcp.Description("New start time (ISO 8601 format). Include timezone parameter if not using UTC.")),
			mcp.WithString("end", mcp.Description("New end time (ISO 8601 format). Must be after start.")),
			mcp.WithString("timezone", mcp.Description("IANA timezone name (e.g. 'America/New_York'). Set when changing meeting time.")),
			mcp.WithString("agenda", mcp.Description("Updated meeting agenda or description.")),
			mcp.WithString("password", mcp.Description("New meeting password.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			meetingID, err := req.RequireString("meetingId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			title, err := req.RequireString("title")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			meeting := &meetings.Meeting{
				Title:    title,
				Start:    req.GetString("start", ""),
				End:      req.GetString("end", ""),
				Timezone: req.GetString("timezone", ""),
				Agenda:   req.GetString("agenda", ""),
				Password: req.GetString("password", ""),
			}

			result, err := client.Meetings().Update(meetingID, meeting)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to update meeting: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_meetings_delete
	s.AddTool(
		mcp.NewTool("webex_meetings_delete",
			mcp.WithDescription("Cancel/delete a Webex meeting. For recurring meetings, deleting the meetingSeries ID cancels ALL occurrences.\n"+
				"\n"+
				"IMPORTANT: Always confirm with the user before canceling a meeting. Participants will be notified of the cancellation."),
			mcp.WithString("meetingId", mcp.Required(), mcp.Description("The ID of the meeting to cancel/delete. Get this from webex_meetings_list. For recurring meetings: series ID cancels all, specific occurrence ID cancels just that one.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			meetingID, err := req.RequireString("meetingId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			err = client.Meetings().Delete(meetingID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to delete meeting: %v", err)), nil
			}

			return mcp.NewToolResultText("Meeting deleted successfully"), nil
		},
	)

	// webex_meetings_list_participants
	s.AddTool(
		mcp.NewTool("webex_meetings_list_participants",
			mcp.WithDescription("List participants who joined a Webex meeting. Shows who actually attended, when they joined/left, whether they were host/co-host, and their device info.\n"+
				"\n"+
				"USE THIS WHEN:\n"+
				"- 'Who attended the meeting?' or 'Who was in the call?'\n"+
				"- 'Did <person> join the meeting?'\n"+
				"- 'How long was <person> in the meeting?'\n"+
				"\n"+
				"NOTE: This only works for meetings that have already started or ended (meetingType='meeting'). You need the meeting instance ID, not the series ID. Use webex_meetings_list with meetingType='meeting' to find past meeting instances.\n"+
				"\n"+
				"RESPONSE: Each participant includes displayName, email, joinedTime, leftTime, state (joined/left/end), host/coHost flags, and device info."),
			mcp.WithString("meetingId", mcp.Required(), mcp.Description("The meeting instance ID (not the series ID). Get this from webex_meetings_list with meetingType='meeting'.")),
			mcp.WithNumber("max", mcp.Description("Maximum number of participants to return. Default varies by API.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			meetingID, err := req.RequireString("meetingId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &meetings.ParticipantListOptions{
				MeetingID: meetingID,
			}
			if v := req.GetInt("max", 0); v > 0 {
				opts.Max = v
			}

			page, err := client.Meetings().ListParticipants(opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list participants: %v", err)), nil
			}

			data, _ := json.MarshalIndent(page.Items, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

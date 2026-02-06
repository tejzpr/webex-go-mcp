package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/meetings"
)

// RegisterMeetingTools registers all meeting-related MCP tools.
func RegisterMeetingTools(s ToolRegistrar, client *webex.WebexClient) {
	// webex_meetings_list
	s.AddTool(
		mcp.NewTool("webex_meetings_list",
			mcp.WithDescription("List Webex meetings. Can filter by date range, type, and state. IMPORTANT: The Webex API requires 'meetingType' to be set whenever 'state' is used as a filter. Without 'meetingType', the API returns meeting series (recurring definitions) rather than actual instances. Use meetingType='meeting' with state='ended' and a from/to date range to list past meeting instances."),
			mcp.WithString("meetingType", mcp.Description("Filter by type: 'meetingSeries', 'scheduledMeeting', or 'meeting'. Required when 'state' is specified.")),
			mcp.WithString("state", mcp.Description("Filter by state: 'active', 'scheduled', 'ready', 'lobby', 'connected', 'started', 'ended', 'missed', 'expired'. Requires 'meetingType' to also be set.")),
			mcp.WithString("scheduledType", mcp.Description("Filter by scheduled type: 'meeting', 'webinar', 'personalRoomMeeting'")),
			mcp.WithString("from", mcp.Description("Start date/time filter (ISO 8601, e.g. '2026-01-01T00:00:00Z')")),
			mcp.WithString("to", mcp.Description("End date/time filter (ISO 8601)")),
			mcp.WithString("hostEmail", mcp.Description("Filter by host email (admin only)")),
			mcp.WithString("meetingNumber", mcp.Description("Filter by meeting number")),
			mcp.WithNumber("max", mcp.Description("Maximum number of meetings to return")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

			page, err := client.Meetings().List(opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list meetings: %v", err)), nil
			}

			data, _ := json.MarshalIndent(page.Items, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_meetings_create
	s.AddTool(
		mcp.NewTool("webex_meetings_create",
			mcp.WithDescription("Schedule a new Webex meeting."),
			mcp.WithString("title", mcp.Required(), mcp.Description("Title of the meeting")),
			mcp.WithString("start", mcp.Required(), mcp.Description("Start time in ISO 8601 format (e.g. '2026-02-01T10:00:00Z')")),
			mcp.WithString("end", mcp.Required(), mcp.Description("End time in ISO 8601 format")),
			mcp.WithString("timezone", mcp.Description("Timezone (e.g. 'America/New_York'). Defaults to UTC.")),
			mcp.WithString("agenda", mcp.Description("Meeting agenda/description")),
			mcp.WithString("password", mcp.Description("Meeting password")),
			mcp.WithString("recurrence", mcp.Description("Recurrence pattern in RFC 2445 format")),
			mcp.WithBoolean("enabledAutoRecordMeeting", mcp.Description("Automatically record the meeting")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			mcp.WithDescription("Get details of a specific Webex meeting by its ID."),
			mcp.WithString("meetingId", mcp.Required(), mcp.Description("The ID of the meeting to retrieve")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			meetingID, err := req.RequireString("meetingId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			result, err := client.Meetings().Get(meetingID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get meeting: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// webex_meetings_update
	s.AddTool(
		mcp.NewTool("webex_meetings_update",
			mcp.WithDescription("Update an existing Webex meeting."),
			mcp.WithString("meetingId", mcp.Required(), mcp.Description("The ID of the meeting to update")),
			mcp.WithString("title", mcp.Required(), mcp.Description("Updated title of the meeting")),
			mcp.WithString("start", mcp.Description("Updated start time (ISO 8601)")),
			mcp.WithString("end", mcp.Description("Updated end time (ISO 8601)")),
			mcp.WithString("timezone", mcp.Description("Updated timezone")),
			mcp.WithString("agenda", mcp.Description("Updated agenda/description")),
			mcp.WithString("password", mcp.Description("Updated meeting password")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			mcp.WithDescription("Delete/cancel a Webex meeting by its ID."),
			mcp.WithString("meetingId", mcp.Required(), mcp.Description("The ID of the meeting to delete")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
}

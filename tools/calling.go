package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/WebexCommunity/webex-go-sdk/v2/calling"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
)

// RegisterCallingTools registers Webex Calling settings tools.
func RegisterCallingTools(s ToolRegistrar, resolver auth.ClientResolver) {
	s.AddTool(
		mcp.NewTool("webex_calling_get_availability",
			mcp.WithDescription("Get the authenticated user's Webex Calling availability setting. This reads Do Not Disturb (DND); available=true means DND is disabled."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			resp, err := client.Calling().CallSettings().GetDoNotDisturbSetting()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get calling availability: %v", err)), nil
			}
			if result := callSettingErrorResult("Failed to get calling availability", resp); result != nil {
				return result, nil
			}

			dnd, err := decodeToggleSetting(resp)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to parse calling availability: %v", err)), nil
			}
			data, _ := json.MarshalIndent(map[string]interface{}{
				"available":  !dnd.Enabled,
				"dndEnabled": dnd.Enabled,
				"response":   resp,
			}, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("webex_calling_set_availability",
			mcp.WithDescription("Set the authenticated user's Webex Calling availability. This writes Do Not Disturb (DND): available=true disables DND; available=false enables DND."),
			mcp.WithBoolean("available", mcp.Required(), mcp.Description("Set true to be available for calls, false to enable Do Not Disturb.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			available, err := req.RequireBool("available")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dndEnabled := !available

			resp, err := client.Calling().CallSettings().SetDoNotDisturbSetting(dndEnabled)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to set calling availability: %v", err)), nil
			}
			if result := callSettingErrorResult("Failed to set calling availability", resp); result != nil {
				return result, nil
			}

			data, _ := json.MarshalIndent(map[string]interface{}{
				"available":  available,
				"dndEnabled": dndEnabled,
				"response":   resp,
			}, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("webex_calling_get_call_forwarding",
			mcp.WithDescription("Get the authenticated user's Webex Calling forwarding settings, including always, busy, no-answer, and business-continuity forwarding."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			resp, err := client.Calling().CallSettings().GetCallForwardSetting()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get call forwarding: %v", err)), nil
			}
			if result := callSettingErrorResult("Failed to get call forwarding", resp); result != nil {
				return result, nil
			}

			data, _ := json.MarshalIndent(resp, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("webex_calling_set_call_forwarding",
			mcp.WithDescription("Update the authenticated user's Webex Calling forwarding settings. Only supplied fields are changed; omitted forwarding rules are preserved. To disable forwarding, pass the relevant *Enabled fields as false."),
			mcp.WithBoolean("alwaysEnabled", mcp.Description("Enable or disable always-on call forwarding.")),
			mcp.WithString("alwaysDestination", mcp.Description("Destination number or address for always-on forwarding. Pass an empty string to clear it.")),
			mcp.WithBoolean("alwaysRingReminderEnabled", mcp.Description("Whether to play a ring reminder when always-on forwarding is enabled.")),
			mcp.WithBoolean("alwaysDestinationVoicemailEnabled", mcp.Description("Forward always-on calls to voicemail instead of a destination number.")),
			mcp.WithBoolean("busyEnabled", mcp.Description("Enable or disable forwarding when the line is busy.")),
			mcp.WithString("busyDestination", mcp.Description("Destination number or address for busy forwarding. Pass an empty string to clear it.")),
			mcp.WithBoolean("busyDestinationVoicemailEnabled", mcp.Description("Forward busy calls to voicemail instead of a destination number.")),
			mcp.WithBoolean("noAnswerEnabled", mcp.Description("Enable or disable forwarding when calls are not answered.")),
			mcp.WithString("noAnswerDestination", mcp.Description("Destination number or address for no-answer forwarding. Pass an empty string to clear it.")),
			mcp.WithNumber("noAnswerNumberOfRings", mcp.Description("Number of rings before no-answer forwarding starts.")),
			mcp.WithBoolean("noAnswerDestinationVoicemailEnabled", mcp.Description("Forward unanswered calls to voicemail instead of a destination number.")),
			mcp.WithBoolean("businessContinuityEnabled", mcp.Description("Enable or disable forwarding when the line is unreachable/offline.")),
			mcp.WithString("businessContinuityDestination", mcp.Description("Destination number or address for business-continuity forwarding. Pass an empty string to clear it.")),
			mcp.WithBoolean("businessContinuityDestinationVoicemailEnabled", mcp.Description("Forward unreachable/offline calls to voicemail instead of a destination number.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			settingsClient := client.Calling().CallSettings()
			currentResp, err := settingsClient.GetCallForwardSetting()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get current call forwarding: %v", err)), nil
			}
			if result := callSettingErrorResult("Failed to get current call forwarding", currentResp); result != nil {
				return result, nil
			}

			setting, err := decodeCallForwardSetting(currentResp)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to parse current call forwarding: %v", err)), nil
			}

			changed, err := applyCallForwardingArgs(req, &setting)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !changed {
				return mcp.NewToolResultError("Provide at least one call forwarding field to update"), nil
			}

			resp, err := settingsClient.SetCallForwardSetting(setting)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to set call forwarding: %v", err)), nil
			}
			if result := callSettingErrorResult("Failed to set call forwarding", resp); result != nil {
				return result, nil
			}

			data, _ := json.MarshalIndent(map[string]interface{}{
				"updated":  setting,
				"response": resp,
			}, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

func decodeToggleSetting(resp *calling.CallSettingResponse) (calling.ToggleSetting, error) {
	var setting calling.ToggleSetting
	data, err := marshalCallSetting(resp)
	if err != nil {
		return setting, err
	}
	if len(data) == 0 {
		return setting, nil
	}
	return setting, json.Unmarshal(data, &setting)
}

func decodeCallForwardSetting(resp *calling.CallSettingResponse) (calling.CallForwardSetting, error) {
	var setting calling.CallForwardSetting
	data, err := marshalCallSetting(resp)
	if err != nil {
		return setting, err
	}
	if len(data) == 0 {
		return setting, nil
	}
	return setting, json.Unmarshal(data, &setting)
}

func marshalCallSetting(resp *calling.CallSettingResponse) ([]byte, error) {
	if resp == nil || resp.Data.CallSetting == nil {
		return nil, nil
	}
	data, err := json.Marshal(resp.Data.CallSetting)
	if err != nil {
		return nil, err
	}
	if string(data) == "null" {
		return nil, nil
	}
	return data, nil
}

func callSettingErrorResult(prefix string, resp *calling.CallSettingResponse) *mcp.CallToolResult {
	if resp == nil {
		return mcp.NewToolResultError(prefix + ": empty response")
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	if resp.Data.Error != "" {
		return mcp.NewToolResultError(fmt.Sprintf("%s: %s", prefix, resp.Data.Error))
	}
	return mcp.NewToolResultError(fmt.Sprintf("%s: status %d", prefix, resp.StatusCode))
}

func applyCallForwardingArgs(req mcp.CallToolRequest, setting *calling.CallForwardSetting) (bool, error) {
	changed := false
	var applied bool
	var err error

	applied, err = applyOptionalBool(req, "alwaysEnabled", func(v bool) {
		setting.CallForwarding.Always.Enabled = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalString(req, "alwaysDestination", func(v string) {
		setting.CallForwarding.Always.Destination = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalBoolPtr(req, "alwaysRingReminderEnabled", func(v *bool) {
		setting.CallForwarding.Always.RingReminderEnabled = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalBoolPtr(req, "alwaysDestinationVoicemailEnabled", func(v *bool) {
		setting.CallForwarding.Always.DestinationVoicemailEnabled = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalBool(req, "busyEnabled", func(v bool) {
		setting.CallForwarding.Busy.Enabled = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalString(req, "busyDestination", func(v string) {
		setting.CallForwarding.Busy.Destination = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalBoolPtr(req, "busyDestinationVoicemailEnabled", func(v *bool) {
		setting.CallForwarding.Busy.DestinationVoicemailEnabled = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalBool(req, "noAnswerEnabled", func(v bool) {
		setting.CallForwarding.NoAnswer.Enabled = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalString(req, "noAnswerDestination", func(v string) {
		setting.CallForwarding.NoAnswer.Destination = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalIntPtr(req, "noAnswerNumberOfRings", func(v *int) {
		setting.CallForwarding.NoAnswer.NumberOfRings = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalBoolPtr(req, "noAnswerDestinationVoicemailEnabled", func(v *bool) {
		setting.CallForwarding.NoAnswer.DestinationVoicemailEnabled = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalBool(req, "businessContinuityEnabled", func(v bool) {
		setting.BusinessContinuity.Enabled = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalString(req, "businessContinuityDestination", func(v string) {
		setting.BusinessContinuity.Destination = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	applied, err = applyOptionalBoolPtr(req, "businessContinuityDestinationVoicemailEnabled", func(v *bool) {
		setting.BusinessContinuity.DestinationVoicemailEnabled = v
	})
	if err != nil {
		return false, err
	}
	changed = changed || applied

	return changed, nil
}

func applyOptionalBool(req mcp.CallToolRequest, key string, apply func(bool)) (bool, error) {
	if !hasArgument(req, key) {
		return false, nil
	}
	value, err := req.RequireBool(key)
	if err != nil {
		return false, err
	}
	apply(value)
	return true, nil
}

func applyOptionalBoolPtr(req mcp.CallToolRequest, key string, apply func(*bool)) (bool, error) {
	if !hasArgument(req, key) {
		return false, nil
	}
	value, err := req.RequireBool(key)
	if err != nil {
		return false, err
	}
	apply(&value)
	return true, nil
}

func applyOptionalString(req mcp.CallToolRequest, key string, apply func(string)) (bool, error) {
	if !hasArgument(req, key) {
		return false, nil
	}
	value, err := req.RequireString(key)
	if err != nil {
		return false, err
	}
	apply(value)
	return true, nil
}

func applyOptionalIntPtr(req mcp.CallToolRequest, key string, apply func(*int)) (bool, error) {
	if !hasArgument(req, key) {
		return false, nil
	}
	value, err := req.RequireInt(key)
	if err != nil {
		return false, err
	}
	if value < 1 {
		return false, fmt.Errorf("%s must be greater than 0", key)
	}
	apply(&value)
	return true, nil
}

func hasArgument(req mcp.CallToolRequest, key string) bool {
	args := req.GetArguments()
	if args == nil {
		return false
	}
	_, ok := args[key]
	return ok
}

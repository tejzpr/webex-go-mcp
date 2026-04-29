package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const (
	approvalFieldName = "confirm"
	maxPreviewLength  = 280
	maxPreviewArgs    = 12
)

var elicitationProtectedToolPrefixes = []string{
	"webex_messages_create",
	"webex_messages_send_attachment",
	"webex_messages_send_adaptive_card",
	"webex_messages_delete",
	"webex_rooms_create",
	"webex_rooms_update",
	"webex_rooms_delete",
	"webex_teams_create",
	"webex_teams_update",
	"webex_memberships_create",
	"webex_memberships_update",
	"webex_memberships_delete",
	"webex_meetings_create",
	"webex_meetings_update",
	"webex_meetings_patch",
	"webex_meetings_delete",
	"webex_transcripts_update_snippet",
	"webex_webhooks_create",
	"webex_webhooks_update",
	"webex_webhooks_delete",
}

// ElicitingRegistrar wraps mutating Webex tools with MCP elicitation.
type ElicitingRegistrar struct {
	mcpServer *mcpserver.MCPServer
	inner     ToolRegistrar
}

// NewElicitingRegistrar creates a registrar that requires user approval before
// running tools that create, send, update, patch, or delete Webex resources.
func NewElicitingRegistrar(mcpServer *mcpserver.MCPServer, inner ToolRegistrar) *ElicitingRegistrar {
	return &ElicitingRegistrar{
		mcpServer: mcpServer,
		inner:     inner,
	}
}

// AddTool registers the tool, wrapping protected handlers with an approval check.
func (r *ElicitingRegistrar) AddTool(tool mcp.Tool, handler mcpserver.ToolHandlerFunc) {
	if r == nil || r.inner == nil {
		return
	}
	if r.mcpServer == nil || !RequiresElicitation(tool.Name) {
		r.inner.AddTool(tool, handler)
		return
	}

	r.inner.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if result := r.requestApproval(ctx, tool.Name, req); result != nil {
			return result, nil
		}
		return handler(ctx, req)
	})
}

func (r *ElicitingRegistrar) requestApproval(ctx context.Context, toolName string, req mcp.CallToolRequest) *mcp.CallToolResult {
	if err := elicitationSupportError(ctx); err != nil {
		return mcp.NewToolResultError(elicitationErrorMessage(toolName, err))
	}

	result, err := r.mcpServer.RequestElicitation(ctx, mcp.ElicitationRequest{
		Request: mcp.Request{
			Method: string(mcp.MethodElicitationCreate),
		},
		Params: mcp.ElicitationParams{
			Mode:    mcp.ElicitationModeForm,
			Message: buildApprovalMessage(toolName, req),
			RequestedSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					approvalFieldName: map[string]any{
						"type":        "boolean",
						"title":       "Approve",
						"description": "Approve this Webex action.",
						"default":     false,
					},
				},
				"required": []string{approvalFieldName},
			},
		},
	})
	if err != nil {
		return mcp.NewToolResultError(elicitationErrorMessage(toolName, err))
	}
	if result == nil {
		return mcp.NewToolResultError(fmt.Sprintf("User approval is required for %s, but the MCP client returned no elicitation response. Action not performed.", toolName))
	}
	if result.Action != mcp.ElicitationResponseActionAccept {
		return mcp.NewToolResultError(fmt.Sprintf("User %s approval for %s. Action not performed.", result.Action, toolName))
	}

	content, ok := result.Content.(map[string]any)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("User approval response for %s did not include valid confirmation content. Action not performed.", toolName))
	}
	confirmed, ok := content[approvalFieldName].(bool)
	if !ok || !confirmed {
		return mcp.NewToolResultError(fmt.Sprintf("User did not confirm approval for %s. Action not performed.", toolName))
	}

	return nil
}

func elicitationSupportError(ctx context.Context) error {
	session := mcpserver.ClientSessionFromContext(ctx)
	if session == nil {
		return mcpserver.ErrNoActiveSession
	}

	capabilitySession, ok := session.(mcpserver.SessionWithClientInfo)
	if !ok {
		return mcpserver.ErrElicitationNotSupported
	}

	capabilities := capabilitySession.GetClientCapabilities().Elicitation
	if capabilities == nil {
		return mcpserver.ErrElicitationNotSupported
	}
	if capabilities.Form == nil && capabilities.URL != nil {
		return mcpserver.ErrElicitationNotSupported
	}

	return nil
}

func elicitationErrorMessage(toolName string, err error) string {
	switch {
	case errors.Is(err, mcpserver.ErrElicitationNotSupported):
		return fmt.Sprintf("User approval is required for %s, but the MCP client does not support form-mode elicitation. Action not performed.", toolName)
	case errors.Is(err, mcpserver.ErrNoActiveSession):
		return fmt.Sprintf("User approval is required for %s, but there is no active MCP session for elicitation. Action not performed.", toolName)
	default:
		return fmt.Sprintf("User approval is required for %s, but MCP elicitation failed: %v. Action not performed.", toolName, err)
	}
}

// RequiresElicitation reports whether a tool must request user approval.
func RequiresElicitation(toolName string) bool {
	for _, prefix := range elicitationProtectedToolPrefixes {
		if toolName == prefix || strings.HasPrefix(toolName, prefix+"_") {
			return true
		}
	}
	return false
}

func buildApprovalMessage(toolName string, req mcp.CallToolRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Approve Webex action %s?", toolName)

	args := formatArgumentPreview(req.GetArguments())
	if args != "" {
		b.WriteString("\n\nArguments:\n")
		b.WriteString(args)
	}

	return b.String()
}

func formatArgumentPreview(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}

	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	if len(keys) > maxPreviewArgs {
		keys = keys[:maxPreviewArgs]
	}

	lines := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("- %s: %s", key, previewValue(key, args[key])))
	}
	if len(args) > maxPreviewArgs {
		lines = append(lines, fmt.Sprintf("- ...: %d more argument(s)", len(args)-maxPreviewArgs))
	}

	return strings.Join(lines, "\n")
}

func previewValue(key string, value any) string {
	if value == nil {
		return "null"
	}

	lowerKey := strings.ToLower(key)
	if strings.Contains(lowerKey, "base64") {
		if raw, ok := value.(string); ok {
			return fmt.Sprintf("<base64 omitted, %d characters>", len(raw))
		}
		return "<base64 omitted>"
	}

	var text string
	if raw, ok := value.(string); ok {
		text = raw
	} else if data, err := json.Marshal(value); err == nil {
		text = string(data)
	} else {
		text = fmt.Sprintf("%v", value)
	}

	text = strings.ReplaceAll(text, "\n", "\\n")
	text = strings.ReplaceAll(text, "\r", "\\r")
	if len(text) > maxPreviewLength {
		text = text[:maxPreviewLength] + "..."
	}
	return text
}

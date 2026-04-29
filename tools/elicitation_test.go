package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type captureRegistrar struct {
	tool    mcp.Tool
	handler mcpserver.ToolHandlerFunc
}

func (r *captureRegistrar) AddTool(tool mcp.Tool, handler mcpserver.ToolHandlerFunc) {
	r.tool = tool
	r.handler = handler
}

type basicTestSession struct {
	sessionID    string
	notify       chan mcp.JSONRPCNotification
	clientInfo   mcp.Implementation
	capabilities mcp.ClientCapabilities
}

func (s *basicTestSession) Initialize() {}

func (s *basicTestSession) Initialized() bool {
	return true
}

func (s *basicTestSession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	if s.notify == nil {
		s.notify = make(chan mcp.JSONRPCNotification, 1)
	}
	return s.notify
}

func (s *basicTestSession) SessionID() string {
	return s.sessionID
}

func (s *basicTestSession) GetClientInfo() mcp.Implementation {
	return s.clientInfo
}

func (s *basicTestSession) SetClientInfo(clientInfo mcp.Implementation) {
	s.clientInfo = clientInfo
}

func (s *basicTestSession) GetClientCapabilities() mcp.ClientCapabilities {
	return s.capabilities
}

func (s *basicTestSession) SetClientCapabilities(clientCapabilities mcp.ClientCapabilities) {
	s.capabilities = clientCapabilities
}

type elicitationTestSession struct {
	basicTestSession
	result      *mcp.ElicitationResult
	lastRequest mcp.ElicitationRequest
	calls       int
}

func (s *elicitationTestSession) RequestElicitation(ctx context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	s.calls++
	s.lastRequest = request
	return s.result, nil
}

func TestRequiresElicitation(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "webex_messages_create", want: true},
		{name: "webex_messages_create_as_logged_in_user", want: true},
		{name: "webex_messages_send_attachment_as_logged_in_user", want: true},
		{name: "webex_rooms_update", want: true},
		{name: "webex_meetings_patch", want: true},
		{name: "webex_webhooks_delete", want: true},
		{name: "webex_transcripts_update_snippet", want: true},
		{name: "webex_messages_list", want: false},
		{name: "webex_messages_get", want: false},
		{name: "webex_find_messages_like_in_room", want: false},
		{name: "webex_transcripts_download", want: false},
		{name: "webex_uploads_request_url", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresElicitation(tt.name); got != tt.want {
				t.Fatalf("RequiresElicitation(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestElicitingRegistrarAcceptRunsHandler(t *testing.T) {
	mcpServer := mcpserver.NewMCPServer("test", "1.0", mcpserver.WithElicitation())
	capture := &captureRegistrar{}
	registrar := NewElicitingRegistrar(mcpServer, capture)

	called := false
	registrar.AddTool(mcp.NewTool("webex_messages_create"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("sent"), nil
	})

	session := &elicitationTestSession{
		basicTestSession: basicTestSession{
			sessionID: "session-1",
			capabilities: mcp.ClientCapabilities{
				Elicitation: &mcp.ElicitationCapability{},
			},
		},
		result: &mcp.ElicitationResult{
			ElicitationResponse: mcp.ElicitationResponse{
				Action: mcp.ElicitationResponseActionAccept,
				Content: map[string]any{
					approvalFieldName: true,
				},
			},
		},
	}
	ctx := mcpServer.WithContext(context.Background(), session)

	result, err := capture.handler(ctx, callRequest("webex_messages_create", map[string]any{
		"toPersonEmail": "user@example.com",
		"text":          "hello",
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("result.IsError = true, want false")
	}
	if !called {
		t.Fatal("wrapped handler was not called after approval")
	}
	if session.calls != 1 {
		t.Fatalf("elicitation calls = %d, want 1", session.calls)
	}
	if !strings.Contains(session.lastRequest.Params.Message, "webex_messages_create") {
		t.Fatalf("approval message %q does not include tool name", session.lastRequest.Params.Message)
	}
	if !strings.Contains(session.lastRequest.Params.Message, "toPersonEmail") {
		t.Fatalf("approval message %q does not include argument preview", session.lastRequest.Params.Message)
	}
}

func TestElicitingRegistrarDeclineBlocksHandler(t *testing.T) {
	mcpServer := mcpserver.NewMCPServer("test", "1.0", mcpserver.WithElicitation())
	capture := &captureRegistrar{}
	registrar := NewElicitingRegistrar(mcpServer, capture)

	called := false
	registrar.AddTool(mcp.NewTool("webex_rooms_delete"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("deleted"), nil
	})

	session := &elicitationTestSession{
		basicTestSession: basicTestSession{
			sessionID: "session-1",
			capabilities: mcp.ClientCapabilities{
				Elicitation: &mcp.ElicitationCapability{},
			},
		},
		result: &mcp.ElicitationResult{
			ElicitationResponse: mcp.ElicitationResponse{
				Action: mcp.ElicitationResponseActionDecline,
			},
		},
	}
	ctx := mcpServer.WithContext(context.Background(), session)

	result, err := capture.handler(ctx, callRequest("webex_rooms_delete", map[string]any{"roomId": "room-1"}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.IsError = false, want true")
	}
	if called {
		t.Fatal("wrapped handler was called after decline")
	}
}

func TestElicitingRegistrarUnsupportedClientFailsClosed(t *testing.T) {
	mcpServer := mcpserver.NewMCPServer("test", "1.0", mcpserver.WithElicitation())
	capture := &captureRegistrar{}
	registrar := NewElicitingRegistrar(mcpServer, capture)

	called := false
	registrar.AddTool(mcp.NewTool("webex_webhooks_create"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("created"), nil
	})

	ctx := mcpServer.WithContext(context.Background(), &basicTestSession{sessionID: "session-1"})

	result, err := capture.handler(ctx, callRequest("webex_webhooks_create", map[string]any{"name": "hook"}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.IsError = false, want true")
	}
	if called {
		t.Fatal("wrapped handler was called when elicitation was unsupported")
	}
	if !strings.Contains(toolResultText(result), "does not support form-mode elicitation") {
		t.Fatalf("result text %q does not mention unsupported elicitation", toolResultText(result))
	}
}

func TestElicitingRegistrarURLOnlyClientFailsClosed(t *testing.T) {
	mcpServer := mcpserver.NewMCPServer("test", "1.0", mcpserver.WithElicitation())
	capture := &captureRegistrar{}
	registrar := NewElicitingRegistrar(mcpServer, capture)

	called := false
	registrar.AddTool(mcp.NewTool("webex_messages_delete"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("deleted"), nil
	})

	ctx := mcpServer.WithContext(context.Background(), &basicTestSession{
		sessionID: "session-1",
		capabilities: mcp.ClientCapabilities{
			Elicitation: &mcp.ElicitationCapability{URL: &struct{}{}},
		},
	})

	result, err := capture.handler(ctx, callRequest("webex_messages_delete", map[string]any{"messageId": "msg-1"}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.IsError = false, want true")
	}
	if called {
		t.Fatal("wrapped handler was called when form-mode elicitation was unsupported")
	}
}

func TestElicitingRegistrarReadToolBypassesElicitation(t *testing.T) {
	mcpServer := mcpserver.NewMCPServer("test", "1.0", mcpserver.WithElicitation())
	capture := &captureRegistrar{}
	registrar := NewElicitingRegistrar(mcpServer, capture)

	called := false
	registrar.AddTool(mcp.NewTool("webex_messages_list"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("listed"), nil
	})

	result, err := capture.handler(context.Background(), callRequest("webex_messages_list", map[string]any{"roomId": "room-1"}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("result.IsError = true, want false")
	}
	if !called {
		t.Fatal("read handler was not called")
	}
}

func callRequest(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

func toolResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if text, ok := result.Content[0].(mcp.TextContent); ok {
		return text.Text
	}
	return ""
}

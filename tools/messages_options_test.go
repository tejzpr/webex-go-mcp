package tools

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type recordingRegistrar struct {
	tools map[string]mcp.Tool
}

func (r *recordingRegistrar) AddTool(tool mcp.Tool, handler server.ToolHandlerFunc) {
	if r.tools == nil {
		r.tools = make(map[string]mcp.Tool)
	}
	r.tools[tool.Name] = tool
}

func TestRegisterMessageToolsSTDIOAttachmentSchema(t *testing.T) {
	registrar := &recordingRegistrar{}
	RegisterMessageTools(registrar, nil)

	attachment := registrar.tools["webex_messages_send_attachment"]
	if _, ok := attachment.InputSchema.Properties["localFilePath"]; !ok {
		t.Fatal("STDIO attachment schema should include localFilePath")
	}
	if _, ok := attachment.InputSchema.Properties["uploadId"]; ok {
		t.Fatal("STDIO attachment schema should not include uploadId")
	}
	if _, ok := registrar.tools["webex_uploads_request_url"]; ok {
		t.Fatal("STDIO should not register webex_uploads_request_url")
	}
}

func TestRegisterMessageToolsHTTPAttachmentSchema(t *testing.T) {
	uploads, err := NewUploadManager("http://localhost:8560", t.TempDir(), 1024)
	if err != nil {
		t.Fatalf("NewUploadManager() error = %v", err)
	}
	defer uploads.Close()

	registrar := &recordingRegistrar{}
	RegisterMessageTools(registrar, nil, MessageToolOptions{
		AllowLocalFilePath: false,
		Uploads:            uploads,
	})

	attachment := registrar.tools["webex_messages_send_attachment"]
	if _, ok := attachment.InputSchema.Properties["localFilePath"]; ok {
		t.Fatal("HTTP attachment schema should not include localFilePath")
	}
	if _, ok := attachment.InputSchema.Properties["uploadId"]; !ok {
		t.Fatal("HTTP attachment schema should include uploadId")
	}
	if _, ok := registrar.tools["webex_uploads_request_url"]; !ok {
		t.Fatal("HTTP should register webex_uploads_request_url")
	}
	if strings.Contains(attachment.Description, "localFilePath") {
		t.Fatalf("HTTP attachment description should not mention localFilePath: %s", attachment.Description)
	}
	if strings.Contains(registrar.tools["webex_uploads_request_url"].Description, "localFilePath") {
		t.Fatalf("HTTP upload URL description should not mention localFilePath: %s", registrar.tools["webex_uploads_request_url"].Description)
	}
}

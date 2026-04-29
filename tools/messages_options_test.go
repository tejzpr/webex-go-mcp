package tools

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
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

	adaptiveCard := registrar.tools["webex_messages_send_adaptive_card"]
	if strings.Contains(adaptiveCard.Description, "/tmp/chart.png") {
		t.Fatalf("HTTP adaptive card description should not recommend local paths: %s", adaptiveCard.Description)
	}
	if !strings.Contains(adaptiveCard.Description, cardUploadURLPrefix+"<uploadId>") {
		t.Fatalf("HTTP adaptive card description should mention %s placeholders: %s", cardUploadURLPrefix, adaptiveCard.Description)
	}
	if !strings.Contains(adaptiveCard.Description, "Do not use the files URL") {
		t.Fatalf("HTTP adaptive card description should warn against Webex content URLs: %s", adaptiveCard.Description)
	}
}

func TestResolveCardURLsHTTPUploadIDToDataURI(t *testing.T) {
	uploads, err := NewUploadManager("http://localhost:8560", t.TempDir(), 1024)
	if err != nil {
		t.Fatalf("NewUploadManager() error = %v", err)
	}
	defer uploads.Close()

	reservation, err := uploads.RequestUpload("chart.png", "image/png")
	if err != nil {
		t.Fatalf("RequestUpload() error = %v", err)
	}

	imageBytes := []byte("fake-png")
	req := httptest.NewRequest(http.MethodPut, reservation.UploadURL, strings.NewReader(string(imageBytes)))
	req.Header.Set("Content-Type", "image/png")
	rr := httptest.NewRecorder()
	uploads.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rr.Code, rr.Body.String())
	}

	card := map[string]interface{}{
		"type": "Image",
		"url":  cardUploadURLPrefix + reservation.UploadID,
	}
	err = resolveCardURLs(card, MessageToolOptions{
		AllowLocalFilePath: false,
		Uploads:            uploads,
	})
	if err != nil {
		t.Fatalf("resolveCardURLs() error = %v", err)
	}

	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	if card["url"] != want {
		t.Fatalf("card url = %q, want %q", card["url"], want)
	}
}

func TestResolveCardURLsHTTPRejectsLocalPath(t *testing.T) {
	card := map[string]interface{}{
		"type": "Image",
		"url":  "/tmp/chart.png",
	}
	err := resolveCardURLs(card, MessageToolOptions{
		AllowLocalFilePath: false,
	})
	if err == nil {
		t.Fatal("resolveCardURLs() error = nil, want local path error")
	}
	if !strings.Contains(err.Error(), "only supported in STDIO mode") {
		t.Fatalf("resolveCardURLs() error = %v, want STDIO guidance", err)
	}
}

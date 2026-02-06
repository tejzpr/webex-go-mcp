package main

import (
	"webex-go-mcp/tools"

	webex "github.com/tejzpr/webex-go-sdk/v2"

	"github.com/mark3labs/mcp-go/server"
)

func startServer(webexClient *webex.WebexClient) error {
	s := server.NewMCPServer(
		"webex-mcp",
		version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	// Register all tool groups
	tools.RegisterMessageTools(s, webexClient)
	tools.RegisterRoomTools(s, webexClient)
	tools.RegisterTeamTools(s, webexClient)
	tools.RegisterMembershipTools(s, webexClient)
	tools.RegisterMeetingTools(s, webexClient)
	tools.RegisterTranscriptTools(s, webexClient)
	tools.RegisterWebhookTools(s, webexClient)

	// Serve over STDIO
	return server.ServeStdio(s)
}

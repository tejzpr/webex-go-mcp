package main

import (
	"log"

	"github.com/tejzpr/webex-go-mcp/tools"

	webex "github.com/tejzpr/webex-go-sdk/v2"

	"github.com/mark3labs/mcp-go/server"
)

func startServer(webexClient *webex.WebexClient, include, exclude string) error {
	s := server.NewMCPServer(
		"webex-mcp",
		version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	// Build the tool registrar — either filtered or direct
	filter := tools.NewToolFilter(include, exclude)
	var registrar tools.ToolRegistrar

	if filter.IsActive() {
		fr := tools.NewFilteredRegistrar(s, filter)
		registrar = fr
		defer func() {
			registered, skipped := fr.Stats()
			log.Printf("Tool filtering active: %d registered, %d skipped", registered, skipped)
		}()
	} else {
		registrar = s
	}

	// Register all tool groups
	tools.RegisterMessageTools(registrar, webexClient)
	tools.RegisterRoomTools(registrar, webexClient)
	tools.RegisterTeamTools(registrar, webexClient)
	tools.RegisterMembershipTools(registrar, webexClient)
	tools.RegisterMeetingTools(registrar, webexClient)
	tools.RegisterTranscriptTools(registrar, webexClient)
	tools.RegisterWebhookTools(registrar, webexClient)

	// Serve over STDIO
	return server.ServeStdio(s)
}

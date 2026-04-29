package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/WebexCommunity/webex-go-sdk/v2/people"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
)

// RegisterPeopleTools registers safe people lookup tools.
func RegisterPeopleTools(s ToolRegistrar, resolver auth.ClientResolver) {
	s.AddTool(
		mcp.NewTool("webex_people_get",
			mcp.WithDescription("Look up a Webex person by personId or email. This does not list the directory; provide exactly one lookup key.\n"+
				"\n"+
				"Use personId='me' to get the authenticated Webex identity."),
			mcp.WithString("personId", mcp.Description("Webex person ID to retrieve. Use 'me' for the authenticated Webex identity.")),
			mcp.WithString("email", mcp.Description("Email address to look up. Returns matching people visible to the authenticated Webex identity.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			personID := req.GetString("personId", "")
			email := req.GetString("email", "")
			if (personID == "" && email == "") || (personID != "" && email != "") {
				return mcp.NewToolResultError("Provide exactly one of 'personId' or 'email'"), nil
			}

			var response interface{}
			if personID != "" {
				person, getErr := client.People().Get(personID)
				if getErr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to get person: %v", getErr)), nil
				}
				response = compactPerson(person)
			} else {
				page, listErr := client.People().List(&people.ListOptions{
					Email: email,
					Max:   10,
				})
				if listErr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to look up person by email: %v", listErr)), nil
				}
				matches := make([]map[string]interface{}, 0, len(page.Items))
				for i := range page.Items {
					matches = append(matches, compactPerson(&page.Items[i]))
				}
				response = map[string]interface{}{
					"people": matches,
					"count":  len(matches),
					"email":  email,
				}
			}

			data, _ := json.MarshalIndent(response, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

func compactPerson(person *people.Person) map[string]interface{} {
	if person == nil {
		return nil
	}
	return map[string]interface{}{
		"id":          person.ID,
		"displayName": person.DisplayName,
		"emails":      person.Emails,
		"type":        person.Type,
		"status":      person.Status,
	}
}

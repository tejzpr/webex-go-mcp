package tools

import (
	"encoding/json"
	"fmt"

	webex "github.com/WebexCommunity/webex-go-sdk/v2"
	"github.com/WebexCommunity/webex-go-sdk/v2/webexsdk"
)

const (
	// PageSize is the fixed number of items per page for all list tools.
	PageSize = 10
)

// FetchPage fetches a page directly from a next-page URL using the SDK's PageFromCursor.
// The nextPageUrl is the raw Webex API Link rel="next" URL from a previous response.
// Returns the raw Page (with []json.RawMessage items).
func FetchPage(client *webex.WebexClient, nextPageUrl string) (*webexsdk.Page, error) {
	if nextPageUrl == "" {
		return nil, fmt.Errorf("empty nextPageUrl")
	}
	return client.Core().PageFromCursor(nextPageUrl)
}

// UnmarshalPageItems unmarshals raw Page items ([]json.RawMessage) into a typed slice.
func UnmarshalPageItems[T any](page *webexsdk.Page) ([]T, error) {
	items := make([]T, len(page.Items))
	for i, raw := range page.Items {
		if err := json.Unmarshal(raw, &items[i]); err != nil {
			return nil, fmt.Errorf("failed to unmarshal item %d: %w", i, err)
		}
	}
	return items, nil
}

// PaginatedResponse is the standard response wrapper for all list tools.
type PaginatedResponse struct {
	Items       interface{} `json:"items"`
	TotalItems  int         `json:"totalItems"`
	HasNextPage bool        `json:"hasNextPage"`
	NextPageUrl string      `json:"nextPageUrl,omitempty"`
}

// FormatPaginatedResponse builds the standard paginated JSON response.
func FormatPaginatedResponse(items interface{}, hasNextPage bool, nextPageUrl string) (string, error) {
	// Count items
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("failed to marshal items: %w", err)
	}
	var raw []json.RawMessage
	totalItems := 0
	if err := json.Unmarshal(itemsJSON, &raw); err == nil {
		totalItems = len(raw)
	}

	resp := PaginatedResponse{
		Items:       items,
		TotalItems:  totalItems,
		HasNextPage: hasNextPage,
		NextPageUrl: nextPageUrl,
	}

	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}
	return string(data), nil
}

// AddPaginationToMap adds pagination fields to an existing response map.
// Use this for tools that return wrapper objects (e.g. messages: {room, messages}).
func AddPaginationToMap(response map[string]interface{}, hasNextPage bool, nextPageUrl string) {
	response["hasNextPage"] = hasNextPage
	if nextPageUrl != "" {
		response["nextPageUrl"] = nextPageUrl
	}
}

// PaginationDescription is the standard instruction block appended to all list tool descriptions.
const PaginationDescription = "\n\n" +
	"PAGINATION: Returns up to 10 items per request. The response includes:\n" +
	"- 'hasNextPage' (boolean): true if more results are available.\n" +
	"- 'nextPageUrl' (string): the Webex API URL to fetch the next page (RFC 5988 Link rel=\"next\").\n" +
	"\n" +
	"To get more results: call this same tool again, passing the 'nextPageUrl' value from the response as the 'nextPageUrl' parameter.\n" +
	"Stop when 'hasNextPage' is false or 'nextPageUrl' is absent.\n" +
	"Do NOT modify the nextPageUrl value -- pass it exactly as received.\n" +
	"\n" +
	"Example flow:\n" +
	"1. First call: omit nextPageUrl → returns first page, hasNextPage=true, nextPageUrl='https://webexapis.com/v1/...'\n" +
	"2. Next call: pass nextPageUrl='https://webexapis.com/v1/...' → next page, hasNextPage=true, nextPageUrl='https://...'\n" +
	"3. Last call: → final page, hasNextPage=false → done."

// NextPageUrlParamDescription is the standard description for the 'nextPageUrl' tool parameter.
const NextPageUrlParamDescription = "The nextPageUrl value from a previous response. Pass this exactly as received to fetch the next page. Omit on the first call."

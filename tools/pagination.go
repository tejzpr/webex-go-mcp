package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	webex "github.com/WebexCommunity/webex-go-sdk/v2"
	"github.com/WebexCommunity/webex-go-sdk/v2/webexsdk"
)

const (
	// PageSize is the fixed number of items per page for all list tools.
	PageSize = 10
)

// EncodeCursor encodes a NextPage URL into an opaque cursor string.
func EncodeCursor(nextURL string) string {
	if nextURL == "" {
		return ""
	}
	return base64.URLEncoding.EncodeToString([]byte(nextURL))
}

// DecodeCursor decodes an opaque cursor back into a NextPage URL.
func DecodeCursor(cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	b, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", fmt.Errorf("invalid cursor: %w", err)
	}
	return string(b), nil
}

// FetchPageFromCursor fetches a page directly from a cursor using the SDK's PageFromCursor.
// Returns the raw Page (with []json.RawMessage items).
func FetchPageFromCursor(client *webex.WebexClient, cursor string) (*webexsdk.Page, error) {
	cursorURL, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	if cursorURL == "" {
		return nil, fmt.Errorf("empty cursor")
	}
	return client.Core().PageFromCursor(cursorURL)
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
	Items      interface{} `json:"items"`
	TotalItems int         `json:"totalItems"`
	HasMore    bool        `json:"hasMore"`
	Cursor     string      `json:"cursor,omitempty"`
}

// FormatPaginatedResponse builds the standard paginated JSON response.
func FormatPaginatedResponse(items interface{}, hasMore bool, nextURL string) (string, error) {
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
		Items:      items,
		TotalItems: totalItems,
		HasMore:    hasMore,
		Cursor:     EncodeCursor(nextURL),
	}

	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}
	return string(data), nil
}

// AddPaginationToMap adds pagination fields to an existing response map.
// Use this for tools that return wrapper objects (e.g. messages: {room, messages}).
func AddPaginationToMap(response map[string]interface{}, hasMore bool, nextURL string) {
	response["hasMore"] = hasMore
	cursor := EncodeCursor(nextURL)
	if cursor != "" {
		response["cursor"] = cursor
	}
}

// PaginationDescription is the standard instruction block appended to all list tool descriptions.
const PaginationDescription = "\n\n" +
	"PAGINATION: Returns up to 10 items per request. The response includes:\n" +
	"- 'hasMore' (boolean): true if more results exist beyond this page.\n" +
	"- 'cursor' (string): an opaque token to fetch the next page.\n" +
	"\n" +
	"To get more results: call this same tool again with cursor=<cursor value from the previous response>.\n" +
	"Stop when 'hasMore' is false or 'cursor' is absent.\n" +
	"Do NOT modify the cursor value -- pass it exactly as received.\n" +
	"\n" +
	"Example flow:\n" +
	"1. Call with no cursor → get first 10 items, hasMore=true, cursor='abc...'\n" +
	"2. Call with cursor='abc...' → get next 10 items, hasMore=true, cursor='def...'\n" +
	"3. Call with cursor='def...' → get last items, hasMore=false → done."

// CursorParamDescription is the standard description for the 'cursor' tool parameter.
const CursorParamDescription = "Pagination cursor from a previous response. Pass this value exactly as received to fetch the next page of results. Do not pass this on the first call."

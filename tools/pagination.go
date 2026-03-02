package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	webex "github.com/WebexCommunity/webex-go-sdk/v2"
	"github.com/WebexCommunity/webex-go-sdk/v2/webexsdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tejzpr/webex-go-mcp/auth"
)

const (
	// PageSize is the number of items per Webex API call (one "page").
	PageSize = 10

	// DefaultMaxResults is the default number of items the server auto-fetches
	// across multiple pages before returning to the caller.
	DefaultMaxResults = 50

	// MaxResultsCap is the absolute upper limit for maxResults.
	MaxResultsCap = 200
)

// FetchPage fetches a page directly from a next-page URL using the SDK's PageFromCursor.
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

// AutoPaginate fetches additional pages from the Webex API until maxResults is
// reached or no more pages exist. It starts from the initial page results and
// appends subsequent pages transparently.
func AutoPaginate[T any](
	initialItems []T,
	hasNext bool,
	nextURL string,
	client *webex.WebexClient,
	maxResults int,
) (items []T, finalHasNext bool, finalNextURL string, err error) {
	if maxResults <= 0 {
		maxResults = DefaultMaxResults
	}
	if maxResults > MaxResultsCap {
		maxResults = MaxResultsCap
	}

	items = initialItems
	finalHasNext = hasNext
	finalNextURL = nextURL

	for len(items) < maxResults && finalHasNext && finalNextURL != "" {
		page, pErr := FetchPage(client, finalNextURL)
		if pErr != nil {
			log.Printf("[AutoPaginate] failed to fetch page: %v", pErr)
			return items, finalHasNext, finalNextURL, nil
		}

		pageItems, uErr := UnmarshalPageItems[T](page)
		if uErr != nil {
			log.Printf("[AutoPaginate] failed to unmarshal page: %v", uErr)
			return items, finalHasNext, finalNextURL, nil
		}

		items = append(items, pageItems...)
		finalHasNext = page.HasNext
		finalNextURL = page.NextPage
	}

	// Trim to maxResults if we over-fetched
	if len(items) > maxResults {
		items = items[:maxResults]
		finalHasNext = true
	}

	return items, finalHasNext, finalNextURL, nil
}

// ClampMaxResults reads the maxResults parameter from the request and clamps it
// to [1, MaxResultsCap], defaulting to DefaultMaxResults.
func ClampMaxResults(req mcp.CallToolRequest) int {
	v := req.GetInt("maxResults", DefaultMaxResults)
	if v <= 0 {
		return DefaultMaxResults
	}
	if v > MaxResultsCap {
		return MaxResultsCap
	}
	return v
}

// --- Response formatting with _pagination at the top ---

// PaginationMeta is placed first in the response JSON so LLMs see it immediately.
type PaginationMeta struct {
	Returned    int    `json:"returned"`
	HasMore     bool   `json:"hasMore"`
	Message     string `json:"message"`
	NextPageUrl string `json:"nextPageUrl,omitempty"`
}

// PaginatedResponse is the standard response wrapper for all list tools.
type PaginatedResponse struct {
	Pagination PaginationMeta `json:"_pagination"`
	Items      interface{}    `json:"items"`
}

func buildPaginationMeta(itemCount int, hasMore bool, nextPageUrl string) PaginationMeta {
	meta := PaginationMeta{
		Returned:    itemCount,
		HasMore:     hasMore,
		NextPageUrl: nextPageUrl,
	}

	if hasMore {
		meta.Message = fmt.Sprintf(
			"Showing %d items. More results available. To get more: (1) re-call with a higher maxResults (up to %d), or (2) call webex_fetch_next_page with the nextPageUrl.",
			itemCount, MaxResultsCap,
		)
	} else {
		meta.Message = fmt.Sprintf("All %d results returned.", itemCount)
	}
	return meta
}

// FormatPaginatedResponse builds the standard paginated JSON response.
func FormatPaginatedResponse(items interface{}, hasNextPage bool, nextPageUrl string) (string, error) {
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
		Pagination: buildPaginationMeta(totalItems, hasNextPage, nextPageUrl),
		Items:      items,
	}

	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}
	return string(data), nil
}

// AddPaginationToMap adds the _pagination block to an existing response map.
func AddPaginationToMap(response map[string]interface{}, itemCount int, hasNextPage bool, nextPageUrl string) {
	response["_pagination"] = buildPaginationMeta(itemCount, hasNextPage, nextPageUrl)
}

// --- Compact / field trimming ---

// TrimFields strips a map down to only the listed keys. Unknown keys are dropped.
func TrimFields(m map[string]interface{}, keep []string) map[string]interface{} {
	keepSet := make(map[string]struct{}, len(keep))
	for _, k := range keep {
		keepSet[k] = struct{}{}
	}
	out := make(map[string]interface{}, len(keep))
	for _, k := range keep {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	// preserve keys not in the original (shouldn't happen, but safe)
	_ = keepSet
	return out
}

// TrimSlice applies TrimFields to every element in a slice of maps.
func TrimSlice(items []map[string]interface{}, keep []string) []map[string]interface{} {
	out := make([]map[string]interface{}, len(items))
	for i, m := range items {
		out[i] = TrimFields(m, keep)
	}
	return out
}

// --- Tool descriptions ---

// PaginationDescription is appended to all list tool descriptions.
const PaginationDescription = "\n\n" +
	"PAGINATION: Returns up to 50 items by default (server auto-fetches multiple pages). " +
	"Set maxResults up to 200 for more. " +
	"If the response shows hasMore=true, you can: " +
	"(1) re-call with a higher maxResults, or " +
	"(2) call webex_fetch_next_page with the provided nextPageUrl. " +
	"Most queries are satisfied by the default."

// MaxResultsParamDescription is the standard description for the maxResults parameter.
const MaxResultsParamDescription = "Max items to return (default 50, max 200). The server auto-fetches multiple pages internally. Increase only if you need more results."

// NextPageUrlParamDescription is the standard description for the 'nextPageUrl' tool parameter.
const NextPageUrlParamDescription = "Resume pagination from a previous response. Pass the nextPageUrl value exactly as received. Omit on the first call."

// CompactParamDescription is the standard description for the compact parameter.
const CompactParamDescription = "When true, returns only essential fields per item (id, title/text, key metadata). Reduces token usage for large result sets."

// --- Universal next-page helper tool ---

// RegisterPaginationTools registers the universal webex_fetch_next_page helper tool.
func RegisterPaginationTools(s ToolRegistrar, resolver auth.ClientResolver) {
	s.AddTool(
		mcp.NewTool("webex_fetch_next_page",
			mcp.WithDescription("Universal pagination helper. Fetch the next page of results from ANY list tool.\n"+
				"\n"+
				"When a list tool response has hasMore=true, pass its nextPageUrl here to get the next batch of raw items. "+
				"This is simpler than re-calling the original tool with nextPageUrl.\n"+
				"\n"+
				"Returns raw Webex API items (no enrichment). For enriched results, re-call the original list tool with nextPageUrl instead."),
			mcp.WithString("nextPageUrl", mcp.Required(), mcp.Description("The nextPageUrl from a previous list tool response. Pass exactly as received.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := resolver(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Auth error: %v", err)), nil
			}

			nextPageUrl, err := req.RequireString("nextPageUrl")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			page, pErr := FetchPage(client, nextPageUrl)
			if pErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch page: %v", pErr)), nil
			}

			// Return raw items with pagination metadata
			result, fErr := FormatPaginatedResponse(page.Items, page.HasNext, page.NextPage)
			if fErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", fErr)), nil
			}
			return mcp.NewToolResultText(result), nil
		},
	)
}

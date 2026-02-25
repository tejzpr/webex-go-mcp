package tools

import (
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"

	webex "github.com/WebexCommunity/webex-go-sdk/v2"
)

// maxTextFileSize is the maximum size of a text file to include inline (100KB).
const maxTextFileSize = 100 * 1024

// FileInfo holds metadata (and optionally content) about a message file attachment.
type FileInfo struct {
	URL         string `json:"url"`
	FileName    string `json:"fileName,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Content     string `json:"content,omitempty"` // populated for text-based files only
}

// resolvePersonName returns the displayName for a personID, or "" on failure.
func resolvePersonName(client *webex.WebexClient, personID string) string {
	if personID == "" {
		return ""
	}
	person, err := client.People().Get(personID)
	if err != nil {
		log.Printf("Enrichment: failed to resolve person %s: %v", personID, err)
		return ""
	}
	return person.DisplayName
}

// RoomInfo holds basic room information for enrichment.
type RoomInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type,omitempty"`
}

// resolveRoomInfo returns basic room info for a roomID, or nil on failure.
func resolveRoomInfo(client *webex.WebexClient, roomID string) *RoomInfo {
	if roomID == "" {
		return nil
	}
	room, err := client.Rooms().Get(roomID)
	if err != nil {
		log.Printf("Enrichment: failed to resolve room %s: %v", roomID, err)
		return nil
	}
	return &RoomInfo{
		ID:    room.ID,
		Title: room.Title,
		Type:  room.Type,
	}
}

// resolveTeamName returns the team name for a teamID, or "" on failure.
func resolveTeamName(client *webex.WebexClient, teamID string) string {
	if teamID == "" {
		return ""
	}
	team, err := client.Teams().Get(teamID)
	if err != nil {
		log.Printf("Enrichment: failed to resolve team %s: %v", teamID, err)
		return ""
	}
	return team.Name
}

// isTextContentType returns true if the content type is text-based and suitable for inline inclusion.
func isTextContentType(ct string) bool {
	ct = strings.ToLower(ct)
	if strings.HasPrefix(ct, "text/") {
		return true
	}
	textTypes := []string{
		"application/json",
		"application/xml",
		"application/javascript",
		"application/x-yaml",
		"application/yaml",
		"application/toml",
		"application/csv",
	}
	for _, t := range textTypes {
		if strings.HasPrefix(ct, t) {
			return true
		}
	}
	return false
}

// parseContentDisposition extracts the filename from a Content-Disposition header value.
func parseContentDisposition(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// makeAuthenticatedRequest creates an HTTP request with the Webex auth token.
func makeAuthenticatedRequest(client *webex.WebexClient, method, url string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.Core().GetAccessToken()))
	return client.Core().GetHTTPClient().Do(req)
}

// resolveFileMetadata does a HEAD request on a Webex content URL to get filename, size, content-type.
// Returns nil on failure.
func resolveFileMetadata(client *webex.WebexClient, fileURL string) *FileInfo {
	if fileURL == "" {
		return nil
	}

	resp, err := makeAuthenticatedRequest(client, http.MethodHead, fileURL)
	if err != nil {
		log.Printf("Enrichment: failed HEAD request for file %s: %v", fileURL, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("Enrichment: HEAD request for file %s returned %d", fileURL, resp.StatusCode)
		return nil
	}

	info := &FileInfo{URL: fileURL}
	info.ContentType = resp.Header.Get("Content-Type")
	info.FileName = parseContentDisposition(resp.Header.Get("Content-Disposition"))
	if sizeStr := resp.Header.Get("Content-Length"); sizeStr != "" {
		if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
			info.Size = size
		}
	}

	return info
}

// resolveFileContent does a GET request and returns content for text-based files.
// For binary files, it falls back to metadata only (HEAD). Caps text content at maxTextFileSize.
func resolveFileContent(client *webex.WebexClient, fileURL string) *FileInfo {
	if fileURL == "" {
		return nil
	}

	// First, HEAD to check content type and size
	info := resolveFileMetadata(client, fileURL)
	if info == nil {
		return nil
	}

	// Only download text-based files within size limit
	if !isTextContentType(info.ContentType) {
		return info // metadata only for binary files
	}
	if info.Size > maxTextFileSize && info.Size > 0 {
		log.Printf("Enrichment: text file %s too large (%d bytes), returning metadata only", fileURL, info.Size)
		return info
	}

	// GET the content
	resp, err := makeAuthenticatedRequest(client, http.MethodGet, fileURL)
	if err != nil {
		log.Printf("Enrichment: failed GET request for file %s: %v", fileURL, err)
		return info // return metadata we already have
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("Enrichment: GET request for file %s returned %d", fileURL, resp.StatusCode)
		return info
	}

	// Read content with size cap
	limited := io.LimitReader(resp.Body, maxTextFileSize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		log.Printf("Enrichment: failed to read file %s: %v", fileURL, err)
		return info
	}

	if len(body) > maxTextFileSize {
		info.Content = string(body[:maxTextFileSize]) + "\n... [truncated at 100KB] ..."
	} else {
		info.Content = string(body)
	}

	return info
}

// PersonNameCache is a simple cache for person ID -> display name lookups to avoid redundant API calls.
type PersonNameCache struct {
	client *webex.WebexClient
	cache  map[string]string
}

// NewPersonNameCache creates a new cache.
func NewPersonNameCache(client *webex.WebexClient) *PersonNameCache {
	return &PersonNameCache{
		client: client,
		cache:  make(map[string]string),
	}
}

// Resolve returns the display name for a person ID, using the cache.
func (c *PersonNameCache) Resolve(personID string) string {
	if personID == "" {
		return ""
	}
	if name, ok := c.cache[personID]; ok {
		return name
	}
	name := resolvePersonName(c.client, personID)
	c.cache[personID] = name
	return name
}

// TeamNameCache is a simple cache for team ID -> name lookups.
type TeamNameCache struct {
	client *webex.WebexClient
	cache  map[string]string
}

// NewTeamNameCache creates a new cache.
func NewTeamNameCache(client *webex.WebexClient) *TeamNameCache {
	return &TeamNameCache{
		client: client,
		cache:  make(map[string]string),
	}
}

// Resolve returns the team name for a team ID, using the cache.
func (c *TeamNameCache) Resolve(teamID string) string {
	if teamID == "" {
		return ""
	}
	if name, ok := c.cache[teamID]; ok {
		return name
	}
	name := resolveTeamName(c.client, teamID)
	c.cache[teamID] = name
	return name
}

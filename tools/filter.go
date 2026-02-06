package tools

import (
	"log"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolRegistrar is the interface used to register MCP tools.
// *server.MCPServer satisfies this interface.
type ToolRegistrar interface {
	AddTool(tool mcp.Tool, handler server.ToolHandlerFunc)
}

// ToolFilter determines which tools should be registered based on
// include/exclude lists provided via CLI flags or environment variables.
type ToolFilter struct {
	include map[string]bool
	exclude map[string]bool
}

// NewToolFilter creates a ToolFilter from comma-separated include/exclude strings.
// If both include and exclude are non-empty, include takes priority and exclude is ignored.
//
// Filter entries use "category:action" shorthand (e.g. "messages:list") which maps
// to the full tool name "webex_messages_list". Singular forms are also accepted
// (e.g. "message:list" maps to "webex_messages_list").
func NewToolFilter(include, exclude string) *ToolFilter {
	f := &ToolFilter{
		include: parseFilterList(include),
		exclude: parseFilterList(exclude),
	}

	// Include takes priority — if both are set, ignore exclude
	if len(f.include) > 0 && len(f.exclude) > 0 {
		log.Println("Both --include and --exclude specified; --include takes priority, --exclude ignored")
		f.exclude = nil
	}

	return f
}

// ShouldRegister returns true if the given tool name should be registered.
func (f *ToolFilter) ShouldRegister(toolName string) bool {
	if len(f.include) > 0 {
		return f.include[toolName]
	}
	if len(f.exclude) > 0 {
		return !f.exclude[toolName]
	}
	return true
}

// IsActive returns true if any filtering is configured.
func (f *ToolFilter) IsActive() bool {
	return len(f.include) > 0 || len(f.exclude) > 0
}

// FilteredRegistrar wraps a ToolRegistrar and only registers tools that pass the filter.
type FilteredRegistrar struct {
	inner      ToolRegistrar
	filter     *ToolFilter
	registered int
	skipped    int
}

// NewFilteredRegistrar creates a FilteredRegistrar wrapping the given ToolRegistrar.
func NewFilteredRegistrar(inner ToolRegistrar, filter *ToolFilter) *FilteredRegistrar {
	return &FilteredRegistrar{
		inner:  inner,
		filter: filter,
	}
}

// AddTool registers the tool only if it passes the filter.
func (fr *FilteredRegistrar) AddTool(tool mcp.Tool, handler server.ToolHandlerFunc) {
	if fr.filter.ShouldRegister(tool.Name) {
		fr.inner.AddTool(tool, handler)
		fr.registered++
	} else {
		fr.skipped++
		log.Printf("Skipping tool: %s (filtered out)", tool.Name)
	}
}

// Stats returns the number of registered and skipped tools.
func (fr *FilteredRegistrar) Stats() (registered, skipped int) {
	return fr.registered, fr.skipped
}

// parseFilterList parses a comma-separated filter string into a set of normalized tool names.
func parseFilterList(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	result := make(map[string]bool)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		for _, name := range normalizeToolName(entry) {
			result[name] = true
		}
	}
	return result
}

// normalizeToolName converts a filter entry into one or more candidate full tool names.
//
// Supported formats:
//   - "category:action"  -> "webex_category_action" (also tries plural category)
//   - "webex_category_action" -> passed through as-is
func normalizeToolName(entry string) []string {
	// If it already looks like a full tool name, use as-is
	if strings.HasPrefix(entry, "webex_") {
		return []string{entry}
	}

	// Split on ":" — expect "category:action"
	parts := strings.SplitN(entry, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		// Doesn't match expected format; try as a literal tool name
		log.Printf("Warning: filter entry %q doesn't match category:action format, using as literal", entry)
		return []string{entry}
	}

	category := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])

	// Replace any remaining ":" or "-" in the action with "_"
	action = strings.ReplaceAll(action, ":", "_")
	action = strings.ReplaceAll(action, "-", "_")

	candidates := []string{
		"webex_" + category + "_" + action,
	}

	// Also try plural form if the category doesn't already end in "s"
	if !strings.HasSuffix(category, "s") {
		candidates = append(candidates, "webex_"+category+"s_"+action)
	}

	return candidates
}

// Preset tool lists for --minimal and --readonly-minimal flags.
// These are full tool names (webex_{category}_{action}).
var (
	// PresetMinimal includes all tools for messages, rooms, teams, meetings, and transcripts.
	// Excludes memberships and webhooks.
	PresetMinimal = []string{
		"webex_messages_list", "webex_messages_create", "webex_messages_send_attachment", "webex_messages_get", "webex_messages_delete",
		"webex_rooms_list", "webex_rooms_create", "webex_rooms_get", "webex_rooms_update", "webex_rooms_delete",
		"webex_teams_list", "webex_teams_create", "webex_teams_get", "webex_teams_update",
		"webex_meetings_list", "webex_meetings_create", "webex_meetings_get", "webex_meetings_update", "webex_meetings_delete",
		"webex_transcripts_list", "webex_transcripts_download", "webex_transcripts_list_snippets", "webex_transcripts_get_snippet", "webex_transcripts_update_snippet",
	}

	// PresetReadonlyMinimal includes only read/GET tools for messages, rooms, teams, meetings, and transcripts.
	// No create, update, or delete operations.
	PresetReadonlyMinimal = []string{
		"webex_messages_list", "webex_messages_get",
		"webex_rooms_list", "webex_rooms_get",
		"webex_teams_list", "webex_teams_get",
		"webex_meetings_list", "webex_meetings_get",
		"webex_transcripts_list", "webex_transcripts_download", "webex_transcripts_list_snippets", "webex_transcripts_get_snippet",
	}
)

// ResolvePresets merges preset flags into the include string.
// Preset tools are added to the include list (they don't override --include or --exclude).
// If both minimal and readonlyMinimal are true, minimal takes priority (it's a superset).
func ResolvePresets(minimal, readonlyMinimal bool, include string) string {
	var preset []string
	switch {
	case minimal:
		preset = PresetMinimal
		log.Println("--minimal flag active: adding minimal tool set to include list")
	case readonlyMinimal:
		preset = PresetReadonlyMinimal
		log.Println("--readonly-minimal flag active: adding readonly-minimal tool set to include list")
	default:
		return include
	}

	// Merge: prepend preset tools to whatever the user already specified in --include
	presetStr := strings.Join(preset, ",")
	if include == "" {
		return presetStr
	}
	return presetStr + "," + include
}

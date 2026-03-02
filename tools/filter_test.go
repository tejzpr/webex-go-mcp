package tools

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type mockRegistrar struct {
	count int
}

func (m *mockRegistrar) AddTool(tool mcp.Tool, handler server.ToolHandlerFunc) {
	m.count++
}

func TestNewToolFilter_EmptyStrings(t *testing.T) {
	f := NewToolFilter("", "")
	if f.IsActive() {
		t.Error("filter should not be active with empty strings")
	}
	if !f.ShouldRegister("webex_messages_list") {
		t.Error("ShouldRegister should be true when filter is not active")
	}
}

func TestNewToolFilter_SingleInclude(t *testing.T) {
	f := NewToolFilter("messages:list", "")
	if !f.ShouldRegister("webex_messages_list") {
		t.Error("ShouldRegister(webex_messages_list) should be true")
	}
	if f.ShouldRegister("webex_rooms_list") {
		t.Error("ShouldRegister(webex_rooms_list) should be false")
	}
}

func TestNewToolFilter_MultipleIncludes(t *testing.T) {
	f := NewToolFilter("messages:list, rooms:get", "")
	if !f.ShouldRegister("webex_messages_list") {
		t.Error("webex_messages_list should pass")
	}
	if !f.ShouldRegister("webex_rooms_get") {
		t.Error("webex_rooms_get should pass")
	}
	if f.ShouldRegister("webex_messages_delete") {
		t.Error("webex_messages_delete should fail")
	}
	if f.ShouldRegister("webex_rooms_list") {
		t.Error("webex_rooms_list should fail")
	}
}

func TestNewToolFilter_Exclude(t *testing.T) {
	f := NewToolFilter("", "messages:delete")
	if f.ShouldRegister("webex_messages_delete") {
		t.Error("webex_messages_delete should be false")
	}
	if !f.ShouldRegister("webex_messages_list") {
		t.Error("webex_messages_list should be true")
	}
	if !f.ShouldRegister("webex_rooms_list") {
		t.Error("webex_rooms_list should be true")
	}
}

func TestNewToolFilter_BothIncludeAndExclude(t *testing.T) {
	// Include wins, exclude is ignored
	f := NewToolFilter("messages:list", "messages:delete")
	if !f.ShouldRegister("webex_messages_list") {
		t.Error("webex_messages_list should pass (include wins)")
	}
	// With include set, exclude is ignored - so messages:delete would NOT be in the exclude map
	// (because exclude is cleared when both are set). So webex_messages_delete would check include,
	// find it false, and return false. So webex_messages_delete should be false (not in include).
	if f.ShouldRegister("webex_messages_delete") {
		t.Error("webex_messages_delete should be false (not in include)")
	}
}

func TestNormalizeToolName_MessagesList(t *testing.T) {
	candidates := normalizeToolName("messages:list")
	if len(candidates) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0] != "webex_messages_list" {
		t.Errorf("expected webex_messages_list, got %q", candidates[0])
	}
}

func TestNormalizeToolName_MessageListSingular(t *testing.T) {
	candidates := normalizeToolName("message:list")
	if len(candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d: %v", len(candidates), candidates)
	}
	expected := map[string]bool{
		"webex_message_list":  true,
		"webex_messages_list": true,
	}
	for _, c := range candidates {
		if !expected[c] {
			t.Errorf("unexpected candidate %q", c)
		}
	}
}

func TestNormalizeToolName_Passthrough(t *testing.T) {
	candidates := normalizeToolName("webex_rooms_list")
	if len(candidates) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0] != "webex_rooms_list" {
		t.Errorf("expected webex_rooms_list, got %q", candidates[0])
	}
}

func TestNormalizeToolName_InvalidLiteral(t *testing.T) {
	candidates := normalizeToolName("invalid")
	if len(candidates) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0] != "invalid" {
		t.Errorf("expected invalid (literal passthrough), got %q", candidates[0])
	}
}

func TestFilteredRegistrar_AddTool(t *testing.T) {
	mock := &mockRegistrar{}
	filter := NewToolFilter("webex_test_tool", "")
	fr := NewFilteredRegistrar(mock, filter)

	fr.AddTool(mcp.Tool{Name: "webex_test_tool"}, nil)
	fr.AddTool(mcp.Tool{Name: "webex_other_tool"}, nil)

	reg, skip := fr.Stats()
	if reg != 1 || skip != 1 {
		t.Errorf("Stats() = (%d, %d), want (1, 1)", reg, skip)
	}
	if mock.count != 1 {
		t.Errorf("mock registrar AddTool called %d times, want 1", mock.count)
	}
}

func TestResolvePresets_MinimalFalseReadonlyMinimalFalse(t *testing.T) {
	include := "webex_custom_tool"
	got := ResolvePresets(false, false, include)
	if got != include {
		t.Errorf("ResolvePresets(false, false, %q) = %q, want %q", include, got, include)
	}
}

func TestResolvePresets_MinimalTrue(t *testing.T) {
	got := ResolvePresets(true, false, "")
	expected := strings.Join(PresetMinimal, ",")
	if got != expected {
		t.Errorf("ResolvePresets(true, false, \"\") = %q, want %q", got, expected)
	}
}

func TestResolvePresets_MinimalTrueWithExistingInclude(t *testing.T) {
	include := "webex_custom_tool"
	got := ResolvePresets(true, false, include)
	expected := strings.Join(PresetMinimal, ",") + "," + include
	if got != expected {
		t.Errorf("ResolvePresets(true, false, %q) = %q, want %q", include, got, expected)
	}
}

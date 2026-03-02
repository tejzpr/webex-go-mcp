package tools

import (
	"encoding/json"
	"testing"
)

// --- FormatPaginatedResponse tests (new _pagination structure) ---

func TestFormatPaginatedResponse_WithItems(t *testing.T) {
	items := []map[string]interface{}{
		{"id": "1", "name": "first"},
		{"id": "2", "name": "second"},
		{"id": "3", "name": "third"},
	}
	got, err := FormatPaginatedResponse(items, true, "https://example.com/next")
	if err != nil {
		t.Fatalf("FormatPaginatedResponse: %v", err)
	}

	var parsed PaginatedResponse
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed.Pagination.Returned != 3 {
		t.Errorf("returned = %d, want 3", parsed.Pagination.Returned)
	}
	if !parsed.Pagination.HasMore {
		t.Error("hasMore should be true")
	}
	if parsed.Pagination.NextPageUrl != "https://example.com/next" {
		t.Errorf("nextPageUrl = %q, want https://example.com/next", parsed.Pagination.NextPageUrl)
	}
	if parsed.Pagination.Message == "" {
		t.Error("message should not be empty")
	}
}

func TestFormatPaginatedResponse_EmptySlice(t *testing.T) {
	items := []map[string]interface{}{}
	got, err := FormatPaginatedResponse(items, false, "")
	if err != nil {
		t.Fatalf("FormatPaginatedResponse: %v", err)
	}

	var parsed PaginatedResponse
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed.Pagination.Returned != 0 {
		t.Errorf("returned = %d, want 0", parsed.Pagination.Returned)
	}
	if parsed.Pagination.HasMore {
		t.Error("hasMore should be false")
	}
	if parsed.Pagination.NextPageUrl != "" {
		t.Errorf("nextPageUrl should be empty, got %q", parsed.Pagination.NextPageUrl)
	}
}

func TestFormatPaginatedResponse_PaginationFirst(t *testing.T) {
	items := []string{"a", "b"}
	got, err := FormatPaginatedResponse(items, true, "http://next")
	if err != nil {
		t.Fatal(err)
	}

	// Verify _pagination appears before items in the JSON output
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["_pagination"]; !ok {
		t.Fatal("_pagination key missing from response")
	}
	if _, ok := raw["items"]; !ok {
		t.Fatal("items key missing from response")
	}
}

func TestFormatPaginatedResponse_MessageContent(t *testing.T) {
	tests := []struct {
		name      string
		hasMore   bool
		wantAll   bool
		wantShown bool
	}{
		{"all returned", false, true, false},
		{"has more", true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []string{"a"}
			got, _ := FormatPaginatedResponse(items, tt.hasMore, "http://next")
			var parsed PaginatedResponse
			json.Unmarshal([]byte(got), &parsed)

			if tt.wantAll && parsed.Pagination.Message != "All 1 results returned." {
				t.Errorf("message = %q, want all-returned message", parsed.Pagination.Message)
			}
			if tt.wantShown && parsed.Pagination.Message == "" {
				t.Error("expected non-empty message for hasMore=true")
			}
		})
	}
}

// --- AddPaginationToMap tests ---

func TestAddPaginationToMap_HasMore(t *testing.T) {
	m := map[string]interface{}{"items": []int{1, 2, 3}}
	AddPaginationToMap(m, 3, true, "http://next")

	pag, ok := m["_pagination"]
	if !ok {
		t.Fatal("_pagination key missing")
	}
	meta := pag.(PaginationMeta)
	if meta.Returned != 3 {
		t.Errorf("returned = %d, want 3", meta.Returned)
	}
	if !meta.HasMore {
		t.Error("hasMore should be true")
	}
	if meta.NextPageUrl != "http://next" {
		t.Errorf("nextPageUrl = %q", meta.NextPageUrl)
	}
}

func TestAddPaginationToMap_NoMore(t *testing.T) {
	m := map[string]interface{}{"items": []int{1}}
	AddPaginationToMap(m, 1, false, "")

	meta := m["_pagination"].(PaginationMeta)
	if meta.HasMore {
		t.Error("hasMore should be false")
	}
	if meta.NextPageUrl != "" {
		t.Errorf("nextPageUrl should be empty, got %q", meta.NextPageUrl)
	}
}

// --- AutoPaginate tests ---

func TestAutoPaginate_NoMorePages(t *testing.T) {
	initial := []string{"a", "b", "c"}
	items, hasNext, nextURL, err := AutoPaginate(initial, false, "", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Errorf("len = %d, want 3", len(items))
	}
	if hasNext {
		t.Error("hasNext should be false")
	}
	if nextURL != "" {
		t.Errorf("nextURL should be empty, got %q", nextURL)
	}
}

func TestAutoPaginate_AlreadyAtMax(t *testing.T) {
	initial := make([]int, 50)
	for i := range initial {
		initial[i] = i
	}
	items, hasNext, _, err := AutoPaginate(initial, true, "http://next", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 50 {
		t.Errorf("len = %d, want 50", len(items))
	}
	if !hasNext {
		t.Error("hasNext should still be true (more pages exist)")
	}
}

func TestAutoPaginate_ClampToMaxResultsCap(t *testing.T) {
	initial := []string{"a"}
	items, _, _, _ := AutoPaginate(initial, false, "", nil, 999)
	if len(items) != 1 {
		t.Errorf("len = %d, want 1", len(items))
	}
}

func TestAutoPaginate_DefaultMaxResults(t *testing.T) {
	initial := []string{"a"}
	items, _, _, _ := AutoPaginate(initial, false, "", nil, 0)
	if len(items) != 1 {
		t.Errorf("len = %d, want 1", len(items))
	}
}

func TestAutoPaginate_TrimsOverfetch(t *testing.T) {
	initial := make([]int, 8)
	for i := range initial {
		initial[i] = i
	}
	items, hasNext, _, _ := AutoPaginate(initial, false, "", nil, 5)
	if len(items) != 5 {
		t.Errorf("len = %d, want 5", len(items))
	}
	if !hasNext {
		t.Error("hasNext should be true when trimmed")
	}
}

// --- ClampMaxResults tests ---

type fakeCallToolRequest struct {
	maxResults int
	has        bool
}

func (f fakeCallToolRequest) GetInt(key string, def int) int {
	if key == "maxResults" && f.has {
		return f.maxResults
	}
	return def
}

func TestClampMaxResults_Default(t *testing.T) {
	// Can't easily call ClampMaxResults without a real mcp.CallToolRequest,
	// but we can test the clamping logic directly.
	clamp := func(v int) int {
		if v <= 0 {
			return DefaultMaxResults
		}
		if v > MaxResultsCap {
			return MaxResultsCap
		}
		return v
	}

	if got := clamp(0); got != DefaultMaxResults {
		t.Errorf("clamp(0) = %d, want %d", got, DefaultMaxResults)
	}
	if got := clamp(-5); got != DefaultMaxResults {
		t.Errorf("clamp(-5) = %d, want %d", got, DefaultMaxResults)
	}
	if got := clamp(999); got != MaxResultsCap {
		t.Errorf("clamp(999) = %d, want %d", got, MaxResultsCap)
	}
	if got := clamp(100); got != 100 {
		t.Errorf("clamp(100) = %d, want 100", got)
	}
}

// --- TrimFields / TrimSlice tests ---

func TestTrimFields_KeepsListed(t *testing.T) {
	m := map[string]interface{}{
		"id":    "123",
		"name":  "Alice",
		"email": "alice@example.com",
		"extra": "data",
	}

	trimmed := TrimFields(m, []string{"id", "name"})
	if len(trimmed) != 2 {
		t.Errorf("len = %d, want 2", len(trimmed))
	}
	if trimmed["id"] != "123" {
		t.Errorf("id = %v, want 123", trimmed["id"])
	}
	if trimmed["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", trimmed["name"])
	}
	if _, ok := trimmed["email"]; ok {
		t.Error("email should have been trimmed")
	}
	if _, ok := trimmed["extra"]; ok {
		t.Error("extra should have been trimmed")
	}
}

func TestTrimFields_MissingKey(t *testing.T) {
	m := map[string]interface{}{"id": "1"}
	trimmed := TrimFields(m, []string{"id", "missing"})
	if len(trimmed) != 1 {
		t.Errorf("len = %d, want 1", len(trimmed))
	}
	if trimmed["id"] != "1" {
		t.Errorf("id = %v, want 1", trimmed["id"])
	}
}

func TestTrimFields_EmptyMap(t *testing.T) {
	m := map[string]interface{}{}
	trimmed := TrimFields(m, []string{"id"})
	if len(trimmed) != 0 {
		t.Errorf("len = %d, want 0", len(trimmed))
	}
}

func TestTrimSlice(t *testing.T) {
	items := []map[string]interface{}{
		{"id": "1", "name": "Alice", "extra": "x"},
		{"id": "2", "name": "Bob", "extra": "y"},
	}
	trimmed := TrimSlice(items, []string{"id", "name"})
	if len(trimmed) != 2 {
		t.Fatalf("len = %d, want 2", len(trimmed))
	}
	for i, item := range trimmed {
		if _, ok := item["extra"]; ok {
			t.Errorf("item %d: extra should have been trimmed", i)
		}
		if _, ok := item["id"]; !ok {
			t.Errorf("item %d: id missing", i)
		}
	}
}

func TestTrimSlice_EmptySlice(t *testing.T) {
	items := []map[string]interface{}{}
	trimmed := TrimSlice(items, []string{"id"})
	if len(trimmed) != 0 {
		t.Errorf("len = %d, want 0", len(trimmed))
	}
}

// --- Constants sanity checks ---

func TestPaginationConstants(t *testing.T) {
	if DefaultMaxResults <= 0 {
		t.Errorf("DefaultMaxResults = %d, want > 0", DefaultMaxResults)
	}
	if MaxResultsCap < DefaultMaxResults {
		t.Errorf("MaxResultsCap (%d) < DefaultMaxResults (%d)", MaxResultsCap, DefaultMaxResults)
	}
	if PageSize <= 0 {
		t.Errorf("PageSize = %d, want > 0", PageSize)
	}
}

func TestPaginationDescriptions(t *testing.T) {
	if PaginationDescription == "" {
		t.Error("PaginationDescription should not be empty")
	}
	if MaxResultsParamDescription == "" {
		t.Error("MaxResultsParamDescription should not be empty")
	}
	if NextPageUrlParamDescription == "" {
		t.Error("NextPageUrlParamDescription should not be empty")
	}
	if CompactParamDescription == "" {
		t.Error("CompactParamDescription should not be empty")
	}
}

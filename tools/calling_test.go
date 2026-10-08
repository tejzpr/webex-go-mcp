package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	webex "github.com/WebexCommunity/webex-go-sdk/v2"
	"github.com/WebexCommunity/webex-go-sdk/v2/calling"
	"github.com/WebexCommunity/webex-go-sdk/v2/webexsdk"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type callingCaptureRegistrar struct {
	tools    map[string]mcp.Tool
	handlers map[string]mcpserver.ToolHandlerFunc
}

func (r *callingCaptureRegistrar) AddTool(tool mcp.Tool, handler mcpserver.ToolHandlerFunc) {
	if r.tools == nil {
		r.tools = make(map[string]mcp.Tool)
	}
	if r.handlers == nil {
		r.handlers = make(map[string]mcpserver.ToolHandlerFunc)
	}
	r.tools[tool.Name] = tool
	r.handlers[tool.Name] = handler
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRegisterCallingTools(t *testing.T) {
	registrar := &callingCaptureRegistrar{}
	RegisterCallingTools(registrar, nil)

	for _, name := range []string{
		"webex_calling_get_availability",
		"webex_calling_set_availability",
		"webex_calling_get_call_forwarding",
		"webex_calling_set_call_forwarding",
	} {
		if _, ok := registrar.tools[name]; !ok {
			t.Fatalf("tool %q was not registered", name)
		}
		if _, ok := registrar.handlers[name]; !ok {
			t.Fatalf("handler for %q was not registered", name)
		}
	}

	availability := registrar.tools["webex_calling_set_availability"]
	if _, ok := availability.InputSchema.Properties["available"]; !ok {
		t.Fatal("set availability schema should include available")
	}
	if len(availability.InputSchema.Required) != 1 || availability.InputSchema.Required[0] != "available" {
		t.Fatalf("set availability required fields = %v, want [available]", availability.InputSchema.Required)
	}

	forwarding := registrar.tools["webex_calling_set_call_forwarding"]
	for _, field := range []string{"alwaysEnabled", "busyEnabled", "noAnswerEnabled", "businessContinuityEnabled"} {
		if _, ok := forwarding.InputSchema.Properties[field]; !ok {
			t.Fatalf("set call forwarding schema should include %s", field)
		}
	}
}

func TestCallingSetAvailabilitySetsDNDInverse(t *testing.T) {
	var gotBody calling.ToggleSetting
	client := newCallingTestClient(t, func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", req.Method)
		}
		if req.URL.Path != "/v1/people/me/features/doNotDisturb" {
			t.Fatalf("path = %s, want /v1/people/me/features/doNotDisturb", req.URL.Path)
		}
		if err := json.NewDecoder(req.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		return jsonHTTPResponse(http.StatusOK, `{"enabled": false}`), nil
	})

	registrar := &callingCaptureRegistrar{}
	RegisterCallingTools(registrar, resolverFor(client))

	result, err := registrar.handlers["webex_calling_set_availability"](
		context.Background(),
		callRequest("webex_calling_set_availability", map[string]any{"available": true}),
	)
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned tool error: %s", toolResultText(result))
	}
	if gotBody.Enabled {
		t.Fatal("DND enabled = true, want false when available=true")
	}
	if !strings.Contains(toolResultText(result), `"available": true`) {
		t.Fatalf("result should include available=true: %s", toolResultText(result))
	}
}

func TestCallingSetCallForwardingPreservesUnspecifiedRules(t *testing.T) {
	var putBody calling.CallForwardSetting
	requests := 0
	client := newCallingTestClient(t, func(req *http.Request) (*http.Response, error) {
		requests++
		switch req.Method {
		case http.MethodGet:
			if req.URL.Path != "/v1/people/me/features/callForwarding" {
				t.Fatalf("GET path = %s, want /v1/people/me/features/callForwarding", req.URL.Path)
			}
			return jsonHTTPResponse(http.StatusOK, `{
				"callForwarding": {
					"always": {"enabled": false, "destination": "+15550000000"},
					"busy": {"enabled": true, "destination": "+15551110000"},
					"noAnswer": {"enabled": true, "destination": "+15552220000", "numberOfRings": 4}
				},
				"businessContinuity": {"enabled": true, "destination": "+15553330000"}
			}`), nil
		case http.MethodPut:
			if req.URL.Path != "/v1/people/me/features/callForwarding" {
				t.Fatalf("PUT path = %s, want /v1/people/me/features/callForwarding", req.URL.Path)
			}
			if err := json.NewDecoder(req.Body).Decode(&putBody); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			return jsonHTTPResponse(http.StatusOK, `{}`), nil
		default:
			t.Fatalf("unexpected method: %s", req.Method)
		}
		return nil, nil
	})

	registrar := &callingCaptureRegistrar{}
	RegisterCallingTools(registrar, resolverFor(client))

	result, err := registrar.handlers["webex_calling_set_call_forwarding"](
		context.Background(),
		callRequest("webex_calling_set_call_forwarding", map[string]any{
			"alwaysEnabled":         true,
			"alwaysDestination":     "+15554440000",
			"noAnswerNumberOfRings": 6,
		}),
	)
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned tool error: %s", toolResultText(result))
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}

	if !putBody.CallForwarding.Always.Enabled {
		t.Fatal("always forwarding should be enabled")
	}
	if putBody.CallForwarding.Always.Destination != "+15554440000" {
		t.Fatalf("always destination = %q, want +15554440000", putBody.CallForwarding.Always.Destination)
	}
	if !putBody.CallForwarding.Busy.Enabled || putBody.CallForwarding.Busy.Destination != "+15551110000" {
		t.Fatalf("busy rule was not preserved: %+v", putBody.CallForwarding.Busy)
	}
	if !putBody.CallForwarding.NoAnswer.Enabled || putBody.CallForwarding.NoAnswer.Destination != "+15552220000" {
		t.Fatalf("no-answer rule was not preserved: %+v", putBody.CallForwarding.NoAnswer)
	}
	if putBody.CallForwarding.NoAnswer.NumberOfRings == nil || *putBody.CallForwarding.NoAnswer.NumberOfRings != 6 {
		t.Fatalf("no-answer rings = %v, want 6", putBody.CallForwarding.NoAnswer.NumberOfRings)
	}
	if !putBody.BusinessContinuity.Enabled || putBody.BusinessContinuity.Destination != "+15553330000" {
		t.Fatalf("business continuity rule was not preserved: %+v", putBody.BusinessContinuity)
	}
}

func TestCallingSetCallForwardingRejectsEmptyUpdate(t *testing.T) {
	putCalled := false
	client := newCallingTestClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			return jsonHTTPResponse(http.StatusOK, `{"callForwarding": {"always": {"enabled": false}}, "businessContinuity": {"enabled": false}}`), nil
		case http.MethodPut:
			putCalled = true
			return jsonHTTPResponse(http.StatusOK, `{}`), nil
		default:
			t.Fatalf("unexpected method: %s", req.Method)
		}
		return nil, nil
	})

	registrar := &callingCaptureRegistrar{}
	RegisterCallingTools(registrar, resolverFor(client))

	result, err := registrar.handlers["webex_calling_set_call_forwarding"](
		context.Background(),
		callRequest("webex_calling_set_call_forwarding", map[string]any{}),
	)
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("handler should return a tool error for empty updates: %s", toolResultText(result))
	}
	if putCalled {
		t.Fatal("PUT should not be called for an empty update")
	}
}

func TestCallingSetCallForwardingRejectsInvalidRings(t *testing.T) {
	putCalled := false
	client := newCallingTestClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			return jsonHTTPResponse(http.StatusOK, `{"callForwarding": {"noAnswer": {"enabled": true, "numberOfRings": 4}}, "businessContinuity": {"enabled": false}}`), nil
		case http.MethodPut:
			putCalled = true
			return jsonHTTPResponse(http.StatusOK, `{}`), nil
		default:
			t.Fatalf("unexpected method: %s", req.Method)
		}
		return nil, nil
	})

	registrar := &callingCaptureRegistrar{}
	RegisterCallingTools(registrar, resolverFor(client))

	result, err := registrar.handlers["webex_calling_set_call_forwarding"](
		context.Background(),
		callRequest("webex_calling_set_call_forwarding", map[string]any{"noAnswerNumberOfRings": 0}),
	)
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("handler should return a tool error for invalid rings: %s", toolResultText(result))
	}
	if !strings.Contains(toolResultText(result), "noAnswerNumberOfRings must be greater than 0") {
		t.Fatalf("unexpected error text: %s", toolResultText(result))
	}
	if putCalled {
		t.Fatal("PUT should not be called for invalid rings")
	}
}

func newCallingTestClient(t *testing.T, fn roundTripFunc) *webex.WebexClient {
	t.Helper()
	client, err := webex.NewClient("test-token", &webexsdk.Config{
		HttpClient: &http.Client{Transport: fn},
	})
	if err != nil {
		t.Fatalf("webex.NewClient() error = %v", err)
	}
	return client
}

func resolverFor(client *webex.WebexClient) func(context.Context) (*webex.WebexClient, error) {
	return func(context.Context) (*webex.WebexClient, error) {
		return client, nil
	}
}

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

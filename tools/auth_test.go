package tools

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tejzpr/webex-go-mcp/auth"
)

func newTestLocalOAuthManager(t *testing.T) *auth.LocalOAuthManager {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	mgr, err := auth.NewLocalOAuthManager(auth.LocalOAuthConfig{
		OAuth: &auth.OAuthConfig{
			ClientID:     "client",
			ClientSecret: "secret",
			RedirectURI:  fmt.Sprintf("http://127.0.0.1:%d/callback", port),
		},
		TokenFile:   filepath.Join(t.TempDir(), "token.json"),
		OpenBrowser: false,
		LoginWait:   -1,
		LoginTTL:    time.Minute,
	})
	if err != nil {
		t.Fatalf("NewLocalOAuthManager() error = %v", err)
	}
	t.Cleanup(mgr.Close)
	return mgr
}

func TestRegisterLocalAuthTools(t *testing.T) {
	mgr := newTestLocalOAuthManager(t)
	reg := &callingCaptureRegistrar{}
	RegisterLocalAuthTools(reg, mgr)

	for _, name := range []string{"webex_auth_status", "webex_auth_login", "webex_auth_logout"} {
		if _, ok := reg.handlers[name]; !ok {
			t.Fatalf("tool %s not registered", name)
		}
	}

	ctx := context.Background()

	res, err := reg.handlers["webex_auth_status"](ctx, callRequest("webex_auth_status", nil))
	if err != nil || res.IsError {
		t.Fatalf("status: %v %v", res, err)
	}
	if text := toolResultText(res); !strings.Contains(text, `"authenticated": false`) {
		t.Errorf("status text = %s, want authenticated false", text)
	}

	res, err = reg.handlers["webex_auth_login"](ctx, callRequest("webex_auth_login", map[string]any{"waitSeconds": float64(0)}))
	if err != nil || res.IsError {
		t.Fatalf("login: %v %v", toolResultText(res), err)
	}
	if text := toolResultText(res); !strings.Contains(text, `"loginUrl"`) || !strings.Contains(text, "/login") {
		t.Errorf("login text = %s, want loginUrl", text)
	}
	if !mgr.Status().LoginInProgress {
		t.Error("expected sign-in to be in progress after webex_auth_login")
	}

	res, err = reg.handlers["webex_auth_logout"](ctx, callRequest("webex_auth_logout", nil))
	if err != nil || res.IsError {
		t.Fatalf("logout: %v %v", toolResultText(res), err)
	}
	if mgr.Status().LoginInProgress {
		t.Error("logout should cancel the pending sign-in")
	}
}

func TestRegisterLocalAuthToolsNilManager(t *testing.T) {
	reg := &callingCaptureRegistrar{}
	RegisterLocalAuthTools(reg, nil)
	if len(reg.handlers) != 0 {
		t.Errorf("expected no tools for nil manager, got %d", len(reg.handlers))
	}
}

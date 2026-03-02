package streaming

import (
	"regexp"
	"testing"
	"time"
)

func TestNewMercuryManager(t *testing.T) {
	m := NewMercuryManager(nil)
	if m == nil {
		t.Fatal("NewMercuryManager(nil) returned nil")
	}
	if m.subscriptions == nil {
		t.Error("subscriptions map is not initialized")
	}
	if m.userConns == nil {
		t.Error("userConns map is not initialized")
	}
	if len(m.subscriptions) != 0 {
		t.Errorf("subscriptions map should be empty, got len=%d", len(m.subscriptions))
	}
	if len(m.userConns) != 0 {
		t.Errorf("userConns map should be empty, got len=%d", len(m.userConns))
	}
}

func TestListSubscriptions_FiltersBySessionID(t *testing.T) {
	m := NewMercuryManager(nil)

	sub1 := &Subscription{
		ID:        "sub1",
		RoomID:    "room1",
		TokenHash: "hash1",
		SessionID: "session1",
		CreatedAt: time.Now(),
		cancel:    func() {},
	}
	sub2 := &Subscription{
		ID:        "sub2",
		RoomID:    "room2",
		TokenHash: "hash2",
		SessionID: "session1",
		CreatedAt: time.Now(),
		cancel:    func() {},
	}
	sub3 := &Subscription{
		ID:        "sub3",
		RoomID:    "room3",
		TokenHash: "hash3",
		SessionID: "session2",
		CreatedAt: time.Now(),
		cancel:    func() {},
	}

	m.mu.Lock()
	m.subscriptions["sub1"] = sub1
	m.subscriptions["sub2"] = sub2
	m.subscriptions["sub3"] = sub3
	m.mu.Unlock()

	// Filter by session1
	subs := m.ListSubscriptions("session1")
	if len(subs) != 2 {
		t.Errorf("ListSubscriptions(\"session1\") expected 2 subs, got %d", len(subs))
	}
	for _, s := range subs {
		if s.SessionID != "session1" {
			t.Errorf("expected SessionID session1, got %q", s.SessionID)
		}
	}

	// Filter by session2
	subs = m.ListSubscriptions("session2")
	if len(subs) != 1 {
		t.Errorf("ListSubscriptions(\"session2\") expected 1 sub, got %d", len(subs))
	}
	if len(subs) > 0 && subs[0].ID != "sub3" {
		t.Errorf("expected sub3, got %q", subs[0].ID)
	}

	// Non-existent session
	subs = m.ListSubscriptions("session99")
	if len(subs) != 0 {
		t.Errorf("ListSubscriptions(\"session99\") expected 0 subs, got %d", len(subs))
	}
}

func TestListSubscriptions_EmptySessionIDReturnsAll(t *testing.T) {
	m := NewMercuryManager(nil)

	sub1 := &Subscription{
		ID:        "sub1",
		RoomID:    "room1",
		TokenHash: "hash1",
		SessionID: "session1",
		CreatedAt: time.Now(),
		cancel:    func() {},
	}
	sub2 := &Subscription{
		ID:        "sub2",
		RoomID:    "room2",
		TokenHash: "hash2",
		SessionID: "session2",
		CreatedAt: time.Now(),
		cancel:    func() {},
	}

	m.mu.Lock()
	m.subscriptions["sub1"] = sub1
	m.subscriptions["sub2"] = sub2
	m.mu.Unlock()

	subs := m.ListSubscriptions("")
	if len(subs) != 2 {
		t.Errorf("ListSubscriptions(\"\") expected 2 subs (all), got %d", len(subs))
	}
}

func TestListSubscriptions_EmptyMap(t *testing.T) {
	m := NewMercuryManager(nil)

	subs := m.ListSubscriptions("")
	if len(subs) != 0 {
		t.Errorf("ListSubscriptions(\"\") on empty manager expected 0 subs, got %d", len(subs))
	}

	subs = m.ListSubscriptions("session1")
	if len(subs) != 0 {
		t.Errorf("ListSubscriptions(\"session1\") on empty manager expected 0 subs, got %d", len(subs))
	}
}

func TestUnsubscribe_NonExistentID(t *testing.T) {
	m := NewMercuryManager(nil)

	err := m.Unsubscribe("non-existent-id")
	if err == nil {
		t.Fatal("Unsubscribe(non-existent-id) expected error, got nil")
	}
	if err.Error() != "subscription non-existent-id not found" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	h1 := hashToken("test-token")
	h2 := hashToken("test-token")
	if h1 != h2 {
		t.Errorf("hashToken(\"test-token\") not deterministic: %q != %q", h1, h2)
	}
}

func TestHashToken_HexString64Chars(t *testing.T) {
	h := hashToken("test-token")
	hexPattern := regexp.MustCompile(`^[a-f0-9]{64}$`)
	if !hexPattern.MatchString(h) {
		t.Errorf("hashToken should return 64-char hex string, got %q (len=%d)", h, len(h))
	}
	if len(h) != 64 {
		t.Errorf("hashToken should return 64 chars for SHA-256, got %d", len(h))
	}
}

func TestHashToken_DifferentInputsDifferentOutputs(t *testing.T) {
	h1 := hashToken("token1")
	h2 := hashToken("token2")
	if h1 == h2 {
		t.Errorf("hashToken should produce different hashes for different inputs")
	}
}

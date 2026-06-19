package streaming

import (
	"regexp"
	"testing"
	"time"

	"github.com/WebexCommunity/webex-go-sdk/v2/conversation"
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

func TestParseIgnoredSenderEmails(t *testing.T) {
	got := ParseIgnoredSenderEmails(" bot@example.com, , Odin@Example.com ")
	want := []string{"bot@example.com", "Odin@Example.com"}
	if len(got) != len(want) {
		t.Fatalf("ParseIgnoredSenderEmails length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseIgnoredSenderEmails[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestShouldIgnoreActivityBySenderEmail(t *testing.T) {
	m := NewMercuryManagerWithIgnoredSenderEmails(nil, []string{" bot@example.com ", "odin@example.com"})

	activity := &conversation.Activity{
		Actor: &conversation.Actor{EmailAddress: "BOT@example.com"},
	}
	if !m.shouldIgnoreActivity(activity) {
		t.Fatal("shouldIgnoreActivity() = false for ignored sender email, want true")
	}

	activity.Actor.EmailAddress = "person@example.com"
	if m.shouldIgnoreActivity(activity) {
		t.Fatal("shouldIgnoreActivity() = true for non-ignored sender email, want false")
	}
}

func TestShouldIgnoreActivityHandlesMissingActor(t *testing.T) {
	m := NewMercuryManagerWithIgnoredSenderEmails(nil, []string{"bot@example.com"})

	if m.shouldIgnoreActivity(nil) {
		t.Fatal("shouldIgnoreActivity(nil) = true, want false")
	}
	if m.shouldIgnoreActivity(&conversation.Activity{}) {
		t.Fatal("shouldIgnoreActivity(activity without actor) = true, want false")
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

func TestIsDirectRoomRecognizesTagVariants(t *testing.T) {
	cases := []string{
		"ONE_ON_ONE",
		"one-on-one",
		"ONE_ON_ONE_CONVERSATION",
		" direct ",
	}

	for _, tag := range cases {
		activity := &conversation.Activity{
			Target: &conversation.Target{Tags: []string{tag}},
		}
		if !isDirectRoom(activity) {
			t.Errorf("isDirectRoom() = false for tag %q, want true", tag)
		}
	}
}

func TestIsDirectRoomFallsBackToTwoParticipants(t *testing.T) {
	activity := &conversation.Activity{
		Target: &conversation.Target{
			Participants: &conversation.Participants{
				Items: []interface{}{"person-1", "person-2"},
			},
		},
	}

	if !isDirectRoom(activity) {
		t.Fatal("isDirectRoom() = false for two participants, want true")
	}
}

func TestMatchMentionActivityDirectMessage(t *testing.T) {
	activity := &conversation.Activity{
		Actor: &conversation.Actor{EmailAddress: "sender@example.com"},
		Target: &conversation.Target{
			Tags: []string{"one-on-one"},
		},
	}

	matched, matchType := matchMentionActivity(
		activity,
		"",
		"<@personemail:target@example.com",
		"",
		"target@example.com",
		true,
	)
	if !matched || matchType != "direct_message" {
		t.Fatalf("matchMentionActivity() = (%v, %q), want (true, direct_message)", matched, matchType)
	}
}

func TestMatchMentionActivitySkipsOwnDirectMessage(t *testing.T) {
	activity := &conversation.Activity{
		Actor: &conversation.Actor{EmailAddress: "target@example.com"},
		Target: &conversation.Target{
			Tags: []string{"ONE_ON_ONE"},
		},
	}

	matched, matchType := matchMentionActivity(
		activity,
		"",
		"<@personemail:target@example.com",
		"",
		"target@example.com",
		true,
	)
	if matched || matchType != "" {
		t.Fatalf("matchMentionActivity() = (%v, %q), want (false, empty)", matched, matchType)
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

func TestMatchFromPersonActivity(t *testing.T) {
	const (
		sender   = "sender@example.com"
		caller   = "me@example.com"
		callerID = "ME123"
	)

	// activity builds an activity from a sender, room tags, room IDs, and participant count.
	activity := func(senderEmail string, tags []string, roomID, globalID string, participants int) *conversation.Activity {
		a := &conversation.Activity{
			Actor: &conversation.Actor{EmailAddress: senderEmail},
		}
		if tags != nil || roomID != "" || globalID != "" || participants > 0 {
			a.Target = &conversation.Target{ID: roomID, GlobalID: globalID, Tags: tags}
			if participants > 0 {
				items := make([]interface{}, participants)
				a.Target.Participants = &conversation.Participants{Items: items}
			}
		}
		return a
	}
	roomSet := func(ids ...string) map[string]bool {
		s := make(map[string]bool, len(ids))
		for _, id := range ids {
			s[id] = true
		}
		return s
	}

	cases := []struct {
		name         string
		activity     *conversation.Activity
		content      string
		roomSet      map[string]bool
		mentionsOnly bool
		wantMatched  bool
		wantType     string
	}{
		{
			name:        "wrong sender rejected",
			activity:    activity("other@example.com", []string{"ONE_ON_ONE"}, "", "", 0),
			wantMatched: false,
		},
		{
			name:        "empty rooms direct message",
			activity:    activity(sender, []string{"ONE_ON_ONE"}, "", "", 0),
			wantMatched: true,
			wantType:    "from_person_direct",
		},
		{
			name:        "empty rooms group rejected",
			activity:    activity(sender, []string{"GROUP"}, "room1", "", 0),
			wantMatched: false,
		},
		{
			name:        "listed room by ID",
			activity:    activity(sender, []string{"GROUP"}, "room1", "", 0),
			roomSet:     roomSet("room1"),
			wantMatched: true,
			wantType:    "from_person",
		},
		{
			name:        "listed room by GlobalID",
			activity:    activity(sender, []string{"GROUP"}, "localid", "globalid1", 0),
			roomSet:     roomSet("globalid1"),
			wantMatched: true,
			wantType:    "from_person",
		},
		{
			name:        "room not in set rejected",
			activity:    activity(sender, []string{"GROUP"}, "room2", "", 0),
			roomSet:     roomSet("room1"),
			wantMatched: false,
		},
		{
			name:         "empty rooms mentionsOnly mentions caller email",
			activity:     activity(sender, []string{"GROUP"}, "room9", "", 0),
			content:      "hey <@personEmail:me@example.com|Me> look",
			mentionsOnly: true,
			wantMatched:  true,
			wantType:     "from_person_mention",
		},
		{
			name:         "mentionsOnly caller email without alias",
			activity:     activity(sender, []string{"GROUP"}, "room9", "", 0),
			content:      "hey <@personEmail:me@example.com> look",
			mentionsOnly: true,
			wantMatched:  true,
			wantType:     "from_person_mention",
		},
		{
			name:         "mentionsOnly prefix-collision email rejected",
			activity:     activity(sender, []string{"GROUP"}, "room9", "", 0),
			content:      "hey <@personEmail:me@example.com.evil.com|Imposter>",
			mentionsOnly: true,
			wantMatched:  false,
		},
		{
			name:         "empty rooms mentionsOnly mentions caller by ID",
			activity:     activity(sender, []string{"GROUP"}, "room9", "", 0),
			content:      "ping <@personId:ME123|Me>",
			mentionsOnly: true,
			wantMatched:  true,
			wantType:     "from_person_mention",
		},
		{
			name:         "empty rooms mentionsOnly mention all",
			activity:     activity(sender, []string{"GROUP"}, "room9", "", 0),
			content:      "announcement <@all>",
			mentionsOnly: true,
			wantMatched:  true,
			wantType:     "from_person_mention_all",
		},
		{
			name:         "mentionsOnly third-party mention rejected",
			activity:     activity(sender, []string{"GROUP"}, "room9", "", 0),
			content:      "hey <@personEmail:bob@example.com|Bob>",
			mentionsOnly: true,
			wantMatched:  false,
		},
		{
			name:         "listed room mentionsOnly mentions caller",
			activity:     activity(sender, []string{"GROUP"}, "room1", "", 0),
			content:      "<@personEmail:me@example.com|Me> hi",
			roomSet:      roomSet("room1"),
			mentionsOnly: true,
			wantMatched:  true,
			wantType:     "from_person_mention",
		},
		{
			name:         "listed room mentionsOnly no mention rejected",
			activity:     activity(sender, []string{"GROUP"}, "room1", "", 0),
			content:      "just a normal message",
			roomSet:      roomSet("room1"),
			mentionsOnly: true,
			wantMatched:  false,
		},
		{
			name:        "case-insensitive sender and mention",
			activity:    activity("SENDER@EXAMPLE.COM", []string{"GROUP"}, "room1", "", 0),
			content:     "HEY <@PERSONEMAIL:ME@EXAMPLE.COM|Me>",
			roomSet:     roomSet("room1"),
			wantMatched: true,
			wantType:    "from_person",
		},
		{
			name:         "mentionsOnly mixed-case mention markup matches",
			activity:     activity(sender, []string{"GROUP"}, "room1", "", 0),
			content:      "HEY <@PersonEmail:Me@Example.com> there",
			roomSet:      roomSet("room1"),
			mentionsOnly: true,
			wantMatched:  true,
			wantType:     "from_person_mention",
		},
		{
			name:        "nil actor rejected",
			activity:    &conversation.Activity{Target: &conversation.Target{Tags: []string{"ONE_ON_ONE"}}},
			wantMatched: false,
		},
		{
			name:        "nil target with non-empty roomSet rejected",
			activity:    &conversation.Activity{Actor: &conversation.Actor{EmailAddress: sender}},
			roomSet:     roomSet("room1"),
			wantMatched: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			callerEmail, callerPersonID := "", ""
			if tc.mentionsOnly {
				callerEmail, callerPersonID = caller, callerID
			}
			matched, matchType := matchFromPersonActivity(
				tc.activity, tc.content, sender, tc.roomSet, tc.mentionsOnly, callerEmail, callerPersonID,
			)
			if matched != tc.wantMatched {
				t.Fatalf("matched = %v, want %v (matchType=%q)", matched, tc.wantMatched, matchType)
			}
			if matched && matchType != tc.wantType {
				t.Fatalf("matchType = %q, want %q", matchType, tc.wantType)
			}
		})
	}
}

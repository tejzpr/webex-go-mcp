package streaming

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	webex "github.com/WebexCommunity/webex-go-sdk/v2"
	"github.com/WebexCommunity/webex-go-sdk/v2/conversation"
	"github.com/WebexCommunity/webex-go-sdk/v2/people"

	"github.com/mark3labs/mcp-go/server"
)

// eventHandler pairs an event type name with its handler function for cleanup.
type eventHandler struct {
	eventType string
	handler   func(*conversation.Activity)
}

// Mention-token prefixes used in decrypted Webex activity content.
// Full syntax: <@personEmail:user@example.com|DisplayName>, <@personId:UUID|Name>, <@all>.
// Kept as lowercase constants because all matching is done on lowercased content.
const (
	mentionEmailPrefix = "<@personemail:"
	mentionIDPrefix    = "<@personid:"
	mentionAllToken    = "<@all>"
)

// Subscription represents an active Mercury event subscription.
type Subscription struct {
	ID           string
	RoomID       string
	Rooms        []string // scoped room list for from-person subscriptions (empty => direct/all)
	Email        string   // target email for mentions; sender email for from-person
	PersonID     string   // resolved person ID: mention target, or caller (from-person)
	MentionsOnly bool     // from-person: require a mention of the caller or @all
	TokenHash    string
	SessionID    string
	CreatedAt    time.Time
	cancel       context.CancelFunc
	handlers     []eventHandler
}

// MercuryManager manages per-user Mercury connections and multiplexes
// conversation events to MCP client sessions as notifications.
type MercuryManager struct {
	mu                  sync.RWMutex
	subscriptions       map[string]*Subscription   // subscriptionId → sub
	userConns           map[string]*userConnection // tokenHash → connection
	mcpServer           *server.MCPServer
	ignoredSenderEmails map[string]struct{}
}

// userConnection holds a per-user Mercury/Conversation connection.
type userConnection struct {
	mu         sync.Mutex
	client     *webex.WebexClient
	convClient *conversation.Client
	connected  bool
	refCount   int // number of active subscriptions using this connection
	tokenHash  string
}

// NewMercuryManager creates a new MercuryManager.
func NewMercuryManager(mcpServer *server.MCPServer) *MercuryManager {
	return NewMercuryManagerWithIgnoredSenderEmails(mcpServer, nil)
}

// NewMercuryManagerWithIgnoredSenderEmails creates a MercuryManager that drops
// incoming stream activities authored by any configured sender email.
func NewMercuryManagerWithIgnoredSenderEmails(mcpServer *server.MCPServer, emails []string) *MercuryManager {
	ignored := normalizeEmailSet(emails)
	return &MercuryManager{
		subscriptions:       make(map[string]*Subscription),
		userConns:           make(map[string]*userConnection),
		mcpServer:           mcpServer,
		ignoredSenderEmails: ignored,
	}
}

func normalizeEmailSet(emails []string) map[string]struct{} {
	if len(emails) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		normalized := strings.ToLower(strings.TrimSpace(email))
		if normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

// ParseIgnoredSenderEmails parses a comma-separated env/config value.
func ParseIgnoredSenderEmails(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	emails := make([]string, 0, len(parts))
	for _, part := range parts {
		email := strings.TrimSpace(part)
		if email != "" {
			emails = append(emails, email)
		}
	}
	return emails
}

func (m *MercuryManager) shouldIgnoreActivity(activity *conversation.Activity) bool {
	if m == nil || len(m.ignoredSenderEmails) == 0 || activity == nil || activity.Actor == nil {
		return false
	}
	email := strings.ToLower(strings.TrimSpace(activity.Actor.EmailAddress))
	if email == "" {
		return false
	}
	_, ignored := m.ignoredSenderEmails[email]
	return ignored
}

// Subscribe creates a new subscription for room messages.
// It sets up a Mercury connection (if not already active for this user),
// registers event handlers, and streams events as MCP notifications.
func (m *MercuryManager) Subscribe(
	ctx context.Context,
	client *webex.WebexClient,
	accessToken string,
	roomID string,
	eventTypes []string,
) (*Subscription, error) {
	if len(eventTypes) == 0 {
		eventTypes = []string{"post", "share"}
	}

	tokHash := hashToken(accessToken)

	// Get or create the user's Mercury connection
	uc, err := m.getOrCreateConnection(client, tokHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create Mercury connection: %w", err)
	}

	// Generate subscription ID
	subID := fmt.Sprintf("sub_%x", sha256.Sum256([]byte(fmt.Sprintf("%s_%s_%d", tokHash, roomID, time.Now().UnixNano()))))[:20]

	// Get the session ID from context for targeted notifications
	sessionID := ""
	if session := server.ClientSessionFromContext(ctx); session != nil {
		sessionID = session.SessionID()
	}

	subCtx, cancel := context.WithCancel(context.Background())

	sub := &Subscription{
		ID:        subID,
		RoomID:    roomID,
		TokenHash: tokHash,
		SessionID: sessionID,
		CreatedAt: time.Now(),
		cancel:    cancel,
	}

	m.mu.Lock()
	m.subscriptions[subID] = sub
	m.mu.Unlock()

	// Register event handlers for the requested event types, storing refs for cleanup
	for _, eventType := range eventTypes {
		et := eventType // capture
		handler := func(activity *conversation.Activity) {
			select {
			case <-subCtx.Done():
				return
			default:
			}

			if roomID != "" && activity.Target != nil && activity.Target.ID != roomID {
				if activity.Target.GlobalID != roomID {
					return
				}
			}
			if m.shouldIgnoreActivity(activity) {
				log.Printf("[Mercury] Dropping ignored sender activity: subscription=%s event=%s activity=%s sender=%s",
					subID, et, activityID(activity), activity.Actor.EmailAddress)
				return
			}

			payload := m.buildEventPayload(sub, et, activity)
			m.sendNotification(sessionID, payload)
		}
		uc.convClient.On(et, handler)
		sub.handlers = append(sub.handlers, eventHandler{eventType: et, handler: handler})
	}

	// Ensure Mercury is connected
	uc.mu.Lock()
	if !uc.connected {
		log.Printf("[Mercury] Connecting Mercury for user (hash=%s...)", tokHash[:8])
		if err := uc.convClient.Connect(); err != nil {
			uc.mu.Unlock()
			m.Unsubscribe(subID)
			return nil, fmt.Errorf("failed to connect Mercury: %w", err)
		}
		uc.connected = true
		log.Printf("[Mercury] Connected successfully for user (hash=%s...)", tokHash[:8])
	}
	uc.mu.Unlock()

	log.Printf("[Mercury] Subscription %s created: room=%s events=%v session=%s", subID, roomID, eventTypes, sessionID)
	return sub, nil
}

// SubscribeMentions creates a subscription that filters for messages mentioning
// a specific email address or sent as direct messages (1:1) to that user.
// It listens to all Mercury post/share events and checks:
//   - The decrypted content for Webex mention syntax: <@personEmail:target@example.com|...>
//   - Whether the room is a 1:1 (direct) conversation via Target.Tags containing "ONE_ON_ONE"
//   - Whether the message was sent directly to the user (Actor.EmailAddress != targetEmail in a 1:1)
func (m *MercuryManager) SubscribeMentions(
	ctx context.Context,
	client *webex.WebexClient,
	accessToken string,
	targetEmail string,
	includeDirect bool,
) (*Subscription, error) {
	if targetEmail == "" {
		return nil, fmt.Errorf("target email is required")
	}
	targetEmail = strings.ToLower(strings.TrimSpace(targetEmail))

	tokHash := hashToken(accessToken)

	// Resolve email to personId for additional matching
	personID := ""
	page, err := client.People().List(&people.ListOptions{Email: targetEmail})
	if err == nil && len(page.Items) > 0 {
		personID = page.Items[0].ID
		log.Printf("[Mercury] Resolved email %s to personId %s", targetEmail, personID)
	} else {
		log.Printf("[Mercury] Could not resolve email %s to personId (will match by email pattern only): %v", targetEmail, err)
	}

	// Get or create the user's Mercury connection
	uc, err := m.getOrCreateConnection(client, tokHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create Mercury connection: %w", err)
	}

	// Generate subscription ID
	subID := fmt.Sprintf("msub_%x", sha256.Sum256([]byte(fmt.Sprintf("%s_%s_%d", tokHash, targetEmail, time.Now().UnixNano()))))[:21]

	// Get the session ID from context for targeted notifications
	sessionID := ""
	if session := server.ClientSessionFromContext(ctx); session != nil {
		sessionID = session.SessionID()
	}

	subCtx, cancel := context.WithCancel(context.Background())

	sub := &Subscription{
		ID:        subID,
		Email:     targetEmail,
		PersonID:  personID,
		TokenHash: tokHash,
		SessionID: sessionID,
		CreatedAt: time.Now(),
		cancel:    cancel,
	}

	m.mu.Lock()
	m.subscriptions[subID] = sub
	m.mu.Unlock()

	// Build the mention pattern to search for in decrypted content
	// Webex mention syntax: <@personEmail:user@example.com|DisplayName>
	mentionPattern := mentionEmailPrefix + targetEmail

	// Register handlers for post and share events (no room filter — listen to everything)
	for _, eventType := range []string{"post", "share"} {
		et := eventType
		handler := func(activity *conversation.Activity) {
			select {
			case <-subCtx.Done():
				return
			default:
			}

			if m.shouldIgnoreActivity(activity) {
				log.Printf("[Mercury] Dropping ignored sender mention activity: subscription=%s event=%s activity=%s sender=%s",
					subID, et, activityID(activity), activity.Actor.EmailAddress)
				return
			}

			content := m.getActivityContent(uc, activity)
			matched, matchType := matchMentionActivity(activity, content, mentionPattern, personID, targetEmail, includeDirect)

			if !matched {
				return
			}

			log.Printf("[Mercury] Mention subscription %s matched event: event=%s matchType=%s activity=%s session=%s",
				subID, et, matchType, activityID(activity), sessionLabel(sessionID))
			payload := m.buildMentionEventPayload(sub, et, "targetEmail", activity, content)
			payload["matchType"] = matchType
			m.sendNotification(sessionID, payload)
		}
		uc.convClient.On(et, handler)
		sub.handlers = append(sub.handlers, eventHandler{eventType: et, handler: handler})
	}

	// Ensure Mercury is connected
	uc.mu.Lock()
	if !uc.connected {
		log.Printf("[Mercury] Connecting Mercury for user (hash=%s...)", tokHash[:8])
		if err := uc.convClient.Connect(); err != nil {
			uc.mu.Unlock()
			m.Unsubscribe(subID)
			return nil, fmt.Errorf("failed to connect Mercury: %w", err)
		}
		uc.connected = true
		log.Printf("[Mercury] Connected successfully for user (hash=%s...)", tokHash[:8])
	}
	uc.mu.Unlock()

	log.Printf("[Mercury] Mention subscription %s created: email=%s personId=%s includeDirect=%v session=%s",
		subID, targetEmail, personID, includeDirect, sessionID)
	return sub, nil
}

func matchMentionActivity(activity *conversation.Activity, content, mentionPattern, personID, targetEmail string, includeDirect bool) (bool, string) {
	lowerContent := strings.ToLower(content)
	if lowerContent != "" && strings.Contains(lowerContent, mentionPattern) {
		return true, "mention"
	}

	if personID != "" {
		personIDPattern := mentionIDPrefix + strings.ToLower(personID)
		if lowerContent != "" && strings.Contains(lowerContent, personIDPattern) {
			return true, "mention"
		}
	}

	if lowerContent != "" && strings.Contains(lowerContent, mentionAllToken) {
		return true, "mention_all"
	}

	if includeDirect && isDirectRoom(activity) && activity.Actor != nil &&
		!strings.EqualFold(strings.TrimSpace(activity.Actor.EmailAddress), targetEmail) {
		return true, "direct_message"
	}

	return false, ""
}

// matchFromPersonActivity reports whether an activity is a message sent BY personEmail
// that satisfies the room scope and optional mention filter.
//
//   - Sender gate (always): activity.Actor.EmailAddress == personEmail.
//   - Room gate: empty roomSet + !mentionsOnly => 1:1 (direct) rooms only;
//     empty roomSet + mentionsOnly => any room (the mention narrows);
//     non-empty roomSet => Target.ID or Target.GlobalID must be in the set.
//   - Mention gate (only when mentionsOnly): the message must mention the caller
//     (by email or person ID) or mention everyone (@all). A third-party mention
//     does not qualify.
//
// personEmail must be lowercased and trimmed by the caller. callerEmail/callerPersonID
// are only consulted when mentionsOnly is true.
func matchFromPersonActivity(
	activity *conversation.Activity,
	content, personEmail string,
	roomSet map[string]bool,
	mentionsOnly bool,
	callerEmail, callerPersonID string,
) (bool, string) {
	// 1. Sender gate — personEmail is always the author filter.
	if activity == nil || activity.Actor == nil {
		return false, ""
	}
	if !strings.EqualFold(strings.TrimSpace(activity.Actor.EmailAddress), personEmail) {
		return false, ""
	}

	// 2. Room scope gate.
	if len(roomSet) == 0 {
		if !mentionsOnly && !isDirectRoom(activity) {
			return false, "" // empty rooms, no mention filter => direct (1:1) only
		}
		// empty rooms + mentionsOnly => listen across all rooms; mention narrows below.
	} else {
		if activity.Target == nil {
			return false, ""
		}
		if !roomSet[activity.Target.ID] && !roomSet[activity.Target.GlobalID] {
			return false, ""
		}
	}

	// 3. Mention gate.
	if mentionsOnly {
		lower := strings.ToLower(content)
		if lower == "" {
			return false, ""
		}
		mentionsCaller := (callerEmail != "" && containsMention(lower, mentionEmailPrefix, callerEmail)) ||
			(callerPersonID != "" && containsMention(lower, mentionIDPrefix, strings.ToLower(callerPersonID)))
		switch {
		case mentionsCaller:
			return true, "from_person_mention"
		case strings.Contains(lower, mentionAllToken):
			return true, "from_person_mention_all"
		default:
			return false, "" // third-party mention (or none) does not qualify
		}
	}

	// 4. No mention filter — sender and scope already satisfied.
	if isDirectRoom(activity) {
		return true, "from_person_direct"
	}
	return true, "from_person"
}

// containsMention reports whether lowerContent mentions a target identifier using
// the given Webex mention prefix. The Webex syntax is <@personEmail:user@example.com>
// or <@personId:ID>, with an optional "|DisplayName" alias before the closing ">".
// We require the identifier to be terminated by ">" or "|" so that a prefix collision
// (e.g. a longer email or ID that begins with the target) does not produce a false match.
// prefix and identifier must already be lowercased by the caller.
func containsMention(lowerContent, prefix, identifier string) bool {
	needle := prefix + identifier
	from := 0
	for {
		idx := strings.Index(lowerContent[from:], needle)
		if idx < 0 {
			return false
		}
		end := from + idx + len(needle)
		if end >= len(lowerContent) {
			// Truncated/malformed token at the very end — treat as no clean match.
			return false
		}
		if c := lowerContent[end]; c == '>' || c == '|' {
			return true
		}
		from = end // keep scanning; this was a prefix of a longer identifier
	}
}

// isDirectRoom checks if the activity's target room is a 1:1 (direct) conversation.
// Webex commonly marks 1:1 rooms with a ONE_ON_ONE-style tag, but the exact
// spelling varies between Mercury payloads. Some payloads only carry the two
// room participants, so use that as a fallback.
func isDirectRoom(activity *conversation.Activity) bool {
	if activity == nil || activity.Target == nil {
		return false
	}
	for _, tag := range activity.Target.Tags {
		normalized := normalizeWebexTag(tag)
		if normalized == "ONEONONE" || normalized == "DIRECT" || strings.Contains(normalized, "ONE_ON_ONE") {
			return true
		}
	}
	if activity.Target.Participants != nil && len(activity.Target.Participants.Items) == 2 {
		return true
	}
	return false
}

func normalizeWebexTag(tag string) string {
	normalized := strings.ToUpper(strings.TrimSpace(tag))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	return normalized
}

// getActivityContent extracts the decrypted message content from an activity.
func (m *MercuryManager) getActivityContent(uc *userConnection, activity *conversation.Activity) string {
	// Try the already-decrypted content first
	if activity.Content != "" {
		return activity.Content
	}

	// Try to get content via the conversation client (handles decryption)
	content, err := uc.convClient.GetMessageContent(activity)
	if err == nil && content != "" {
		return content
	}

	// Fall back to DecryptedObject
	if activity.DecryptedObject != nil {
		if activity.DecryptedObject.DisplayName != "" {
			return activity.DecryptedObject.DisplayName
		}
		if activity.DecryptedObject.Content != "" {
			return activity.DecryptedObject.Content
		}
	}

	return ""
}

// buildMentionEventPayload creates a notification payload for a mention/DM event.
// buildMentionEventPayload creates a notification payload for a mention/DM/from-person event.
// emailKey controls the key under which sub.Email is reported ("targetEmail" for mention
// and DM subscriptions, "personEmail" for from-person subscriptions). The caller may override
// the "matchType" afterwards; the value computed here is a best-effort default.
func (m *MercuryManager) buildMentionEventPayload(sub *Subscription, eventType, emailKey string, activity *conversation.Activity, content string) map[string]interface{} {
	matchType := "mention"
	if isDirectRoom(activity) {
		matchType = "direct_message"
	} else if content != "" && strings.Contains(strings.ToLower(content), mentionAllToken) {
		matchType = "mention_all"
	}

	payload := map[string]interface{}{
		"subscriptionId": sub.ID,
		"eventType":      eventType,
		"matchType":      matchType,
		emailKey:         sub.Email,
		"timestamp":      activity.Published,
	}

	if activity.Actor != nil {
		payload["sender"] = map[string]interface{}{
			"displayName":  activity.Actor.DisplayName,
			"emailAddress": activity.Actor.EmailAddress,
			"id":           activity.Actor.ID,
		}
	}

	if activity.Target != nil {
		roomInfo := map[string]interface{}{
			"id":       activity.Target.ID,
			"globalId": activity.Target.GlobalID,
		}
		if isDirectRoom(activity) {
			roomInfo["type"] = "direct"
		} else {
			roomInfo["type"] = "group"
		}
		payload["room"] = roomInfo
	}

	if content != "" {
		payload["content"] = content
	}

	return payload
}

// SubscribeDirectMessages creates a subscription that only delivers messages from
// 1:1 (direct) conversations. Unlike SubscribeMentions, this ignores @mentions and
// @all — it purely streams DMs so an AI can have a direct conversation with the user.
func (m *MercuryManager) SubscribeDirectMessages(
	ctx context.Context,
	client *webex.WebexClient,
	accessToken string,
	targetEmail string,
) (*Subscription, error) {
	if targetEmail == "" {
		return nil, fmt.Errorf("target email is required")
	}
	targetEmail = strings.ToLower(strings.TrimSpace(targetEmail))

	tokHash := hashToken(accessToken)

	// Get or create the user's Mercury connection
	uc, err := m.getOrCreateConnection(client, tokHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create Mercury connection: %w", err)
	}

	// Generate subscription ID
	subID := fmt.Sprintf("dmsub_%x", sha256.Sum256([]byte(fmt.Sprintf("%s_%s_%d", tokHash, targetEmail, time.Now().UnixNano()))))[:22]

	// Get the session ID from context for targeted notifications
	sessionID := ""
	if session := server.ClientSessionFromContext(ctx); session != nil {
		sessionID = session.SessionID()
	}

	subCtx, cancel := context.WithCancel(context.Background())

	sub := &Subscription{
		ID:        subID,
		Email:     targetEmail,
		TokenHash: tokHash,
		SessionID: sessionID,
		CreatedAt: time.Now(),
		cancel:    cancel,
	}

	m.mu.Lock()
	m.subscriptions[subID] = sub
	m.mu.Unlock()

	// Register handlers for post and share events — only deliver 1:1 messages
	for _, eventType := range []string{"post", "share"} {
		et := eventType
		handler := func(activity *conversation.Activity) {
			select {
			case <-subCtx.Done():
				return
			default:
			}

			// Only deliver messages from 1:1 (direct) rooms
			if !isDirectRoom(activity) {
				return
			}

			// Skip messages sent by the target user themselves
			if activity.Actor != nil && strings.EqualFold(strings.TrimSpace(activity.Actor.EmailAddress), targetEmail) {
				return
			}

			if m.shouldIgnoreActivity(activity) {
				log.Printf("[Mercury] Dropping ignored sender DM activity: subscription=%s event=%s activity=%s sender=%s",
					subID, et, activityID(activity), activity.Actor.EmailAddress)
				return
			}

			content := m.getActivityContent(uc, activity)

			log.Printf("[Mercury] DM subscription %s matched event: event=%s activity=%s session=%s",
				subID, et, activityID(activity), sessionLabel(sessionID))

			payload := m.buildMentionEventPayload(sub, et, "targetEmail", activity, content)
			payload["matchType"] = "direct_message"
			m.sendNotification(sessionID, payload)
		}
		uc.convClient.On(et, handler)
		sub.handlers = append(sub.handlers, eventHandler{eventType: et, handler: handler})
	}

	// Ensure Mercury is connected
	uc.mu.Lock()
	if !uc.connected {
		log.Printf("[Mercury] Connecting Mercury for user (hash=%s...)", tokHash[:8])
		if err := uc.convClient.Connect(); err != nil {
			uc.mu.Unlock()
			m.Unsubscribe(subID)
			return nil, fmt.Errorf("failed to connect Mercury: %w", err)
		}
		uc.connected = true
		log.Printf("[Mercury] Connected successfully for user (hash=%s...)", tokHash[:8])
	}
	uc.mu.Unlock()

	log.Printf("[Mercury] DM subscription %s created: email=%s session=%s", subID, targetEmail, sessionID)
	return sub, nil
}

// SubscribeMessagesFromPerson creates a subscription that streams messages sent BY
// personEmail, scoped by rooms and an optional mentions filter (see matchFromPersonActivity).
// personEmail is always the sender filter. When mentionsOnly is true, only messages that
// mention the authenticated caller (or @all) are delivered, and the caller is resolved via
// the People API.
func (m *MercuryManager) SubscribeMessagesFromPerson(
	ctx context.Context,
	client *webex.WebexClient,
	accessToken string,
	personEmail string,
	rooms []string,
	mentionsOnly bool,
) (*Subscription, error) {
	personEmail = strings.ToLower(strings.TrimSpace(personEmail))
	if personEmail == "" {
		return nil, fmt.Errorf("personEmail is required")
	}

	tokHash := hashToken(accessToken)

	// Resolve the caller only when matching mentions of "me".
	var callerEmail, callerPersonID string
	if mentionsOnly {
		var err error
		callerEmail, callerPersonID, err = resolveCaller(client)
		if err != nil {
			return nil, err
		}
		log.Printf("[Mercury] Resolved caller %s (personId %s) for from-person mention filter", callerEmail, callerPersonID)
	}

	uc, err := m.getOrCreateConnection(client, tokHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create Mercury connection: %w", err)
	}

	roomSet := buildRoomSet(rooms)
	normalizedRooms := make([]string, 0, len(roomSet))
	for r := range roomSet {
		normalizedRooms = append(normalizedRooms, r)
	}

	subID := fmt.Sprintf("fpsub_%x", sha256.Sum256([]byte(fmt.Sprintf("%s_%s_%d", tokHash, personEmail, time.Now().UnixNano()))))[:22]

	sessionID := ""
	if session := server.ClientSessionFromContext(ctx); session != nil {
		sessionID = session.SessionID()
	}

	subCtx, cancel := context.WithCancel(context.Background())

	sub := &Subscription{
		ID:           subID,
		Email:        personEmail,
		PersonID:     callerPersonID,
		Rooms:        normalizedRooms,
		MentionsOnly: mentionsOnly,
		TokenHash:    tokHash,
		SessionID:    sessionID,
		CreatedAt:    time.Now(),
		cancel:       cancel,
	}

	m.mu.Lock()
	m.subscriptions[subID] = sub
	m.mu.Unlock()

	for _, eventType := range []string{"post", "share"} {
		et := eventType
		handler := func(activity *conversation.Activity) {
			select {
			case <-subCtx.Done():
				return
			default:
			}

			if m.shouldIgnoreActivity(activity) {
				log.Printf("[Mercury] Dropping ignored sender from-person activity: subscription=%s event=%s activity=%s sender=%s",
					subID, et, activityID(activity), activity.Actor.EmailAddress)
				return
			}

			content := m.getActivityContent(uc, activity)
			matched, matchType := matchFromPersonActivity(activity, content, personEmail, roomSet, mentionsOnly, callerEmail, callerPersonID)
			if !matched {
				return
			}

			log.Printf("[Mercury] From-person subscription %s matched event: event=%s matchType=%s activity=%s session=%s",
				subID, et, matchType, activityID(activity), sessionLabel(sessionID))
			payload := m.buildMentionEventPayload(sub, et, "personEmail", activity, content)
			payload["matchType"] = matchType
			m.sendNotification(sessionID, payload)
		}
		uc.convClient.On(et, handler)
		sub.handlers = append(sub.handlers, eventHandler{eventType: et, handler: handler})
	}

	// Ensure Mercury is connected
	uc.mu.Lock()
	if !uc.connected {
		log.Printf("[Mercury] Connecting Mercury for user (hash=%s...)", tokHash[:8])
		if err := uc.convClient.Connect(); err != nil {
			uc.mu.Unlock()
			m.Unsubscribe(subID)
			return nil, fmt.Errorf("failed to connect Mercury: %w", err)
		}
		uc.connected = true
		log.Printf("[Mercury] Connected successfully for user (hash=%s...)", tokHash[:8])
	}
	uc.mu.Unlock()

	log.Printf("[Mercury] From-person subscription %s created: personEmail=%s rooms=%v mentionsOnly=%v session=%s",
		subID, personEmail, normalizedRooms, mentionsOnly, sessionID)
	return sub, nil
}

// Unsubscribe cancels a subscription and cleans up resources.
func (m *MercuryManager) Unsubscribe(subscriptionID string) error {
	m.mu.Lock()
	sub, ok := m.subscriptions[subscriptionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("subscription %s not found", subscriptionID)
	}
	delete(m.subscriptions, subscriptionID)
	m.mu.Unlock()

	// Cancel the subscription context
	sub.cancel()

	// Remove registered event handlers
	m.mu.RLock()
	uc, ok := m.userConns[sub.TokenHash]
	m.mu.RUnlock()

	if ok {
		for _, h := range sub.handlers {
			uc.convClient.Off(h.eventType, h.handler)
		}
		sub.handlers = nil

		m.releaseConnection(sub.TokenHash)
	}

	log.Printf("[Mercury] Subscription %s cancelled", subscriptionID)
	return nil
}

// releaseConnection decrements the refcount on a user's Mercury connection and,
// when no subscriptions or waiters remain, disconnects and removes it. It is the
// counterpart to the refCount++ in getOrCreateConnection; every getOrCreateConnection
// must be paired with exactly one releaseConnection.
func (m *MercuryManager) releaseConnection(tokHash string) {
	m.mu.RLock()
	uc, ok := m.userConns[tokHash]
	m.mu.RUnlock()
	if !ok {
		return
	}

	uc.mu.Lock()
	uc.refCount--
	if uc.refCount <= 0 {
		log.Printf("[Mercury] No more subscriptions for user (hash=%s...), disconnecting", tokHash[:8])
		uc.convClient.Disconnect()
		uc.connected = false
		uc.mu.Unlock()

		m.mu.Lock()
		delete(m.userConns, tokHash)
		m.mu.Unlock()
	} else {
		uc.mu.Unlock()
	}
}

// UnsubscribeBySession cancels all subscriptions for a given MCP session.
func (m *MercuryManager) UnsubscribeBySession(sessionID string) {
	m.mu.RLock()
	var toCancel []string
	for id, sub := range m.subscriptions {
		if sub.SessionID == sessionID {
			toCancel = append(toCancel, id)
		}
	}
	m.mu.RUnlock()

	for _, id := range toCancel {
		m.Unsubscribe(id)
	}
}

// buildRoomSet normalizes a list of room IDs into a lookup set, dropping blanks.
// Returns nil when no usable room IDs remain (the "empty rooms" branch).
func buildRoomSet(rooms []string) map[string]bool {
	set := make(map[string]bool, len(rooms))
	for _, r := range rooms {
		r = strings.TrimSpace(r)
		if r != "" {
			set[r] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

// resolveCaller fetches the authenticated user (the token owner) so mention
// matching can target "me". Returns lowercased email and person ID.
func resolveCaller(client *webex.WebexClient) (email, personID string, err error) {
	me, err := client.People().GetMe()
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve authenticated user: %w", err)
	}
	if me == nil || len(me.Emails) == 0 {
		return "", "", fmt.Errorf("authenticated user has no email address")
	}
	return strings.ToLower(strings.TrimSpace(me.Emails[0])), me.ID, nil
}

// WaitForMessage blocks until a message arrives in the specified room or timeout.
func (m *MercuryManager) WaitForMessage(
	ctx context.Context,
	client *webex.WebexClient,
	accessToken string,
	roomID string,
	timeout time.Duration,
) (map[string]interface{}, error) {
	tokHash := hashToken(accessToken)

	uc, err := m.getOrCreateConnection(client, tokHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create Mercury connection: %w", err)
	}
	defer m.releaseConnection(tokHash)

	resultCh := make(chan map[string]interface{}, 1)
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Register a one-shot handler
	handler := func(activity *conversation.Activity) {
		if roomID != "" && activity.Target != nil {
			if activity.Target.ID != roomID && activity.Target.GlobalID != roomID {
				return
			}
		}
		if m.shouldIgnoreActivity(activity) {
			log.Printf("[Mercury] Dropping ignored sender wait activity: activity=%s sender=%s",
				activityID(activity), activity.Actor.EmailAddress)
			return
		}

		content, _ := uc.convClient.GetMessageContent(activity)
		payload := map[string]interface{}{
			"type":      activity.Verb,
			"content":   content,
			"roomId":    "",
			"sender":    "",
			"timestamp": activity.Published,
		}
		if activity.Target != nil {
			payload["roomId"] = activity.Target.ID
		}
		if activity.Actor != nil {
			payload["sender"] = activity.Actor.DisplayName
			payload["senderEmail"] = activity.Actor.EmailAddress
		}

		select {
		case resultCh <- payload:
		default:
		}
	}

	uc.convClient.On("post", handler)
	uc.convClient.On("share", handler)
	defer uc.convClient.Off("post", handler)
	defer uc.convClient.Off("share", handler)

	// Ensure connected
	uc.mu.Lock()
	if !uc.connected {
		if err := uc.convClient.Connect(); err != nil {
			uc.mu.Unlock()
			return nil, fmt.Errorf("failed to connect Mercury: %w", err)
		}
		uc.connected = true
	}
	uc.mu.Unlock()

	select {
	case result := <-resultCh:
		return result, nil
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("timeout waiting for message after %v", timeout)
	}
}

// WaitForMessageFromPerson blocks until a message sent BY personEmail arrives that
// satisfies the room scope and optional mention filter (see matchFromPersonActivity),
// or until timeout. personEmail is always the sender filter.
func (m *MercuryManager) WaitForMessageFromPerson(
	ctx context.Context,
	client *webex.WebexClient,
	accessToken string,
	personEmail string,
	rooms []string,
	mentionsOnly bool,
	timeout time.Duration,
) (map[string]interface{}, error) {
	personEmail = strings.ToLower(strings.TrimSpace(personEmail))
	if personEmail == "" {
		return nil, fmt.Errorf("personEmail is required")
	}

	tokHash := hashToken(accessToken)
	uc, err := m.getOrCreateConnection(client, tokHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create Mercury connection: %w", err)
	}
	defer m.releaseConnection(tokHash)

	roomSet := buildRoomSet(rooms)

	// Resolve the caller only when we need to match mentions of "me".
	var callerEmail, callerPersonID string
	if mentionsOnly {
		callerEmail, callerPersonID, err = resolveCaller(client)
		if err != nil {
			return nil, err
		}
	}

	resultCh := make(chan map[string]interface{}, 1)
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	handler := func(activity *conversation.Activity) {
		if m.shouldIgnoreActivity(activity) {
			return
		}

		content := m.getActivityContent(uc, activity)
		matched, matchType := matchFromPersonActivity(activity, content, personEmail, roomSet, mentionsOnly, callerEmail, callerPersonID)
		if !matched {
			return
		}

		payload := map[string]interface{}{
			"type":        activity.Verb,
			"matchType":   matchType,
			"content":     content,
			"roomId":      "",
			"sender":      "",
			"personEmail": personEmail,
			"timestamp":   activity.Published,
		}
		if activity.Target != nil {
			payload["roomId"] = activity.Target.ID
			payload["roomGlobalId"] = activity.Target.GlobalID
		}
		if activity.Actor != nil {
			payload["sender"] = activity.Actor.DisplayName
			payload["senderEmail"] = activity.Actor.EmailAddress
		}

		select {
		case resultCh <- payload:
		default:
		}
	}

	uc.convClient.On("post", handler)
	uc.convClient.On("share", handler)
	defer uc.convClient.Off("post", handler)
	defer uc.convClient.Off("share", handler)

	// Ensure connected
	uc.mu.Lock()
	if !uc.connected {
		if err := uc.convClient.Connect(); err != nil {
			uc.mu.Unlock()
			return nil, fmt.Errorf("failed to connect Mercury: %w", err)
		}
		uc.connected = true
	}
	uc.mu.Unlock()

	select {
	case result := <-resultCh:
		return result, nil
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("timeout waiting for message from %s after %v", personEmail, timeout)
	}
}

// ListSubscriptions returns all active subscriptions for a session.
func (m *MercuryManager) ListSubscriptions(sessionID string) []*Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var subs []*Subscription
	for _, sub := range m.subscriptions {
		if sessionID == "" || sub.SessionID == sessionID {
			subs = append(subs, sub)
		}
	}
	return subs
}

// getOrCreateConnection returns or creates a Mercury connection for the user.
func (m *MercuryManager) getOrCreateConnection(client *webex.WebexClient, tokHash string) (*userConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if uc, ok := m.userConns[tokHash]; ok {
		uc.mu.Lock()
		uc.refCount++
		uc.mu.Unlock()
		return uc, nil
	}

	// Create conversation client (handles device registration, Mercury, encryption)
	convClient, err := client.Conversation()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize conversation client: %w", err)
	}

	uc := &userConnection{
		client:     client,
		convClient: convClient,
		tokenHash:  tokHash,
		refCount:   1,
	}

	m.userConns[tokHash] = uc
	return uc, nil
}

// buildEventPayload creates a structured notification payload from a conversation activity.
func (m *MercuryManager) buildEventPayload(sub *Subscription, eventType string, activity *conversation.Activity) map[string]interface{} {
	payload := map[string]interface{}{
		"subscriptionId": sub.ID,
		"eventType":      eventType,
		"roomId":         sub.RoomID,
		"timestamp":      activity.Published,
	}

	if activity.Actor != nil {
		payload["sender"] = map[string]interface{}{
			"displayName":  activity.Actor.DisplayName,
			"emailAddress": activity.Actor.EmailAddress,
			"id":           activity.Actor.ID,
		}
	}

	if activity.Target != nil {
		payload["room"] = map[string]interface{}{
			"id":       activity.Target.ID,
			"globalId": activity.Target.GlobalID,
		}
	}

	// Try to get decrypted content
	if activity.Content != "" {
		payload["content"] = activity.Content
	} else if activity.DecryptedObject != nil {
		if activity.DecryptedObject.DisplayName != "" {
			payload["content"] = activity.DecryptedObject.DisplayName
		}
		if activity.DecryptedObject.Content != "" {
			payload["contentHtml"] = activity.DecryptedObject.Content
		}
	}

	return payload
}

// sendNotification sends an MCP notification to the specified session.
func (m *MercuryManager) sendNotification(sessionID string, payload map[string]interface{}) {
	if m.mcpServer == nil {
		return
	}

	// Marshal payload for logging notification
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[Mercury] Failed to marshal notification payload: %v", err)
		return
	}

	if sessionID != "" {
		err = m.mcpServer.SendNotificationToSpecificClient(
			sessionID,
			"notifications/message",
			map[string]any{
				"level":  "info",
				"logger": "webex-mercury",
				"data":   string(data),
			},
		)
	} else {
		m.mcpServer.SendNotificationToAllClients(
			"notifications/message",
			map[string]any{
				"level":  "info",
				"logger": "webex-mercury",
				"data":   string(data),
			},
		)
	}

	if err != nil {
		log.Printf("[Mercury] Failed to send notification to session %s: %v", sessionLabel(sessionID), err)
	} else {
		log.Printf("[Mercury] Sent notification to session %s", sessionLabel(sessionID))
	}
}

func activityID(activity *conversation.Activity) string {
	if activity == nil || activity.ID == "" {
		return "(unknown)"
	}
	return activity.ID
}

func sessionLabel(sessionID string) string {
	if sessionID == "" {
		return "(broadcast)"
	}
	if len(sessionID) <= 8 {
		return sessionID
	}
	return sessionID[:8] + "..."
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}

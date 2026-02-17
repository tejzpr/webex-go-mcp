package streaming

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	webex "github.com/tejzpr/webex-go-sdk/v2"
	"github.com/tejzpr/webex-go-sdk/v2/conversation"

	"github.com/mark3labs/mcp-go/server"
)

// Subscription represents an active Mercury event subscription.
type Subscription struct {
	ID        string
	RoomID    string
	TokenHash string
	SessionID string
	CreatedAt time.Time
	cancel    context.CancelFunc
}

// MercuryManager manages per-user Mercury connections and multiplexes
// conversation events to MCP client sessions as notifications.
type MercuryManager struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription   // subscriptionId → sub
	userConns     map[string]*userConnection // tokenHash → connection
	mcpServer     *server.MCPServer
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
	return &MercuryManager{
		subscriptions: make(map[string]*Subscription),
		userConns:     make(map[string]*userConnection),
		mcpServer:     mcpServer,
	}
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

	// Register event handlers for the requested event types
	for _, eventType := range eventTypes {
		et := eventType // capture
		uc.convClient.On(et, func(activity *conversation.Activity) {
			// Check if subscription is still active
			select {
			case <-subCtx.Done():
				return
			default:
			}

			// Filter by room ID if specified
			if roomID != "" && activity.Target != nil && activity.Target.ID != roomID {
				// Also check GlobalID since Mercury may use internal IDs
				if activity.Target.GlobalID != roomID {
					return
				}
			}

			// Build the notification payload
			payload := m.buildEventPayload(sub, et, activity)

			// Send as MCP notification
			m.sendNotification(sessionID, payload)
		})
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

	// Decrement ref count and potentially disconnect
	m.mu.RLock()
	uc, ok := m.userConns[sub.TokenHash]
	m.mu.RUnlock()

	if ok {
		uc.mu.Lock()
		uc.refCount--
		if uc.refCount <= 0 {
			log.Printf("[Mercury] No more subscriptions for user (hash=%s...), disconnecting", sub.TokenHash[:8])
			uc.convClient.Disconnect()
			uc.connected = false
			uc.mu.Unlock()

			m.mu.Lock()
			delete(m.userConns, sub.TokenHash)
			m.mu.Unlock()
		} else {
			uc.mu.Unlock()
		}
	}

	log.Printf("[Mercury] Subscription %s cancelled", subscriptionID)
	return nil
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
		log.Printf("[Mercury] Failed to send notification to session %s: %v", sessionID, err)
	}
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}

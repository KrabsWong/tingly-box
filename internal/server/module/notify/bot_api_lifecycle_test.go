package notify

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// fakeBotChatManager is a test double for BotChatManager: each func field is
// optional, defaulting to a no-op/false, so a test wires only the method it
// exercises. Field names mirror the interface methods they implement.
type fakeBotChatManager struct {
	listChats    func(botUUID string, includeDisabled bool) ([]ChatSummary, error)
	deleteChat   func(botUUID, chatID string) error
	setDisabled  func(botUUID, chatID string, disabled bool) error
	isDisabled   func(chatID string) bool
}

func (f *fakeBotChatManager) ListChats(botUUID string, includeDisabled bool) ([]ChatSummary, error) {
	if f.listChats == nil {
		return nil, nil
	}
	return f.listChats(botUUID, includeDisabled)
}
func (f *fakeBotChatManager) DeleteChat(botUUID, chatID string) error {
	if f.deleteChat == nil {
		return nil
	}
	return f.deleteChat(botUUID, chatID)
}
func (f *fakeBotChatManager) SetChatDisabled(botUUID, chatID string, disabled bool) error {
	if f.setDisabled == nil {
		return nil
	}
	return f.setDisabled(botUUID, chatID, disabled)
}
func (f *fakeBotChatManager) IsChatDisabled(chatID string) bool {
	if f.isDisabled == nil {
		return false
	}
	return f.isDisabled(chatID)
}

// newLifecycleRouter mounts the chat lifecycle routes with the given manager,
// mirroring RegisterBotRoutes' path shape on a plain group.
func newLifecycleRouter(t *testing.T, ch channel.Channel, chats BotChatManager) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	registry := channel.NewRegistry()
	if ch != nil {
		registry.Register(ch)
	}
	results := interaction.New[interaction.Result](30 * time.Second)
	handler := NewBotAPIHandler(registry, results, chats)

	g := r.Group("/api/v1")
	g.POST("/bots/:bot/notify", handler.Notify)
	g.POST("/bots/:bot/interact", handler.Interact)
	g.DELETE("/bots/:bot/chats/:chat_id", handler.DeleteChat)
	g.PUT("/bots/:bot/chats/:chat_id/disabled", handler.SetChatDisabled)
	return r
}

func TestDeleteChat_OK(t *testing.T) {
	var got string
	r := newLifecycleRouter(t, newFakeChannel("bot-1"), &fakeBotChatManager{
		deleteChat: func(botUUID, chatID string) error {
			got = botUUID + "/" + chatID
			return nil
		},
	})

	w := doJSON(t, r, http.MethodDelete, "/api/v1/bots/bot-1/chats/telegram:123", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got != "bot-1/telegram:123" {
		t.Fatalf("deleter called with %q", got)
	}
}

func TestDeleteChat_NotFound_404(t *testing.T) {
	r := newLifecycleRouter(t, newFakeChannel("bot-1"), &fakeBotChatManager{
		deleteChat: func(string, string) error { return ErrChatNotFound },
	})

	w := doJSON(t, r, http.MethodDelete, "/api/v1/bots/bot-1/chats/nope", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteChat_Locked_409(t *testing.T) {
	r := newLifecycleRouter(t, newFakeChannel("bot-1"), &fakeBotChatManager{
		deleteChat: func(string, string) error { return ErrChatLocked },
	})

	w := doJSON(t, r, http.MethodDelete, "/api/v1/bots/bot-1/chats/locked", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for chat-id lock, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteChat_NoDeleter_503(t *testing.T) {
	r := newLifecycleRouter(t, newFakeChannel("bot-1"), nil)

	w := doJSON(t, r, http.MethodDelete, "/api/v1/bots/bot-1/chats/x", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when unwired, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetChatDisabled_OK(t *testing.T) {
	var gotChat string
	var gotDisabled bool
	r := newLifecycleRouter(t, newFakeChannel("bot-1"), &fakeBotChatManager{
		setDisabled: func(botUUID, chatID string, disabled bool) error {
			gotChat, gotDisabled = chatID, disabled
			return nil
		},
	})

	w := doJSON(t, r, http.MethodPut, "/api/v1/bots/bot-1/chats/telegram:123/disabled", gin.H{"disabled": true})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotChat != "telegram:123" || !gotDisabled {
		t.Fatalf("disabler called with chat=%q disabled=%v", gotChat, gotDisabled)
	}
}

func TestSetChatDisabled_MissingFlag_400(t *testing.T) {
	r := newLifecycleRouter(t, newFakeChannel("bot-1"), &fakeBotChatManager{
		setDisabled: func(string, string, bool) error {
			t.Fatal("disabler should not run without the disabled field")
			return nil
		},
	})

	// Omitted "disabled" must be a 400, not a silent false.
	w := doJSON(t, r, http.MethodPut, "/api/v1/bots/bot-1/chats/x/disabled", gin.H{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing disabled field, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetChatDisabled_NotFound_404(t *testing.T) {
	r := newLifecycleRouter(t, newFakeChannel("bot-1"), &fakeBotChatManager{
		setDisabled: func(string, string, bool) error { return ErrChatNotFound },
	})

	w := doJSON(t, r, http.MethodPut, "/api/v1/bots/bot-1/chats/nope/disabled", gin.H{"disabled": true})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestNotify_DisabledChat_404 covers the outbound half of the blocklist: a
// disabled chat is unreachable for pushes, with the same body an unknown chat
// gets so callers can't distinguish (no probing).
func TestNotify_DisabledChat_404(t *testing.T) {
	ch := newRecordingChannel("bot-1")
	r := newLifecycleRouter(t, ch, &fakeBotChatManager{
		isDisabled: func(chatID string) bool { return chatID == "blocked" },
	})

	w := doJSON(t, r, http.MethodPost, "/api/v1/bots/bot-1/notify", gin.H{
		"chat_id": "blocked", "body": "x",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for disabled chat, got %d: %s", w.Code, w.Body.String())
	}
	if len(ch.sent) != 0 {
		t.Fatalf("notification delivered to disabled chat: %+v", ch.sent)
	}

	// A non-blocked chat on the same wiring still goes through.
	w2 := doJSON(t, r, http.MethodPost, "/api/v1/bots/bot-1/notify", gin.H{
		"chat_id": "open", "body": "x",
	})
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for open chat, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestInteract_DisabledChat_404(t *testing.T) {
	r := newLifecycleRouter(t, newFakeChannel("bot-1"), &fakeBotChatManager{
		isDisabled: func(chatID string) bool { return chatID == "blocked" },
	})

	w := doJSON(t, r, http.MethodPost, "/api/v1/bots/bot-1/interact", gin.H{
		"chat_id": "blocked", "kind": "ask", "title": "x",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for disabled chat, got %d: %s", w.Code, w.Body.String())
	}
}

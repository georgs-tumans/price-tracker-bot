package botfixer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"pricetrackerbot/config"
)

const (
	testToken       = "123456:test-token"
	allowedChatID   = int64(1001)
	forbiddenChatID = int64(2002)
)

// apiCall is one request the bot made to the fake Telegram API.
type apiCall struct {
	Method string
	Fields map[string]string
}

// fakeTelegram is a minimal stand-in for api.telegram.org: it hands out queued updates via getUpdates
// and records every other call the bot makes.
type fakeTelegram struct {
	t      *testing.T
	server *httptest.Server

	mu            sync.Mutex
	pending       []map[string]any
	nextUpdateID  int
	nextMessageID int
	calls         []apiCall
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()

	f := &fakeTelegram{t: t, nextUpdateID: 1, nextMessageID: 500}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)

	return f
}

func (f *fakeTelegram) handle(w http.ResponseWriter, r *http.Request) {
	prefix := "/bot" + testToken + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}

	method := strings.TrimPrefix(r.URL.Path, prefix)

	fields := map[string]string{}
	if err := r.ParseMultipartForm(1 << 20); err == nil {
		for key, values := range r.MultipartForm.Value {
			fields[key] = values[0]
		}
	}

	var result any

	switch method {
	case "getMe":
		result = map[string]any{"id": 1, "is_bot": true, "first_name": "Test", "username": "test_bot"}
	case "getUpdates":
		f.mu.Lock()
		updates := f.pending
		f.pending = nil
		f.mu.Unlock()

		if len(updates) == 0 {
			time.Sleep(20 * time.Millisecond) // Stand-in for long polling
		}

		if updates == nil {
			updates = []map[string]any{}
		}

		result = updates
	case "sendMessage", "editMessageText":
		f.record(method, fields)

		f.mu.Lock()
		f.nextMessageID++
		messageID := f.nextMessageID
		f.mu.Unlock()

		var chatID int64
		_, _ = fmt.Sscan(fields["chat_id"], &chatID)
		result = map[string]any{"message_id": messageID, "date": 0, "chat": map[string]any{"id": chatID, "type": "private"}, "text": fields["text"]}
	default: // deleteWebhook, answerCallbackQuery, deleteMessage
		f.record(method, fields)
		result = true
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func (f *fakeTelegram) record(method string, fields map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, apiCall{Method: method, Fields: fields})
}

func (f *fakeTelegram) queueMessage(chatID int64, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	message := map[string]any{
		"message_id": 1,
		"date":       0,
		"chat":       map[string]any{"id": chatID, "type": "private"},
		"from":       map[string]any{"id": chatID, "is_bot": false, "first_name": "Tester"},
		"text":       text,
	}
	if strings.HasPrefix(text, "/") {
		command := strings.SplitN(text, " ", 2)[0]
		message["entities"] = []map[string]any{{"type": "bot_command", "offset": 0, "length": len(command)}}
	}

	f.pending = append(f.pending, map[string]any{"update_id": f.nextUpdateID, "message": message})
	f.nextUpdateID++
}

func (f *fakeTelegram) queueButtonClick(chatID int64, messageID int, data string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.pending = append(f.pending, map[string]any{
		"update_id": f.nextUpdateID,
		"callback_query": map[string]any{
			"id":            fmt.Sprintf("cb-%d", f.nextUpdateID),
			"from":          map[string]any{"id": chatID, "is_bot": false, "first_name": "Tester"},
			"chat_instance": "test",
			"data":          data,
			"message": map[string]any{
				"message_id": messageID,
				"date":       1,
				"chat":       map[string]any{"id": chatID, "type": "private"},
				"text":       "menu",
			},
		},
	})
	f.nextUpdateID++
}

func (f *fakeTelegram) callsSnapshot() []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]apiCall(nil), f.calls...)
}

// waitForCall waits until a call matching the predicate has been recorded and returns it.
func (f *fakeTelegram) waitForCall(description string, match func(apiCall) bool) apiCall {
	f.t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, call := range f.callsSnapshot() {
			if match(call) {
				return call
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	f.t.Fatalf("timed out waiting for: %s\nrecorded calls: %+v", description, f.callsSnapshot())

	return apiCall{}
}

func textContains(method, substring string) func(apiCall) bool {
	return func(call apiCall) bool {
		return call.Method == method && strings.Contains(call.Fields["text"], substring)
	}
}

func newTestConfig(t *testing.T, stateFile string) *config.Configuration {
	t.Helper()

	// A tracked "API" that reports a value of 5, which meets the "< 10" notification criterion
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"price": 5}`))
	}))
	t.Cleanup(api.Close)

	return &config.Configuration{
		BotAPIKey:        testToken,
		ErrorNotifyLimit: 3,
		AllowedChatIDs:   []int64{allowedChatID},
		StateFile:        stateFile,
		APITrackers: []*config.Tracker{{
			Code:               "bonds",
			DataURL:            api.URL,
			ViewURL:            "https://example.com/bonds",
			Interval:           "1h",
			DataExtractionPath: "price",
			NotifyCriteria:     []config.NotifyCriteria{{Operator: "<", Value: "10"}},
		}},
	}
}

// startTestBot runs the bot until the test ends: t.Context() is cancelled when the test finishes,
// and the cleanup waits for the bot to stop.
func startTestBot(t *testing.T, fake *fakeTelegram, cfg *config.Configuration) *BotFixer {
	t.Helper()

	botFixer, err := newBotFixer(cfg,
		bot.WithServerURL(fake.server.URL),
		bot.WithHTTPClient(2*time.Second, &http.Client{Timeout: 5 * time.Second}),
	)
	if err != nil {
		t.Fatalf("newBotFixer: %v", err)
	}

	done := make(chan struct{})

	go func() {
		defer close(done)
		botFixer.Run(t.Context())
	}()

	t.Cleanup(func() {
		<-done
		botFixer.CommandHandler.StopAllTrackers()
	})

	return botFixer
}

func TestBotEndToEnd(t *testing.T) {
	fake := newFakeTelegram(t)
	stateFile := filepath.Join(t.TempDir(), "state.json")
	startTestBot(t, fake, newTestConfig(t, stateFile))

	fake.waitForCall("deleteWebhook at startup", func(c apiCall) bool { return c.Method == "deleteWebhook" })

	// /status lists the trackers and attaches an inline menu
	fake.queueMessage(allowedChatID, "/status")
	status := fake.waitForCall("status message", textContains("sendMessage", "All available trackers"))
	if status.Fields["parse_mode"] != "HTML" {
		t.Errorf("parse_mode = %q, want HTML", status.Fields["parse_mode"])
	}

	if !strings.Contains(status.Fields["reply_markup"], `"callback_data":"/run bonds"`) {
		t.Errorf("status menu is missing the start button: %s", status.Fields["reply_markup"])
	}

	// Messages from other chats are ignored
	fake.queueMessage(forbiddenChatID, "/help")

	// Clicking "Start [bonds]" on the status message edits it and starts the tracker
	fake.queueButtonClick(allowedChatID, 42, "/run bonds")
	fake.waitForCall("callback answered", func(c apiCall) bool { return c.Method == "answerCallbackQuery" })
	edited := fake.waitForCall("status message edited", textContains("editMessageText", "Tracker 'bonds' has been started"))
	if edited.Fields["message_id"] != "42" {
		t.Errorf("edited message_id = %q, want 42", edited.Fields["message_id"])
	}

	if !strings.Contains(edited.Fields["reply_markup"], `"callback_data":"back"`) {
		t.Errorf("edited message is missing the Return button: %s", edited.Fields["reply_markup"])
	}

	// The tracker runs right away, finds 5 < 10 and notifies the chat
	notification := fake.waitForCall("tracker notification", textContains("sendMessage", "Good news, tracker <b>bonds</b>"))
	if _, hasMarkup := notification.Fields["reply_markup"]; hasMarkup {
		t.Errorf("a message without a menu must not send reply_markup, got %q", notification.Fields["reply_markup"])
	}

	// The running tracker is saved for the next start
	data, err := os.ReadFile(stateFile)
	if err != nil || !strings.Contains(string(data), `"code": "bonds"`) {
		t.Errorf("state file does not contain the running tracker: %s (%v)", data, err)
	}

	// Changing the interval via the menu: a reply keyboard is shown, the typed value is applied and saved
	fake.queueButtonClick(allowedChatID, 42, "/interval bonds")
	prompt := fake.waitForCall("interval prompt", textContains("sendMessage", "Send me the new interval value"))
	if !strings.Contains(prompt.Fields["reply_markup"], `"one_time_keyboard":true`) || !strings.Contains(prompt.Fields["reply_markup"], `"text":"10m"`) {
		t.Errorf("interval prompt is missing the reply keyboard: %s", prompt.Fields["reply_markup"])
	}

	fake.queueMessage(allowedChatID, "10m")
	removal := fake.waitForCall("keyboard removal", textContains("sendMessage", "processing input..."))
	if !strings.Contains(removal.Fields["reply_markup"], `"remove_keyboard":true`) {
		t.Errorf("keyboard removal message has the wrong markup: %s", removal.Fields["reply_markup"])
	}

	fake.waitForCall("keyboard removal message deleted", func(c apiCall) bool { return c.Method == "deleteMessage" })
	fake.waitForCall("interval updated", textContains("sendMessage", "run interval successfully updated to 10m"))

	data, _ = os.ReadFile(stateFile)
	if !strings.Contains(string(data), `"interval": "10m0s"`) {
		t.Errorf("state file does not contain the new interval: %s", data)
	}

	// "Return" goes back to the previous menu
	fake.queueButtonClick(allowedChatID, 42, "/status")
	fake.queueButtonClick(allowedChatID, 42, "/run bonds")
	fake.queueButtonClick(allowedChatID, 42, "back")
	fake.waitForCall("return to status", textContains("editMessageText", "All available trackers"))

	// "Return" on an old message after a restart (empty navigation history) must not crash the bot
	fake.queueButtonClick(allowedChatID, 43, "back")
	fake.queueButtonClick(allowedChatID, 43, "back")
	fake.queueMessage(allowedChatID, "/help")
	fake.waitForCall("bot still responds", textContains("sendMessage", "Welcome to the bot help section"))

	for _, call := range fake.callsSnapshot() {
		if call.Fields["chat_id"] == fmt.Sprint(forbiddenChatID) {
			t.Errorf("bot replied to a chat that is not allowed: %+v", call)
		}
	}
}

func TestBotResumesTrackersAfterRestart(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	state := fmt.Sprintf(`[{"code": "bonds", "chatId": %d, "interval": "30m0s"}, {"code": "removed-from-config", "chatId": %d, "interval": "1h0m0s"}]`, allowedChatID, allowedChatID)
	if err := os.WriteFile(stateFile, []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}

	fake := newFakeTelegram(t)
	botFixer := startTestBot(t, fake, newTestConfig(t, stateFile))

	restarted := fake.waitForCall("restart notice", textContains("sendMessage", "The bot was restarted"))
	if !strings.Contains(restarted.Fields["text"], "resumed 1 tracker(s): <b>bonds</b>") {
		t.Errorf("unexpected restart notice: %q", restarted.Fields["text"])
	}

	tracker := botFixer.CommandHandler.GetActiveTracker("bonds")
	if tracker == nil {
		t.Fatal("tracker 'bonds' was not resumed")
	}

	if got := tracker.Status().CurrentInterval; got != 30*time.Minute {
		t.Errorf("resumed interval = %s, want 30m", got)
	}

	// The tracker that no longer exists in the configuration is dropped from the state file
	data, _ := os.ReadFile(stateFile)
	if strings.Contains(string(data), "removed-from-config") {
		t.Errorf("state file still contains a tracker that could not be resumed: %s", data)
	}
}

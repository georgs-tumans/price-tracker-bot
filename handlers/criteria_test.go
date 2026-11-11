package handlers

import (
	"os"
	"slices"
	"strings"
	"testing"

	"pricetrackerbot/config"
)

// Telegram rejects a message whose button data is longer than this many bytes.
const telegramCallbackDataLimit = 64

func TestParseCriterion(t *testing.T) {
	current := &config.NotifyCriteria{Operator: ">=", Value: "10"}

	tests := []struct {
		input   string
		current *config.NotifyCriteria
		want    config.NotifyCriteria
		wantErr bool
	}{
		{input: "<= 45.5", want: config.NotifyCriteria{Operator: "<=", Value: "45.5"}},
		{input: "<=45,5", want: config.NotifyCriteria{Operator: "<=", Value: "45.5"}},
		{input: " >  1 234 ", want: config.NotifyCriteria{Operator: ">", Value: "1234"}},
		{input: "= -0.50", want: config.NotifyCriteria{Operator: "=", Value: "-0.5"}},
		{input: "45", current: current, want: config.NotifyCriteria{Operator: ">=", Value: "45"}},
		{input: "1e5", current: current, want: config.NotifyCriteria{Operator: ">=", Value: "100000"}},
		{input: "45", wantErr: true}, // Adding needs an operator
		{input: "!= 5", wantErr: true},
		{input: "=> 5", wantErr: true},
		{input: "<=", wantErr: true},
		{input: "", current: current, wantErr: true},
		{input: "< cheap", wantErr: true},
		{input: "< NaN", wantErr: true},
		{input: "< Inf", wantErr: true},
		{input: "< 1e400", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseCriterion(tt.input, tt.current)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseCriterion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("parseCriterion(%q) = %+v, want %+v", tt.input, got, tt.want)
			}

			// A stored criterion must also pass the tracker config rules, or it would be dropped on the next start
			if err == nil {
				if validationErr := config.ValidateNotifyCriteria([]config.NotifyCriteria{got}); validationErr != nil {
					t.Errorf("parsed criterion %+v fails the config validation: %v", got, validationErr)
				}
			}
		})
	}
}

func TestParseCriteriaAction(t *testing.T) {
	tests := []struct {
		input   string
		want    criteriaAction
		wantErr bool
	}{
		{input: "add <= 45", want: criteriaAction{name: criteriaAdd, criterion: "<= 45"}},
		{input: "add", want: criteriaAction{name: criteriaAdd}},
		{input: "edit 2 v00ff >= 3", want: criteriaAction{name: criteriaEdit, index: 2, version: "v00ff", criterion: ">= 3"}},
		{input: "edit 2 45", want: criteriaAction{name: criteriaEdit, index: 2, criterion: "45"}},
		{input: "edit 2", want: criteriaAction{name: criteriaEdit, index: 2}},
		{input: "del 1 v1234", want: criteriaAction{name: criteriaDelete, index: 1, version: "v1234"}},
		{input: "del 1", want: criteriaAction{name: criteriaDelete, index: 1}},
		{input: "", wantErr: true},
		{input: "remove 1", wantErr: true},
		{input: "edit", wantErr: true},
		{input: "edit x 4", wantErr: true},
		{input: "edit 0 4", wantErr: true},
		{input: "del 1 45", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseCriteriaAction(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseCriteriaAction(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("parseCriteriaAction(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCriteriaSuggestions(t *testing.T) {
	tests := []struct {
		name      string
		reference float64
		current   *config.NotifyCriteria
		want      []string
	}{
		{name: "lower is better", reference: 100, current: &config.NotifyCriteria{Operator: "<=", Value: "90"}, want: []string{"<= 100", "<= 95", "<= 90"}},
		{name: "higher is better", reference: 100, current: &config.NotifyCriteria{Operator: ">", Value: "1"}, want: []string{"> 100", "> 105", "> 110"}},
		{name: "equal", reference: 100, current: &config.NotifyCriteria{Operator: "=", Value: "1"}, want: []string{"= 100"}},
		{name: "adding", reference: 47.2, want: []string{"<= 44.84", "<= 42.48", ">= 49.56"}},
		{name: "negative value", reference: -10, current: &config.NotifyCriteria{Operator: "<=", Value: "1"}, want: []string{"<= -10", "<= -10.5", "<= -11"}},
		{name: "zero has no steps", reference: 0, current: &config.NotifyCriteria{Operator: "<", Value: "1"}, want: []string{"< 0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := criteriaSuggestions(tt.reference, tt.current); !slices.Equal(got, tt.want) {
				t.Errorf("criteriaSuggestions() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Starts "bonds" and waits for its first run, which records 5 and sends a notification (5 < 10).
func startBonds(t *testing.T, ch *CommandHandler, messenger *recordingMessenger) *Tracker {
	t.Helper()

	if err := ch.handleStart("bonds", testChatID, nil); err != nil {
		t.Fatalf("handleStart: %v", err)
	}

	waitFor(t, func() bool { return messenger.hasMessage("Good news") })

	return ch.GetActiveTracker("bonds")
}

func TestCriteriaChanges(t *testing.T) {
	ch, messenger := newTestCommandHandler(t)
	tracker := startBonds(t, ch, messenger)

	steps := []struct {
		command    string
		want       []config.NotifyCriteria
		wantNotice string
	}{
		{command: "/criteria bonds add >= 100", want: []config.NotifyCriteria{{Operator: "<", Value: "10"}, {Operator: ">=", Value: "100"}},
			wantNotice: "Added: tracked value &gt;= 100\nWith the last recorded value 5.00 it is not met."},
		{command: "/criteria bonds edit 1 7", want: []config.NotifyCriteria{{Operator: "<", Value: "7"}, {Operator: ">=", Value: "100"}},
			wantNotice: "Criterion #1 is now: tracked value &lt; 7\nWith the last recorded value 5.00 it is already met."},
		{command: "/criteria bonds del 2", want: []config.NotifyCriteria{{Operator: "<", Value: "7"}},
			wantNotice: "Removed: tracked value &gt;= 100"},
		{command: "/criteria bonds del 1", want: []config.NotifyCriteria{},
			wantNotice: "Removed: tracked value &lt; 7"},
	}

	for _, step := range steps {
		if err := ch.HandleCommand(testChatID, step.command, nil, false); err != nil {
			t.Fatalf("%s: %v", step.command, err)
		}

		if got := ch.effectiveCriteria("bonds"); !slices.Equal(got, step.want) {
			t.Errorf("%s: criteria = %+v, want %+v", step.command, got, step.want)
		}

		if got := tracker.Status().NotifyCriteria; !slices.Equal(got, step.want) {
			t.Errorf("%s: running tracker criteria = %+v, want %+v", step.command, got, step.want)
		}

		if page := messenger.last().text; !strings.Contains(page, step.wantNotice) {
			t.Errorf("%s: page %q does not contain %q", step.command, page, step.wantNotice)
		}
	}

	if page := messenger.last().text; !strings.Contains(page, "No criteria: the tracker never notifies") {
		t.Errorf("page without criteria: %q", page)
	}

	// Removing every criterion is a change too, and must survive a restart
	data, _ := os.ReadFile(ch.config.StateFile)
	if !strings.Contains(string(data), `"notifyCriteria": []`) {
		t.Errorf("state file does not contain the empty criteria: %s", data)
	}

	// Changing back to the config drops the change
	for _, command := range []string{"/criteria bonds add < 10", "/criteria bonds edit 1 < 10.0"} {
		if err := ch.HandleCommand(testChatID, command, nil, false); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}

	if settings := ch.trackerSettings("bonds"); settings.NotifyCriteria != nil {
		t.Errorf("criteria equal to the config are still saved as a change: %+v", *settings.NotifyCriteria)
	}
}

func TestCriteriaRejectsInvalidInput(t *testing.T) {
	ch, messenger := newTestCommandHandler(t)
	configured := ch.config.APITrackers[0].NotifyCriteria

	tests := []struct {
		command    string
		wantNotice string
	}{
		{command: "/criteria bonds add <= <b>", wantNotice: "&#39;&lt;b&gt;&#39; is not a number"},
		{command: "/criteria bonds add 45", wantNotice: "operator"},
		{command: "/criteria bonds edit 5 1", wantNotice: "There is no criterion #5"},
		{command: "/criteria bonds remove 1", wantNotice: "Use: add"},
		{command: "/criteria unknown add < 1", wantNotice: "<b>unknown</b> not found"},
	}

	for _, tt := range tests {
		if err := ch.HandleCommand(testChatID, tt.command, nil, false); err == nil {
			t.Errorf("%s: expected an error", tt.command)
		}

		if text := messenger.last().text; !strings.Contains(text, tt.wantNotice) {
			t.Errorf("%s: message %q does not contain %q", tt.command, text, tt.wantNotice)
		}

		if got := ch.effectiveCriteria("bonds"); !slices.Equal(got, configured) {
			t.Errorf("%s: criteria changed to %+v", tt.command, got)
		}
	}

	for len(ch.effectiveCriteria("bonds")) < maxNotifyCriteria {
		if err := ch.HandleCommand(testChatID, "/criteria bonds add > 1", nil, false); err != nil {
			t.Fatalf("adding up to the limit: %v", err)
		}
	}

	if err := ch.HandleCommand(testChatID, "/criteria bonds add > 1", nil, false); err == nil {
		t.Error("expected an error when adding past the limit")
	}

	if got := len(ch.effectiveCriteria("bonds")); got != maxNotifyCriteria {
		t.Errorf("%d criteria, want the limit of %d", got, maxNotifyCriteria)
	}
}

// A button on an older message must not change a criterion that has moved or changed since.
func TestCriteriaStaleButton(t *testing.T) {
	ch, messenger := newTestCommandHandler(t)
	messageID := 42
	oldVersion := criteriaVersion(ch.effectiveCriteria("bonds"))

	if err := ch.HandleCommand(testChatID, "/criteria bonds add > 1", nil, false); err != nil {
		t.Fatalf("add: %v", err)
	}

	before := ch.effectiveCriteria("bonds")
	for _, action := range []string{"del 1 ", "edit 1 "} {
		if err := ch.HandleCommand(testChatID, "/criteria bonds "+action+oldVersion, &messageID, false); err == nil {
			t.Errorf("%s with an old version: expected an error", action)
		}

		if got := ch.effectiveCriteria("bonds"); !slices.Equal(got, before) {
			t.Errorf("%s with an old version changed the criteria to %+v", action, got)
		}

		if text := messenger.last().text; !strings.Contains(text, "The criteria have changed since") {
			t.Errorf("%s with an old version: unexpected message %q", action, text)
		}
	}
}

// The menu flow: status → criteria page → edit button → typed value → page again → Return to the status.
func TestCriteriaEditViaButtons(t *testing.T) {
	ch, messenger := newTestCommandHandler(t)
	startBonds(t, ch, messenger)
	messageID := 42
	state := ch.GetUserNavigationState(testChatID)
	state.BackButtonEnabled = true

	if err := ch.HandleCommand(testChatID, "/status bonds", &messageID, false); err != nil {
		t.Fatalf("status: %v", err)
	}

	if !hasButton(messenger.last().menu, "/criteria bonds") {
		t.Fatalf("status page is missing the criteria button: %+v", messenger.last().menu)
	}

	if err := ch.HandleCommand(testChatID, "/criteria bonds", &messageID, false); err != nil {
		t.Fatalf("criteria page: %v", err)
	}

	editButton := "/criteria bonds edit 1 " + criteriaVersion(ch.effectiveCriteria("bonds"))
	if page := messenger.last(); !hasButton(page.menu, editButton) || !hasButton(page.menu, "/criteria bonds add") {
		t.Fatalf("criteria page is missing the edit or add button: %+v", page.menu)
	}

	if err := ch.HandleCommand(testChatID, editButton, &messageID, false); err != nil {
		t.Fatalf("edit button: %v", err)
	}

	if !state.AwaitingUserInput() || !strings.Contains(messenger.last().text, "Send me the new criterion #1") {
		t.Fatalf("the edit button did not ask for the criterion: %q", messenger.last().text)
	}

	// Suggestions start from the last recorded value 5 and keep the "<" operator
	if got := messenger.lastKeyboardOptions(); !slices.Equal(got, []string{"< 5", "< 4.75", "< 4.5"}) {
		t.Errorf("suggestions = %q", got)
	}

	if err := ch.HandleUserInput(testChatID, "<= 6", nil); err != nil {
		t.Fatalf("HandleUserInput: %v", err)
	}

	if got := ch.effectiveCriteria("bonds"); !slices.Equal(got, []config.NotifyCriteria{{Operator: "<=", Value: "6"}}) {
		t.Errorf("criteria after the typed value = %+v", got)
	}

	if state.AwaitingUserInput() || !strings.Contains(messenger.last().text, "Criterion #1 is now: tracked value &lt;= 6") {
		t.Errorf("unexpected state after the typed value: awaiting %t, message %q", state.AwaitingUserInput(), messenger.last().text)
	}

	// The change itself is not a step in the history: Return goes from the criteria page to the status page
	if err := ch.HandleReturn(testChatID, &messageID); err != nil {
		t.Fatalf("HandleReturn: %v", err)
	}

	if text := messenger.last().text; !strings.Contains(text, "Status for tracker bonds") {
		t.Errorf("Return showed %q, want the tracker status", text)
	}
}

func TestCriteriaPageResetButton(t *testing.T) {
	ch, messenger := newTestCommandHandler(t)

	if err := ch.HandleCommand(testChatID, "/criteria bonds add > 1", nil, false); err != nil {
		t.Fatalf("add: %v", err)
	}

	resetButton := "/reset bonds " + resetFromCriteriaParam
	if !hasButton(messenger.last().menu, resetButton) {
		t.Fatalf("criteria page with changes is missing the reset button: %+v", messenger.last().menu)
	}

	if err := ch.HandleCommand(testChatID, resetButton, nil, false); err != nil {
		t.Fatalf("reset: %v", err)
	}

	page := messenger.last()
	if !strings.Contains(page.text, "Notify criteria for bonds") || hasButton(page.menu, resetButton) {
		t.Errorf("reset from the criteria page should show the unchanged criteria page: %q %+v", page.text, page.menu)
	}
}

func TestCriteriaButtonsFitTelegramLimit(t *testing.T) {
	criteria := make([]config.NotifyCriteria, maxNotifyCriteria)
	for i := range criteria {
		criteria[i] = config.NotifyCriteria{Operator: ">=", Value: "123456789.123456"}
	}

	code := strings.Repeat("c", config.MaxTrackerCodeLength)
	messenger := &recordingMessenger{}
	ch := NewCommandHandler(messenger, &config.Configuration{
		APITrackers: []*config.Tracker{{Code: code, DataURL: "https://example.com", Interval: "1h", DataExtractionPath: "price"}},
	})
	ch.setSettings(code, TrackerSettings{NotifyCriteria: &criteria})
	ch.GetUserNavigationState(testChatID).BackButtonEnabled = true

	ch.showCriteriaPage(code, testChatID, "")
	_ = ch.handleStatus(code, testChatID, nil)

	for _, message := range messenger.sentMessages() {
		if message.menu == nil {
			continue
		}

		for _, row := range message.menu.InlineKeyboard {
			for _, button := range row {
				if len(button.CallbackData) > telegramCallbackDataLimit {
					t.Errorf("button %q has %d bytes of data, the limit is %d", button.CallbackData, len(button.CallbackData), telegramCallbackDataLimit)
				}
			}
		}
	}
}

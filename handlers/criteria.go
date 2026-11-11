package handlers

import (
	"errors"
	"fmt"
	"hash/fnv"
	"html"
	"log"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"pricetrackerbot/config"
	"pricetrackerbot/helpers"
)

const (
	criteriaAdd    = "add"
	criteriaEdit   = "edit"
	criteriaDelete = "del"

	// Keeps the criteria page readable and the button data short (a two-digit number at most).
	maxNotifyCriteria = 10

	// Buttons carry a short version of the criteria they were made for, so that an older message can't change a
	// criterion that has moved or changed since: "v" and 4 hex digits.
	criteriaVersionPrefix = "v"
	criteriaVersionLength = 5
	criteriaVersionMask   = 0xffff

	// Marks the "Reset to config" button of the criteria page, so that the reset shows that page again.
	resetFromCriteriaParam = "c"

	// Suggested values are rounded to cents.
	suggestionPrecision = 100

	criteriaUsage = "use: add <criterion>, edit <number> <criterion> or del <number>; a criterion is e.g. <= 45.5"
)

// The operators a criterion can have; two-character ones first, so that "<=" is not read as "<".
var criteriaOperators = []string{"<=", ">=", "<", ">", "="}

// How far the suggested values are from the reference value: as is, 5% and 10% towards a better deal.
var suggestionSteps = []float64{0, 0.05, 0.10}

type criteriaAction struct {
	name      string
	index     int    // 1-based; edit and del only
	version   string // The criteria version a button was made for; empty for typed commands
	criterion string // The new criterion for add and edit; empty to ask for it
}

// handleCriteria shows the notify criteria page or changes the criteria. Changes go to the running tracker right away
// and are kept like the interval: through stop, run and restarts, until reset.
func (ch *CommandHandler) handleCriteria(code string, chatID int64, commandParam *string) error {
	if ch.config.GetTrackerData(code) == nil {
		log.Printf("[CommandHandler] Tracker '%s' not found for criteria", code)
		ch.handleCommandMessage(chatID, "Tracker <b>"+html.EscapeString(code)+"</b> not found", nil)

		return errUnrecognizedTracker
	}

	if commandParam == nil {
		ch.showCriteriaPage(code, chatID, "")

		return nil
	}

	action, err := parseCriteriaAction(*commandParam)
	if err != nil {
		ch.showCriteriaPage(code, chatID, errorNotice(err))

		return err
	}

	criteria := ch.effectiveCriteria(code)

	if action.version != "" && action.version != criteriaVersion(criteria) {
		ch.showCriteriaPage(code, chatID, "The criteria have changed since that message, so nothing was changed. These are the current ones.")

		return errors.New("criteria changed since the button was made")
	}

	if action.index > len(criteria) {
		ch.showCriteriaPage(code, chatID, fmt.Sprintf("There is no criterion #%d.", action.index))

		return errors.New("no such criterion")
	}

	if action.name != criteriaDelete && action.criterion == "" {
		ch.askForCriterion(code, chatID, action, criteria)

		return nil
	}

	updated, notice, err := applyCriteriaAction(criteria, action)
	if err != nil {
		ch.showCriteriaPage(code, chatID, errorNotice(err))

		return err
	}

	ch.updateCriteria(code, updated)

	if action.name != criteriaDelete {
		notice += ch.criterionFeedback(code, updated[action.indexIn(updated)])
	}

	ch.showCriteriaPage(code, chatID, notice)

	return nil
}

// Shows an error about the user's input as a notice: HTML-escaped and starting with a capital letter.
func errorNotice(err error) string {
	message := err.Error()
	if message == "" {
		return ""
	}

	return html.EscapeString(strings.ToUpper(message[:1]) + message[1:])
}

// Returns where the added or edited criterion is in the updated list.
func (a criteriaAction) indexIn(updated []config.NotifyCriteria) int {
	if a.name == criteriaAdd {
		return len(updated) - 1
	}

	return a.index - 1
}

// Parses e.g. "add <= 45", "edit 2 v1a2b 45" or "del 1".
func parseCriteriaAction(param string) (criteriaAction, error) {
	fields := strings.Fields(param)
	if len(fields) == 0 {
		return criteriaAction{}, errors.New(criteriaUsage)
	}

	action := criteriaAction{name: fields[0]}
	rest := fields[1:]

	switch action.name {
	case criteriaAdd:
	case criteriaEdit, criteriaDelete:
		if len(rest) == 0 {
			return criteriaAction{}, errors.New(criteriaUsage)
		}

		index, err := strconv.Atoi(rest[0])
		if err != nil || index < 1 {
			return criteriaAction{}, fmt.Errorf("'%s' is not a criterion number; %s", rest[0], criteriaUsage)
		}

		action.index = index
		rest = rest[1:]

		if len(rest) > 0 && isCriteriaVersion(rest[0]) {
			action.version = rest[0]
			rest = rest[1:]
		}
	default:
		return criteriaAction{}, errors.New(criteriaUsage)
	}

	if action.name == criteriaDelete && len(rest) > 0 {
		return criteriaAction{}, errors.New(criteriaUsage)
	}

	action.criterion = strings.Join(rest, " ")

	return action, nil
}

// Parses a criterion such as "<= 45.5", "<=45,5" or "1 234". Without an operator the current criterion's operator is
// kept; current is nil when adding. The value is stored in a plain form that also passes the tracker config rules.
func parseCriterion(text string, current *config.NotifyCriteria) (config.NotifyCriteria, error) {
	text = strings.Join(strings.Fields(text), "")

	var operator string
	for _, candidate := range criteriaOperators {
		if strings.HasPrefix(text, candidate) {
			operator = candidate
			text = strings.TrimPrefix(text, candidate)

			break
		}
	}

	if operator == "" {
		if current == nil {
			return config.NotifyCriteria{}, errors.New("a new criterion needs an operator, e.g. <= 45")
		}

		operator = current.Operator
	}

	value, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", "."), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return config.NotifyCriteria{}, fmt.Errorf("'%s' is not a number; a criterion is e.g. <= 45.5", text)
	}

	criterion := config.NotifyCriteria{Operator: operator, Value: strconv.FormatFloat(value, 'f', -1, 64)}
	if err := config.ValidateNotifyCriteria([]config.NotifyCriteria{criterion}); err != nil {
		return config.NotifyCriteria{}, fmt.Errorf("invalid criterion: %w", err)
	}

	return criterion, nil
}

// Returns the changed criteria and an HTML notice that describes the change.
func applyCriteriaAction(criteria []config.NotifyCriteria, action criteriaAction) ([]config.NotifyCriteria, string, error) {
	updated := slices.Clone(criteria)

	switch action.name {
	case criteriaAdd:
		if len(criteria) >= maxNotifyCriteria {
			return nil, "", fmt.Errorf("a tracker can have at most %d criteria", maxNotifyCriteria)
		}

		criterion, err := parseCriterion(action.criterion, nil)
		if err != nil {
			return nil, "", err
		}

		return append(updated, criterion), "Added: " + helpers.FormatCriterion(criterion), nil
	case criteriaEdit:
		criterion, err := parseCriterion(action.criterion, &criteria[action.index-1])
		if err != nil {
			return nil, "", err
		}

		updated[action.index-1] = criterion

		return updated, fmt.Sprintf("Criterion #%d is now: %s", action.index, helpers.FormatCriterion(criterion)), nil
	default:
		removed := criteria[action.index-1]

		return slices.Delete(updated, action.index-1, action.index), "Removed: " + helpers.FormatCriterion(removed), nil
	}
}

// Stores the criteria as a change (dropped again if they equal the config) and gives them to the running tracker.
func (ch *CommandHandler) updateCriteria(code string, criteria []config.NotifyCriteria) {
	settings := ch.trackerSettings(code)
	settings.NotifyCriteria = &criteria
	ch.setSettings(code, settings)

	if tracker := ch.GetActiveTracker(code); tracker != nil {
		tracker.SetNotifyCriteria(criteria)
	}

	ch.saveState()
	log.Printf("[CommandHandler] Updated tracker '%s' notify criteria to %+v", code, criteria)
}

// Returns the criteria the tracker uses: the changed ones, or else the configured ones.
func (ch *CommandHandler) effectiveCriteria(code string) []config.NotifyCriteria {
	if settings := ch.trackerSettings(code); settings.NotifyCriteria != nil {
		return *settings.NotifyCriteria
	}

	if trackerData := ch.config.GetTrackerData(code); trackerData != nil {
		return slices.Clone(trackerData.NotifyCriteria)
	}

	return nil
}

// Returns the last value the running tracker recorded, as shown and as a number.
func (ch *CommandHandler) lastRecordedValue(code string) (string, float64, bool) {
	tracker := ch.GetActiveTracker(code)
	if tracker == nil {
		return "", 0, false
	}

	text := tracker.Status().LastRecordedValue
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return "", 0, false
	}

	return text, value, true
}

// Tells whether a changed criterion is met by the last recorded value, without fetching the site again.
func (ch *CommandHandler) criterionFeedback(code string, criterion config.NotifyCriteria) string {
	text, value, ok := ch.lastRecordedValue(code)
	if !ok {
		return ""
	}

	target, err := strconv.ParseFloat(criterion.Value, 64)
	if err != nil {
		return ""
	}

	met, err := helpers.CompareNumbers(value, target, criterion.Operator)
	if err != nil {
		return ""
	}

	if met {
		return fmt.Sprintf("\nWith the last recorded value %s it is already met.", text)
	}

	return fmt.Sprintf("\nWith the last recorded value %s it is not met.", text)
}

// Asks the user to type in the new criterion, with a few suggestions as reply buttons.
func (ch *CommandHandler) askForCriterion(code string, chatID int64, action criteriaAction, criteria []config.NotifyCriteria) {
	pendingCommand := "/criteria " + code + " " + action.name
	prompt := "Send me the new criterion.\n\n"

	var current *config.NotifyCriteria
	if action.name == criteriaEdit {
		current = &criteria[action.index-1]
		// Pinned to the criteria the user is looking at, in case they change before the answer arrives
		pendingCommand += fmt.Sprintf(" %d %s", action.index, criteriaVersion(criteria))
		prompt = fmt.Sprintf("Send me the new criterion #%d, now: <i>%s</i>.\nA number alone keeps the operator.\n\n", action.index, helpers.FormatCriterion(*current))
	}

	prompt += "The format: <i>[operator] [number]</i>, e.g. <i>&lt;= 45.5</i>\nOperators: &lt; &lt;= = &gt;= &gt;"

	// The suggestions start from the last recorded value, or else from the criterion being edited
	var suggestions []string
	if _, reference, ok := ch.lastRecordedValue(code); ok {
		suggestions = criteriaSuggestions(reference, current)
	} else if current != nil {
		if reference, err := strconv.ParseFloat(current.Value, 64); err == nil {
			suggestions = criteriaSuggestions(reference, current)
		}
	}

	keyboard := helpers.GetReplyKeyboard(suggestions)
	navigationState := ch.GetUserNavigationState(chatID)
	navigationState.CustomKeyboardActive = keyboard != nil
	navigationState.PendingCommand = pendingCommand

	ch.messenger.SendHTMLWithKeyboard(chatID, prompt, keyboard)
}

// Returns suggested criteria around the reference value. When editing they keep the operator and go towards a better
// deal (lower for "<", higher for ">"); when adding (current is nil) they offer a lower and a higher target.
func criteriaSuggestions(reference float64, current *config.NotifyCriteria) []string {
	step := func(direction, size float64) string {
		value := reference + direction*math.Abs(reference)*size

		return strconv.FormatFloat(math.Round(value*suggestionPrecision)/suggestionPrecision, 'f', -1, 64)
	}

	var suggestions []string
	add := func(suggestion string) {
		if !slices.Contains(suggestions, suggestion) {
			suggestions = append(suggestions, suggestion)
		}
	}

	if current == nil {
		add("<= " + step(-1, suggestionSteps[1]))
		add("<= " + step(-1, suggestionSteps[2]))
		add(">= " + step(1, suggestionSteps[1]))

		return suggestions
	}

	direction := 0.0
	switch current.Operator {
	case "<", "<=":
		direction = -1
	case ">", ">=":
		direction = 1
	}

	for _, size := range suggestionSteps {
		add(current.Operator + " " + step(direction, size))
	}

	return suggestions
}

func criteriaVersion(criteria []config.NotifyCriteria) string {
	hash := fnv.New32a()
	for _, criterion := range criteria {
		fmt.Fprintf(hash, "%s %s;", criterion.Operator, criterion.Value)
	}

	return fmt.Sprintf("%s%04x", criteriaVersionPrefix, hash.Sum32()&criteriaVersionMask)
}

func isCriteriaVersion(text string) bool {
	if len(text) != criteriaVersionLength || !strings.HasPrefix(text, criteriaVersionPrefix) {
		return false
	}

	_, err := strconv.ParseUint(text[len(criteriaVersionPrefix):], 16, 32)

	return err == nil
}

// showCriteriaPage lists the criteria with buttons to change them. The notice (HTML) tells what just happened.
func (ch *CommandHandler) showCriteriaPage(code string, chatID int64, notice string) {
	criteria := ch.effectiveCriteria(code)
	settings := ch.trackerSettings(code)
	version := criteriaVersion(criteria)

	var builder strings.Builder
	fmt.Fprintf(&builder, "<b>Notify criteria for %s</b>\n\n", code)

	if notice != "" {
		fmt.Fprintf(&builder, "%s\n\n", notice)
	}

	if len(criteria) == 0 {
		builder.WriteString("No criteria: the tracker never notifies.\n")
	}

	menu := &models.InlineKeyboardMarkup{}
	for i, criterion := range criteria {
		number := strconv.Itoa(i + 1)
		fmt.Fprintf(&builder, " %s. %s\n", number, helpers.FormatCriterion(criterion))
		menu.InlineKeyboard = append(menu.InlineKeyboard, []models.InlineKeyboardButton{
			helpers.InlineButton("Edit #"+number, "/criteria "+code+" "+criteriaEdit+" "+number+" "+version),
			helpers.InlineButton("Delete #"+number, "/criteria "+code+" "+criteriaDelete+" "+number+" "+version),
		})
	}

	if text, _, ok := ch.lastRecordedValue(code); ok {
		fmt.Fprintf(&builder, "\nLast recorded value: %s\n", text)
	}

	if settings.NotifyCriteria != nil {
		builder.WriteString("\nChanged from the config, which has:\n")
		configured := ch.config.GetTrackerData(code).NotifyCriteria
		if len(configured) == 0 {
			builder.WriteString(" - no criteria\n")
		}

		for _, criterion := range configured {
			fmt.Fprintf(&builder, " - %s\n", helpers.FormatCriterion(criterion))
		}
	}

	if len(criteria) < maxNotifyCriteria {
		menu.InlineKeyboard = append(menu.InlineKeyboard, []models.InlineKeyboardButton{helpers.InlineButton("Add criterion", "/criteria "+code+" "+criteriaAdd)})
	}

	if !settings.isEmpty() {
		menu.InlineKeyboard = append(menu.InlineKeyboard, []models.InlineKeyboardButton{helpers.InlineButton("Reset to config", "/reset "+code+" "+resetFromCriteriaParam)})
	}

	ch.handleCommandMessage(chatID, builder.String(), menu)
}

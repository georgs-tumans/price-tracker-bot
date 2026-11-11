package helpers

import (
	"fmt"
	"html"
	"strings"

	"pricetrackerbot/config"
)

func FormatNotificationCriteriaString(notifyCriteria []config.NotifyCriteria) string {
	if len(notifyCriteria) == 0 {
		return "Active notify criteria: none\n"
	}

	var builder strings.Builder
	builder.WriteString("Active notify criteria:\n")
	for _, criteria := range notifyCriteria {
		fmt.Fprintf(&builder, " - %s\n", FormatCriterion(criteria))
	}

	return builder.String()
}

// FormatCriterion returns a criterion as HTML, e.g. "tracked value &lt;= 45".
func FormatCriterion(criteria config.NotifyCriteria) string {
	return fmt.Sprintf("tracked value %s %s", html.EscapeString(criteria.Operator), html.EscapeString(criteria.Value))
}

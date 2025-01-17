package helpers

import (
	"fmt"
	"strings"

	"pricetrackerbot/config"
)

func FormatNotificationCriteriaString(notifyCriteria []config.NotifyCriteria) string {
	if len(notifyCriteria) > 0 {
		var builder strings.Builder
		builder.WriteString("Active notify criteria:\n")
		for _, criteria := range notifyCriteria {
			operatorEscaped := strings.ReplaceAll(strings.ReplaceAll(criteria.Operator, "<", "&lt;"), ">", "&gt;")
			builder.WriteString(fmt.Sprintf(" - tracked value %s %s\n", operatorEscaped, criteria.Value))
		}

		return builder.String()
	}

	return ""
}

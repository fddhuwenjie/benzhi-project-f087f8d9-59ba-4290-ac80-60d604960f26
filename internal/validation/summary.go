package validation

import (
	"broadcastdesk/internal/domain"
	"fmt"
	"strings"
)

func Summary(r domain.ValidationReport) string {
	if r.Passed {
		return "校验通过"
	}
	parts := []string{}
	for _, i := range r.Issues {
		parts = append(parts, fmt.Sprintf("%s:%s", i.RuleID, i.Message))
	}
	return strings.Join(parts, "；")
}

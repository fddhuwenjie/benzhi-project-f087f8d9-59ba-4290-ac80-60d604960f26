package validation

import (
	"broadcastdesk/internal/domain"
	"sort"
)

func IssueIDs(issues []domain.ValidationIssue) []string {
	out := make([]string, len(issues))
	for i, v := range issues {
		out[i] = v.RuleID
	}
	sort.Strings(out)
	return out
}

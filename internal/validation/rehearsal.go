package validation

import (
	"broadcastdesk/internal/domain"
	"fmt"
	"sort"
	"strings"
)

type RehearsalEvaluation struct {
	FieldErrors []domain.FieldError      `json:"field_errors"`
	Issues      []domain.ValidationIssue `json:"issues"`
	Statistics  domain.RehearsalStats    `json:"statistics"`
}

func EvaluateRehearsal(p *domain.BroadcastPackage, results []domain.SegmentResult) RehearsalEvaluation {
	e := RehearsalEvaluation{FieldErrors: []domain.FieldError{}, Issues: []domain.ValidationIssue{}}
	expected := map[string]domain.ScriptSegment{}
	for _, segment := range p.Segments {
		expected[segment.SegmentID] = segment
		e.Statistics.EstimatedTotalSeconds += segment.EstimatedSeconds
	}
	seen := map[string]int{}
	allowedAudibility := map[string]bool{"清晰": true, "不清晰": true}
	for n, result := range results {
		field := fmt.Sprintf("results[%d]", n)
		segment, known := expected[result.SegmentID]
		if !known {
			e.FieldErrors = append(e.FieldErrors, domain.FieldError{Field: field + ".segment_id", Message: "未知脚本段", SegmentIDs: []string{result.SegmentID}})
		} else {
			seen[result.SegmentID]++
			if seen[result.SegmentID] > 1 {
				e.FieldErrors = append(e.FieldErrors, domain.FieldError{Field: field + ".segment_id", Message: "脚本段重复出现", SegmentIDs: []string{result.SegmentID}})
			}
		}
		if result.ActualSeconds <= 0 {
			e.FieldErrors = append(e.FieldErrors, domain.FieldError{Field: field + ".actual_seconds", Message: "实际耗时必须为正", SegmentIDs: []string{result.SegmentID}})
		}
		if strings.TrimSpace(result.ReaderID) == "" {
			e.FieldErrors = append(e.FieldErrors, domain.FieldError{Field: field + ".reader_id", Message: "朗读人不能为空", SegmentIDs: []string{result.SegmentID}})
		}
		if strings.TrimSpace(result.Evidence) == "" {
			e.FieldErrors = append(e.FieldErrors, domain.FieldError{Field: field + ".evidence", Message: "证据摘要不能为空", SegmentIDs: []string{result.SegmentID}})
		}
		if !allowedAudibility[result.Audibility] {
			e.FieldErrors = append(e.FieldErrors, domain.FieldError{Field: field + ".audibility", Message: "听辨结论不在允许范围", SegmentIDs: []string{result.SegmentID}})
		}
		if !known {
			continue
		}
		e.Statistics.ActualTotalSeconds += result.ActualSeconds
		e.Statistics.Segments = append(e.Statistics.Segments, domain.SegmentTiming{SegmentID: result.SegmentID, EstimatedSeconds: segment.EstimatedSeconds, ActualSeconds: result.ActualSeconds, DeviationSeconds: result.ActualSeconds - segment.EstimatedSeconds})
		if result.Audibility != "清晰" {
			e.Issues = append(e.Issues, domain.ValidationIssue{RuleID: "AUDIBILITY", Message: fmt.Sprintf("脚本段%s听辨结论为%s", result.SegmentID, result.Audibility), SegmentIDs: []string{result.SegmentID}, Severity: "error"})
		}
		if strings.TrimSpace(result.Slip) != "" {
			e.Issues = append(e.Issues, domain.ValidationIssue{RuleID: "SLIP", Message: fmt.Sprintf("脚本段%s存在口误", result.SegmentID), SegmentIDs: []string{result.SegmentID}, Severity: "error"})
		}
	}
	for _, segment := range p.Segments {
		if seen[segment.SegmentID] == 0 {
			e.FieldErrors = append(e.FieldErrors, domain.FieldError{Field: "results", Message: "缺少冻结脚本段演练记录", SegmentIDs: []string{segment.SegmentID}})
		}
	}
	if len(e.FieldErrors) > 0 {
		return e
	}
	for i := range e.Statistics.Segments {
		if e.Statistics.ActualTotalSeconds > 0 {
			e.Statistics.Segments[i].TotalRatio = float64(e.Statistics.Segments[i].ActualSeconds) / float64(e.Statistics.ActualTotalSeconds)
		}
		if e.Statistics.Segments[i].DeviationSeconds > 0 {
			e.Statistics.OverrunContributors = append(e.Statistics.OverrunContributors, e.Statistics.Segments[i])
		}
	}
	sort.SliceStable(e.Statistics.OverrunContributors, func(i, j int) bool {
		if e.Statistics.OverrunContributors[i].DeviationSeconds == e.Statistics.OverrunContributors[j].DeviationSeconds {
			return e.Statistics.OverrunContributors[i].SegmentID < e.Statistics.OverrunContributors[j].SegmentID
		}
		return e.Statistics.OverrunContributors[i].DeviationSeconds > e.Statistics.OverrunContributors[j].DeviationSeconds
	})
	e.Statistics.DeviationSeconds = e.Statistics.ActualTotalSeconds - e.Statistics.EstimatedTotalSeconds
	if p.Baseline != nil {
		e.Statistics.RemainingSeconds = p.Baseline.MaxSeconds - e.Statistics.ActualTotalSeconds
		if e.Statistics.RemainingSeconds < 0 {
			e.Issues = append(e.Issues, domain.ValidationIssue{RuleID: "REHEARSAL_DURATION", Message: fmt.Sprintf("实际总时长%d秒超过上限%d秒", e.Statistics.ActualTotalSeconds, p.Baseline.MaxSeconds), Severity: "error"})
		}
	}
	return e
}
func RetestSet(issue domain.RemediationIssue, report domain.ValidationReport, runIssues []domain.ValidationIssue) []string {
	set := map[string]bool{}
	for _, id := range issue.AffectedSegmentIDs {
		set[id] = true
	}
	for _, i := range append(report.Issues, runIssues...) {
		for _, id := range i.SegmentIDs {
			if set[id] {
				set[id] = true
			}
		}
	}
	out := []string{}
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

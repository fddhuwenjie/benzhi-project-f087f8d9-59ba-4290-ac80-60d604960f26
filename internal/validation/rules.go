package validation

import (
	"broadcastdesk/internal/domain"
	"fmt"
	"strings"
	"time"
)

var ambiguous = []string{"尽快", "适当", "可能", "大概", "左右"}

func Validate(p *domain.BroadcastPackage) domain.ValidationReport {
	r := domain.ValidationReport{Passed: true, CheckedAt: time.Now()}
	if p.Baseline == nil {
		r.Passed = false
		r.Issues = append(r.Issues, domain.ValidationIssue{RuleID: "BASELINE", Message: "尚未冻结基线", Severity: "error"})
		return r
	}
	b := p.Baseline
	all := strings.ToLower(p.CanonicalScript())
	for _, phrase := range b.RequiredPhrases {
		if !strings.Contains(all, strings.ToLower(phrase)) {
			r.Passed = false
			r.Issues = append(r.Issues, domain.ValidationIssue{RuleID: "REQUIRED_PHRASE", Message: fmt.Sprintf("缺少必含提示语：%s", phrase), Severity: "error"})
		}
	}
	seen := map[string]string{}
	total := 0
	for _, s := range p.Segments {
		total += s.EstimatedSeconds
		for _, w := range ambiguous {
			if strings.Contains(s.Text, w) {
				r.Passed = false
				r.Issues = append(r.Issues, domain.ValidationIssue{RuleID: "AMBIGUOUS_WORD", Message: fmt.Sprintf("包含含混措辞：%s", w), SegmentIDs: []string{s.SegmentID}, Severity: "error"})
			}
		}
		if prev, ok := seen[s.Text]; ok {
			r.Passed = false
			r.Issues = append(r.Issues, domain.ValidationIssue{RuleID: "DUPLICATE_SEGMENT", Message: fmt.Sprintf("与脚本段%s重复", prev), SegmentIDs: []string{prev, s.SegmentID}, Severity: "error"})
		} else {
			seen[s.Text] = s.SegmentID
		}
		for _, k := range s.PronunciationKeys {
			if _, ok := b.Pronunciation[k]; !ok {
				r.Passed = false
				r.Issues = append(r.Issues, domain.ValidationIssue{RuleID: "PRONUNCIATION_COVERAGE", Message: fmt.Sprintf("发音词条未覆盖：%s", k), SegmentIDs: []string{s.SegmentID}, Severity: "error"})
			}
		}
	}
	if total > b.MaxSeconds {
		r.Passed = false
		r.Issues = append(r.Issues, domain.ValidationIssue{RuleID: "MAX_DURATION", Message: fmt.Sprintf("估算总时长%d秒超过上限%d秒", total, b.MaxSeconds), Severity: "error"})
	}
	for i := 1; i < len(p.Segments); i++ {
		if p.Segments[i].SpeakerRole == p.Segments[i-1].SpeakerRole {
			r.Passed = false
			r.Issues = append(r.Issues, domain.ValidationIssue{RuleID: "ROLE_HANDOFF", Message: fmt.Sprintf("相邻脚本段角色未交接：%s", p.Segments[i].SpeakerRole), SegmentIDs: []string{p.Segments[i-1].SegmentID, p.Segments[i].SegmentID}, Severity: "error"})
		}
	}
	return r
}

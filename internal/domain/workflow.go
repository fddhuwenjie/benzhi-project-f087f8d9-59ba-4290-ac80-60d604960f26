package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type FieldError struct {
	Field      string   `json:"field"`
	Message    string   `json:"message"`
	SegmentIDs []string `json:"segment_ids,omitempty"`
}

type BaselinePreview struct {
	Valid      bool         `json:"valid"`
	Errors     []FieldError `json:"errors"`
	Normalized Baseline     `json:"normalized"`
	Digest     string       `json:"digest"`
}

func NormalizeSegments(packageID string, segments []ScriptSegment) ([]ScriptSegment, error) {
	if len(segments) == 0 {
		return nil, errors.New("删除后脚本不能为空")
	}
	out := append([]ScriptSegment(nil), segments...)
	seen := map[string]bool{}
	for i := range out {
		out[i].SegmentID = strings.TrimSpace(out[i].SegmentID)
		out[i].SpeakerRole = strings.TrimSpace(out[i].SpeakerRole)
		out[i].Text = strings.TrimSpace(out[i].Text)
		out[i].PackageID = packageID
		out[i].Position = i + 1
		if out[i].SegmentID == "" {
			return nil, fmt.Errorf("第%d段的segment_id不能为空", i+1)
		}
		if seen[out[i].SegmentID] {
			return nil, fmt.Errorf("脚本段标识重复: %s", out[i].SegmentID)
		}
		seen[out[i].SegmentID] = true
		if out[i].SpeakerRole == "" || out[i].Text == "" {
			return nil, fmt.Errorf("脚本段%s的角色和文案不能为空", out[i].SegmentID)
		}
		if out[i].EstimatedSeconds <= 0 {
			return nil, fmt.Errorf("脚本段%s的估算时长必须为正", out[i].SegmentID)
		}
		keys := make([]string, 0, len(out[i].PronunciationKeys))
		for _, key := range out[i].PronunciationKeys {
			if key = strings.TrimSpace(key); key != "" {
				keys = append(keys, key)
			}
		}
		out[i].PronunciationKeys = keys
	}
	return out, nil
}

func PreviewBaseline(b Baseline, segments []ScriptSegment) BaselinePreview {
	n := Baseline{
		Scenario: strings.TrimSpace(b.Scenario), Audience: strings.TrimSpace(b.Audience),
		Channel: strings.TrimSpace(b.Channel), MaxSeconds: b.MaxSeconds,
		RequiredPhrases: []string{}, Pronunciation: map[string]string{},
	}
	p := BaselinePreview{Normalized: n, Errors: []FieldError{}}
	if n.Scenario == "" {
		p.Errors = append(p.Errors, FieldError{Field: "scenario", Message: "适用场景不能为空"})
	}
	if n.Audience == "" {
		p.Errors = append(p.Errors, FieldError{Field: "audience", Message: "目标听众不能为空"})
	}
	if n.Channel == "" {
		p.Errors = append(p.Errors, FieldError{Field: "channel", Message: "播出渠道不能为空"})
	}
	if n.MaxSeconds <= 0 {
		p.Errors = append(p.Errors, FieldError{Field: "max_seconds", Message: "最大时长必须为正"})
	}
	phrases := map[string]int{}
	for i, phrase := range b.RequiredPhrases {
		phrase = strings.TrimSpace(phrase)
		if phrase == "" {
			p.Errors = append(p.Errors, FieldError{Field: fmt.Sprintf("required_phrases[%d]", i), Message: "必含提示语不能为空"})
			continue
		}
		key := strings.ToLower(phrase)
		if first, exists := phrases[key]; exists {
			p.Errors = append(p.Errors, FieldError{Field: fmt.Sprintf("required_phrases[%d]", i), Message: fmt.Sprintf("与第%d条必含提示语重复", first+1)})
			continue
		}
		phrases[key] = i
		p.Normalized.RequiredPhrases = append(p.Normalized.RequiredPhrases, phrase)
	}
	normalizedKeys := map[string]string{}
	keys := SortedKeys(b.Pronunciation)
	for _, rawKey := range keys {
		key, value := strings.TrimSpace(rawKey), strings.TrimSpace(b.Pronunciation[rawKey])
		if key == "" || value == "" {
			p.Errors = append(p.Errors, FieldError{Field: "pronunciation." + rawKey, Message: "发音词条键和值均不能为空"})
			continue
		}
		canonical := strings.ToLower(key)
		if previous, exists := normalizedKeys[canonical]; exists && previous != rawKey {
			p.Errors = append(p.Errors, FieldError{Field: "pronunciation." + rawKey, Message: "规范化后词条键名冲突"})
			continue
		}
		normalizedKeys[canonical] = rawKey
		p.Normalized.Pronunciation[key] = value
	}
	for _, segment := range segments {
		for _, rawKey := range segment.PronunciationKeys {
			key := strings.TrimSpace(rawKey)
			found := false
			for defined := range p.Normalized.Pronunciation {
				if strings.EqualFold(defined, key) {
					found = true
					break
				}
			}
			if !found {
				p.Errors = append(p.Errors, FieldError{Field: "pronunciation." + key, Message: "脚本引用的发音词条未定义", SegmentIDs: []string{segment.SegmentID}})
			}
		}
	}
	p.Valid = len(p.Errors) == 0
	p.Digest = DigestJSON(p.Normalized)
	return p
}

func ValidationIssueKey(issue ValidationIssue) string {
	ids := append([]string(nil), issue.SegmentIDs...)
	sort.Strings(ids)
	return issue.RuleID + "|" + strings.Join(ids, ",")
}

type ValidationDiff struct {
	FromBatchID string            `json:"from_batch_id"`
	ToBatchID   string            `json:"to_batch_id"`
	Added       []ValidationIssue `json:"added"`
	Resolved    []ValidationIssue `json:"resolved"`
	Remaining   []ValidationIssue `json:"remaining"`
	Counts      map[string]int    `json:"counts"`
}

func DiffValidation(from, to ValidationBatch) ValidationDiff {
	d := ValidationDiff{FromBatchID: from.BatchID, ToBatchID: to.BatchID, Added: []ValidationIssue{}, Resolved: []ValidationIssue{}, Remaining: []ValidationIssue{}}
	a, b := map[string]ValidationIssue{}, map[string]ValidationIssue{}
	for _, issue := range from.Report.Issues {
		a[ValidationIssueKey(issue)] = issue
	}
	for _, issue := range to.Report.Issues {
		b[ValidationIssueKey(issue)] = issue
	}
	keys := make([]string, 0, len(a)+len(b))
	seen := map[string]bool{}
	for k := range a {
		keys = append(keys, k)
		seen[k] = true
	}
	for k := range b {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		old, was := a[k]
		current, is := b[k]
		switch {
		case was && is:
			d.Remaining = append(d.Remaining, current)
		case was:
			d.Resolved = append(d.Resolved, old)
		default:
			d.Added = append(d.Added, current)
		}
	}
	d.Counts = map[string]int{"added": len(d.Added), "resolved": len(d.Resolved), "remaining": len(d.Remaining)}
	return d
}

var taskStatuses = map[string]bool{"待分派": true, "处理中": true, "待复验": true, "已关闭": true}
var priorities = map[string]bool{"低": true, "中": true, "高": true, "紧急": true}

func (i *RemediationIssue) Assign(assignee, dueDate, priority, nextStatus string, now time.Time) error {
	if i.Status == "已关闭" {
		return errors.New("已关闭问题不能调整")
	}
	if !taskStatuses[nextStatus] {
		return errors.New("未知任务状态")
	}
	if priority != "" && !priorities[priority] {
		return errors.New("未知优先级")
	}
	if nextStatus == "处理中" && strings.TrimSpace(assignee) == "" {
		return errors.New("处理中问题必须设置负责人")
	}
	if nextStatus == "待复验" && (strings.TrimSpace(i.Cause) == "" || strings.TrimSpace(i.ChangeSummary) == "" || (len(i.AffectedSegmentIDs) == 0 && strings.TrimSpace(i.SourceRef) == "")) {
		return errors.New("原因、修订说明和受影响范围完整后才能转为待复验")
	}
	if i.Status == "待分派" && nextStatus == "已关闭" {
		return errors.New("待分派问题不能直接关闭")
	}
	if dueDate != "" {
		if _, err := time.Parse("2006-01-02", dueDate); err != nil {
			return errors.New("截止日期格式必须为YYYY-MM-DD")
		}
	}
	i.AssigneeID, i.DueDate, i.Priority, i.Status = strings.TrimSpace(assignee), dueDate, priority, nextStatus
	i.Overdue = dueDate != "" && dueDate < now.Format("2006-01-02") && nextStatus != "已关闭"
	return nil
}

const ReleaseManifestVersion = "broadcastdesk.release-manifest.v1"

type AuditAnchor struct {
	Sequence int    `json:"sequence"`
	Digest   string `json:"digest"`
}
type ReleaseManifest struct {
	FormatVersion       string            `json:"format_version"`
	PackageID           string            `json:"package_id"`
	CanonicalScript     string            `json:"canonical_script"`
	ScriptDigest        string            `json:"script_digest"`
	Pronunciation       map[string]string `json:"pronunciation"`
	PronunciationDigest string            `json:"pronunciation_digest"`
	Rehearsal           RehearsalRun      `json:"rehearsal"`
	RehearsalDigest     string            `json:"rehearsal_digest"`
	Signing             SigningSnapshot   `json:"signing"`
	SigningDigest       string            `json:"signing_digest"`
	AuditAnchor         AuditAnchor       `json:"audit_anchor"`
	AuditDigest         string            `json:"audit_digest"`
	TotalDigest         string            `json:"total_digest"`
}

func (m *ReleaseManifest) CalculateDigests() {
	m.ScriptDigest = DigestJSON(m.CanonicalScript)
	m.PronunciationDigest = DigestJSON(m.Pronunciation)
	m.RehearsalDigest = DigestJSON(m.Rehearsal)
	m.SigningDigest = DigestJSON(m.Signing)
	m.AuditDigest = DigestJSON(m.AuditAnchor)
	m.TotalDigest = DigestJSON(struct{ FormatVersion, PackageID, Script, Pronunciation, Rehearsal, Signing, Audit string }{m.FormatVersion, m.PackageID, m.ScriptDigest, m.PronunciationDigest, m.RehearsalDigest, m.SigningDigest, m.AuditDigest})
}

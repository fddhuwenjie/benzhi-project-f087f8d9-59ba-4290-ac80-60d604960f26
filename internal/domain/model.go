package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

type State string

const (
	StateDraft       State = "草拟"
	StateReview      State = "待校验"
	StateRehearsal   State = "待演练"
	StateRemediation State = "整改中"
	StateApproval    State = "待批准"
	StatePublished   State = "已发布"
	StateRejected    State = "已拒绝"
)

type Baseline struct {
	Scenario        string            `json:"scenario"`
	Audience        string            `json:"audience"`
	Channel         string            `json:"channel"`
	MaxSeconds      int               `json:"max_seconds"`
	RequiredPhrases []string          `json:"required_phrases"`
	Pronunciation   map[string]string `json:"pronunciation"`
}

type ScriptSegment struct {
	SegmentID         string   `json:"segment_id"`
	PackageID         string   `json:"package_id"`
	Position          int      `json:"position"`
	SpeakerRole       string   `json:"speaker_role"`
	Text              string   `json:"text"`
	PronunciationKeys []string `json:"pronunciation_keys"`
	EstimatedSeconds  int      `json:"estimated_seconds"`
	RevisionReason    string   `json:"revision_reason,omitempty"`
	WriterID          string   `json:"writer_id,omitempty"`
}

type BroadcastPackage struct {
	PackageID            string          `json:"package_id"`
	Title                string          `json:"title"`
	Scenario             string          `json:"scenario"`
	Audience             string          `json:"audience"`
	Channel              string          `json:"channel"`
	State                State           `json:"state"`
	Revision             int             `json:"revision"`
	BaselineDigest       string          `json:"baseline_digest"`
	Baseline             *Baseline       `json:"baseline,omitempty"`
	Segments             []ScriptSegment `json:"segments"`
	Writers              []string        `json:"writers"`
	RehearsalRecorderIDs []string        `json:"rehearsal_recorder_ids"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type ValidationIssue struct {
	RuleID     string   `json:"rule_id"`
	Message    string   `json:"message"`
	SegmentIDs []string `json:"segment_ids,omitempty"`
	Severity   string   `json:"severity"`
}

type ValidationReport struct {
	Passed    bool              `json:"passed"`
	Issues    []ValidationIssue `json:"issues"`
	CheckedAt time.Time         `json:"checked_at"`
}

type ValidationBatch struct {
	BatchID        string           `json:"batch_id"`
	PackageID      string           `json:"package_id"`
	ScriptRevision int              `json:"script_revision"`
	ScriptDigest   string           `json:"script_digest"`
	BaselineDigest string           `json:"baseline_digest"`
	Report         ValidationReport `json:"report"`
	CheckedAt      time.Time        `json:"checked_at"`
}

type SegmentResult struct {
	SegmentID     string `json:"segment_id"`
	ActualSeconds int    `json:"actual_seconds"`
	ReaderID      string `json:"reader_id"`
	Slip          string `json:"slip"`
	Audibility    string `json:"audibility"`
	Evidence      string `json:"evidence"`
}

type RehearsalRun struct {
	RunID          string          `json:"run_id"`
	PackageID      string          `json:"package_id"`
	ScriptRevision int             `json:"script_revision"`
	RecorderID     string          `json:"recorder_id"`
	StartedAt      time.Time       `json:"started_at"`
	SegmentResults []SegmentResult `json:"segment_results"`
	TotalSeconds   int             `json:"total_seconds"`
	EvidenceDigest string          `json:"evidence_digest"`
	Outcome        string          `json:"outcome"`
	ScriptDigest   string          `json:"script_digest"`
	Statistics     RehearsalStats  `json:"statistics"`
}

type SegmentTiming struct {
	SegmentID        string  `json:"segment_id"`
	EstimatedSeconds int     `json:"estimated_seconds"`
	ActualSeconds    int     `json:"actual_seconds"`
	DeviationSeconds int     `json:"deviation_seconds"`
	TotalRatio       float64 `json:"total_ratio"`
}

type RehearsalStats struct {
	EstimatedTotalSeconds int             `json:"estimated_total_seconds"`
	ActualTotalSeconds    int             `json:"actual_total_seconds"`
	DeviationSeconds      int             `json:"deviation_seconds"`
	RemainingSeconds      int             `json:"remaining_seconds"`
	Segments              []SegmentTiming `json:"segments"`
	OverrunContributors   []SegmentTiming `json:"overrun_contributors"`
}

type RemediationIssue struct {
	IssueID            string   `json:"issue_id"`
	PackageID          string   `json:"package_id"`
	SourceType         string   `json:"source_type"`
	SourceRef          string   `json:"source_ref"`
	AffectedSegmentIDs []string `json:"affected_segment_ids"`
	Cause              string   `json:"cause"`
	ChangeSummary      string   `json:"change_summary"`
	Status             string   `json:"status"`
	RetestResult       string   `json:"retest_result"`
	AssigneeID         string   `json:"assignee_id,omitempty"`
	DueDate            string   `json:"due_date,omitempty"`
	Priority           string   `json:"priority,omitempty"`
	Overdue            bool     `json:"overdue"`
}

type ReviewItem struct {
	ItemID   string `json:"item_id"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
}

type ReviewSnapshot struct {
	PackageID       string       `json:"package_id"`
	PackageRevision int          `json:"package_revision"`
	Items           []ReviewItem `json:"items"`
	Digest          string       `json:"digest"`
	CreatedAt       time.Time    `json:"created_at"`
}

type SigningSnapshot struct {
	ReviewSnapshot ReviewSnapshot `json:"review_snapshot"`
	ApproverID     string         `json:"approver_id"`
	Decision       string         `json:"decision"`
	Statement      string         `json:"statement"`
	SignedAt       time.Time      `json:"signed_at"`
	Digest         string         `json:"digest"`
}

type ReleaseBundle struct {
	BundleID              string            `json:"bundle_id"`
	PackageID             string            `json:"package_id"`
	PackageRevision       int               `json:"package_revision"`
	CanonicalScript       string            `json:"canonical_script"`
	PronunciationGlossary map[string]string `json:"pronunciation_glossary"`
	RehearsalSummary      string            `json:"rehearsal_summary"`
	ApproverID            string            `json:"approver_id"`
	ApprovalStatement     string            `json:"approval_statement"`
	IssuedAt              time.Time         `json:"issued_at"`
	SHA256Digest          string            `json:"sha256_digest"`
	SigningSnapshot       *SigningSnapshot  `json:"signing_snapshot,omitempty"`
}

type AuditEvent struct {
	Sequence       int       `json:"sequence"`
	At             time.Time `json:"at"`
	Action         string    `json:"action"`
	PackageID      string    `json:"package_id"`
	Revision       int       `json:"revision"`
	Digest         string    `json:"digest"`
	PreviousDigest string    `json:"previous_digest"`
}

func (p *BroadcastPackage) ValidateIdentity() error {
	if strings.TrimSpace(p.PackageID) == "" || strings.TrimSpace(p.Title) == "" {
		return errors.New("方案标识和标题不能为空")
	}
	if len(p.PackageID) > 128 {
		return errors.New("方案标识不能超过128个字符")
	}
	for _, r := range p.PackageID {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.') {
			return errors.New("方案标识只能包含字母、数字、连字符、下划线和点")
		}
	}
	if len(p.Segments) == 0 {
		return errors.New("至少需要一个脚本段")
	}
	for i, s := range p.Segments {
		if s.Position != i+1 {
			return fmt.Errorf("脚本段顺序必须连续: %s", s.SegmentID)
		}
		if strings.TrimSpace(s.Text) == "" || strings.TrimSpace(s.SpeakerRole) == "" {
			return errors.New("脚本段角色和文案不能为空")
		}
	}
	return nil
}

func (p *BroadcastPackage) FreezeBaseline(b Baseline) error {
	if p.State != StateDraft {
		return errors.New("当前状态不能冻结基线")
	}
	if b.MaxSeconds <= 0 || strings.TrimSpace(b.Scenario) == "" || strings.TrimSpace(b.Audience) == "" || strings.TrimSpace(b.Channel) == "" {
		return errors.New("基线字段不完整")
	}
	p.Baseline = &b
	p.Scenario = b.Scenario
	p.Audience = b.Audience
	p.Channel = b.Channel
	p.BaselineDigest = DigestJSON(b)
	p.State = StateReview
	p.Revision++
	return nil
}

func (p *BroadcastPackage) Transition(next State) error {
	if p.State == StatePublished || p.State == StateRejected {
		return errors.New("终态方案只读")
	}
	valid := map[State]map[State]bool{StateDraft: {StateReview: true}, StateReview: {StateRehearsal: true, StateRemediation: true}, StateRehearsal: {StateRemediation: true, StateApproval: true}, StateRemediation: {StateRehearsal: true, StateApproval: true}, StateApproval: {StatePublished: true, StateRejected: true}}
	if !valid[p.State][next] {
		return fmt.Errorf("不允许从%s转为%s", p.State, next)
	}
	p.State = next
	p.Revision++
	return nil
}

func (p *BroadcastPackage) CanonicalScript() string {
	segs := append([]ScriptSegment(nil), p.Segments...)
	sort.Slice(segs, func(i, j int) bool { return segs[i].Position < segs[j].Position })
	var b strings.Builder
	for _, s := range segs {
		fmt.Fprintf(&b, "%d|%s|%s|%d|%s\n", s.Position, s.SpeakerRole, s.Text, s.EstimatedSeconds, strings.Join(s.PronunciationKeys, ","))
	}
	return b.String()
}

func DigestJSON(v any) string {
	data, _ := json.Marshal(v)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (p *BroadcastPackage) IndependentApprover(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("批准人员不能为空")
	}
	for _, v := range append(append([]string{}, p.Writers...), p.RehearsalRecorderIDs...) {
		if v == id {
			return errors.New("批准人员不能参与编写或演练记录")
		}
	}
	return nil
}

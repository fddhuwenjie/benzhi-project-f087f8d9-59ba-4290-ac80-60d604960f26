package application

import (
	"broadcastdesk/internal/domain"
	"time"
)

type View struct {
	Package           *domain.BroadcastPackage  `json:"package"`
	Validation        *domain.ValidationReport  `json:"validation,omitempty"`
	Rehearsal         *domain.RehearsalRun      `json:"rehearsal,omitempty"`
	Issues            []domain.RemediationIssue `json:"issues"`
	Bundle            *domain.ReleaseBundle     `json:"bundle,omitempty"`
	Timeline          []domain.AuditEvent       `json:"timeline"`
	AllowedActions    []string                  `json:"allowed_actions"`
	ValidationBatches []domain.ValidationBatch  `json:"validation_batches"`
	ReviewSnapshot    *domain.ReviewSnapshot    `json:"review_snapshot,omitempty"`
	SigningSnapshot   *domain.SigningSnapshot   `json:"signing_snapshot,omitempty"`
}

type CreateInput struct {
	PackageID string                 `json:"package_id"`
	Title     string                 `json:"title"`
	WriterID  string                 `json:"writer_id"`
	Segments  []domain.ScriptSegment `json:"segments"`
}
type BaselineInput struct {
	ExpectedRevision int             `json:"expected_revision"`
	Baseline         domain.Baseline `json:"baseline"`
	PreviewDigest    string          `json:"preview_digest"`
}
type EditDraftInput struct {
	ExpectedRevision int                    `json:"expected_revision"`
	WriterID         string                 `json:"writer_id"`
	Segments         []domain.ScriptSegment `json:"segments"`
}
type RehearsalInput struct {
	ExpectedRevision int                    `json:"expected_revision"`
	RecorderID       string                 `json:"recorder_id"`
	Results          []domain.SegmentResult `json:"results"`
}
type ReviseInput struct {
	ExpectedRevision int                    `json:"expected_revision"`
	IssueID          string                 `json:"issue_id"`
	Cause            string                 `json:"cause"`
	ChangeSummary    string                 `json:"change_summary"`
	WriterID         string                 `json:"writer_id"`
	Segments         []domain.ScriptSegment `json:"segments"`
}
type ApproveInput struct {
	ExpectedRevision int      `json:"expected_revision"`
	ApproverID       string   `json:"approver_id"`
	Decision         string   `json:"decision"`
	Statement        string   `json:"statement"`
	ReviewDigest     string   `json:"review_digest"`
	ConfirmedItemIDs []string `json:"confirmed_item_ids"`
}

type TaskUpdate struct {
	IssueID    string `json:"issue_id"`
	AssigneeID string `json:"assignee_id"`
	DueDate    string `json:"due_date"`
	Priority   string `json:"priority"`
	Status     string `json:"status"`
}
type BatchTasksInput struct {
	ExpectedRevision int          `json:"expected_revision"`
	Updates          []TaskUpdate `json:"updates"`
}

type WorkItem struct {
	PackageID      string       `json:"package_id"`
	Title          string       `json:"title"`
	State          domain.State `json:"state"`
	Revision       int          `json:"revision"`
	OpenIssueCount int          `json:"open_issue_count"`
	LatestAction   string       `json:"latest_action"`
	NextAction     string       `json:"next_action"`
	ReadOnly       bool         `json:"read_only"`
	UpdatedAt      time.Time    `json:"updated_at"`
}
type Worklist struct {
	Items      []WorkItem `json:"items"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	Total      int        `json:"total"`
	TotalPages int        `json:"total_pages"`
}

type IssueFilter struct {
	AssigneeID string
	Status     string
	Priority   string
	Overdue    *bool
	Today      time.Time
}

type CommandMeta struct {
	RequestID   string
	Fingerprint string
}

type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *AppError) Error() string { return e.Message }
func conflict(msg string) error   { return &AppError{Code: "REVISION_CONFLICT", Message: msg} }
func invalid(msg string) error    { return &AppError{Code: "INVALID_COMMAND", Message: msg} }

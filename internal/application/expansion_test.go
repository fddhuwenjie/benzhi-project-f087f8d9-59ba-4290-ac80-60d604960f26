package application

import (
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/store"
	"broadcastdesk/internal/validation"
	"errors"
	"testing"
)

func newDraft(t *testing.T, id string, segments []domain.ScriptSegment) (*Service, *View) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := New(st)
	v, err := app.Create(CommandMeta{RequestID: "create-" + id, Fingerprint: "create"}, CreateInput{PackageID: id, Title: "应急方案 " + id, WriterID: "writer", Segments: segments})
	if err != nil {
		t.Fatal(err)
	}
	return app, v
}

func TestEditDraftNormalizesAndPreservesIDs(t *testing.T) {
	segments := []domain.ScriptSegment{{SegmentID: "a", Position: 1, SpeakerRole: "播音员", Text: "第一段", EstimatedSeconds: 3}, {SegmentID: "b", Position: 2, SpeakerRole: "引导员", Text: "第二段", EstimatedSeconds: 4}, {SegmentID: "c", Position: 3, SpeakerRole: "播音员", Text: "第三段", EstimatedSeconds: 5}}
	app, v := newDraft(t, "draft-edit", segments)
	oldRevision := v.Package.Revision
	v, err := app.EditDraft("draft-edit", CommandMeta{"edit-1", "payload-1"}, EditDraftInput{ExpectedRevision: oldRevision, WriterID: "writer", Segments: []domain.ScriptSegment{segments[2], segments[0]}})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Package.Segments) != 2 || v.Package.Segments[0].SegmentID != "c" || v.Package.Segments[1].SegmentID != "a" || v.Package.Segments[0].Position != 1 || v.Package.Segments[1].Position != 2 {
		t.Fatalf("段序或身份未保留: %#v", v.Package.Segments)
	}
	_, err = app.EditDraft("draft-edit", CommandMeta{"edit-2", "payload-2"}, EditDraftInput{ExpectedRevision: oldRevision, Segments: segments})
	var ae *AppError
	if !errors.As(err, &ae) || ae.Code != "REVISION_CONFLICT" {
		t.Fatalf("期望版本冲突，得到%v", err)
	}
}

func TestBaselinePreviewLocatesDuplicateAndMissingGlossary(t *testing.T) {
	app, v := newDraft(t, "baseline-errors", []domain.ScriptSegment{{SegmentID: "s1", Position: 1, SpeakerRole: "播音员", Text: "疏散", PronunciationKeys: []string{"疏散"}, EstimatedSeconds: 3}})
	p, err := app.PreviewBaseline("baseline-errors", v.Package.Revision, domain.Baseline{Scenario: "火警", Audience: "公众", Channel: "广播", MaxSeconds: 30, RequiredPhrases: []string{" 请撤离 ", "请撤离"}, Pronunciation: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Valid || len(p.Errors) != 2 {
		t.Fatalf("预检错误不完整: %#v", p.Errors)
	}
	if len(p.Errors[1].SegmentIDs) != 1 || p.Errors[1].SegmentIDs[0] != "s1" {
		t.Fatalf("缺失词条未定位脚本段: %#v", p.Errors)
	}
}

func TestRehearsalCoverageErrorsDoNotPersist(t *testing.T) {
	segments := []domain.ScriptSegment{{SegmentID: "s1", Position: 1, SpeakerRole: "播音员", Text: "请撤离", EstimatedSeconds: 3}, {SegmentID: "s2", Position: 2, SpeakerRole: "引导员", Text: "走出口", EstimatedSeconds: 4}}
	app, v := newDraft(t, "rehearsal-errors", segments)
	b := domain.Baseline{Scenario: "火警", Audience: "公众", Channel: "广播", MaxSeconds: 30, RequiredPhrases: []string{"请撤离"}, Pronunciation: map[string]string{}}
	p, _ := app.PreviewBaseline("rehearsal-errors", v.Package.Revision, b)
	v, err := app.Freeze("rehearsal-errors", CommandMeta{"freeze", "f"}, BaselineInput{ExpectedRevision: v.Package.Revision, Baseline: b, PreviewDigest: p.Digest})
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.Validate("rehearsal-errors", CommandMeta{"validate", "v"}, v.Package.Revision)
	if err != nil {
		t.Fatal(err)
	}
	before := v.Package.Revision
	_, err = app.Rehearse("rehearsal-errors", CommandMeta{"run", "r"}, RehearsalInput{ExpectedRevision: before, RecorderID: "recorder", Results: []domain.SegmentResult{{SegmentID: "s1", ActualSeconds: 3, ReaderID: "r", Audibility: "清晰", Evidence: "e"}, {SegmentID: "s1", ActualSeconds: 3, ReaderID: "r", Audibility: "清晰", Evidence: "e"}, {SegmentID: "unknown", ActualSeconds: 1, ReaderID: "r", Audibility: "清晰", Evidence: "e"}}})
	var ae *AppError
	if !errors.As(err, &ae) || ae.Code != "VALIDATION_ERROR" {
		t.Fatalf("期望演练完整性错误: %v", err)
	}
	details := ae.Details.([]domain.FieldError)
	kinds := map[string]bool{}
	for _, d := range details {
		kinds[d.Message] = true
	}
	for _, want := range []string{"脚本段重复出现", "未知脚本段", "缺少冻结脚本段演练记录"} {
		if !kinds[want] {
			t.Fatalf("缺少错误%s: %#v", want, details)
		}
	}
	after, _ := app.View("rehearsal-errors")
	if after.Package.Revision != before || after.Rehearsal != nil {
		t.Fatalf("无效演练改变了方案")
	}
}

func TestBatchTasksIsAllOrNothing(t *testing.T) {
	segments := []domain.ScriptSegment{{SegmentID: "s1", Position: 1, SpeakerRole: "播音员", Text: "尽快处理", EstimatedSeconds: 3}, {SegmentID: "s2", Position: 2, SpeakerRole: "播音员", Text: "尽快处理", EstimatedSeconds: 3}}
	app, v := newDraft(t, "tasks", segments)
	b := domain.Baseline{Scenario: "火警", Audience: "公众", Channel: "广播", MaxSeconds: 30, Pronunciation: map[string]string{}}
	p, _ := app.PreviewBaseline("tasks", v.Package.Revision, b)
	v, _ = app.Freeze("tasks", CommandMeta{"f", "f"}, BaselineInput{ExpectedRevision: v.Package.Revision, Baseline: b, PreviewDigest: p.Digest})
	v, _ = app.Validate("tasks", CommandMeta{"v", "v"}, v.Package.Revision)
	if len(v.Issues) < 2 {
		t.Fatalf("测试需要多个问题")
	}
	v.Issues[1].Status = "已关闭"
	if err := app.save(v, "准备已关闭任务"); err != nil {
		t.Fatal(err)
	}
	revision := v.Package.Revision
	eventsBefore, _ := app.store.Audit("tasks")
	_, err := app.BatchTasks("tasks", CommandMeta{"tasks-update", "payload"}, BatchTasksInput{ExpectedRevision: revision, Updates: []TaskUpdate{{IssueID: v.Issues[0].IssueID, AssigneeID: "owner", DueDate: "2099-01-01", Priority: "高", Status: "处理中"}, {IssueID: v.Issues[1].IssueID, AssigneeID: "owner", DueDate: "2099-01-01", Priority: "高", Status: "处理中"}}})
	if err == nil {
		t.Fatal("包含已关闭问题的批量更新应失败")
	}
	after, _ := app.View("tasks")
	eventsAfter, _ := app.store.Audit("tasks")
	if after.Issues[0].AssigneeID != "" || after.Package.Revision != revision || len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("批量失败产生了部分更新")
	}
}

func TestValidationDiffAndWorklistValidation(t *testing.T) {
	from := domain.ValidationBatch{BatchID: "a", Report: domain.ValidationReport{Issues: []domain.ValidationIssue{{RuleID: "REQUIRED_PHRASE"}, {RuleID: "AMBIGUOUS_WORD", SegmentIDs: []string{"s1"}}}}}
	to := domain.ValidationBatch{BatchID: "b", Report: domain.ValidationReport{Issues: []domain.ValidationIssue{{RuleID: "ROLE_HANDOFF", SegmentIDs: []string{"s2"}}}}}
	d := domain.DiffValidation(from, to)
	if d.Counts["resolved"] != 2 || d.Counts["added"] != 1 || d.Counts["remaining"] != 0 {
		t.Fatalf("差异分类错误: %#v", d)
	}
	app, _ := newDraft(t, "search", []domain.ScriptSegment{{SegmentID: "s", Position: 1, SpeakerRole: "播音员", Text: "正文", EstimatedSeconds: 1}})
	before, _ := app.store.Audit("")
	_, err := app.Worklist("", " ", "", 1, 20)
	if err == nil {
		t.Fatal("空白关键字应拒绝")
	}
	_, err = app.Worklist("", "", "未知", 1, 20)
	if err == nil {
		t.Fatal("未知状态应拒绝")
	}
	_, err = app.Worklist("", "", "", 1, 101)
	if err == nil {
		t.Fatal("越界分页应拒绝")
	}
	after, _ := app.store.Audit("")
	if len(before) != len(after) {
		t.Fatal("无效查询改变了审计记录")
	}
}

func TestStoredValidationBatchDiff(t *testing.T) {
	segments := []domain.ScriptSegment{{SegmentID: "s1", Position: 1, SpeakerRole: "播音员", Text: "请尽快撤离", EstimatedSeconds: 3}}
	app, v := newDraft(t, "batch-diff", segments)
	b := domain.Baseline{Scenario: "火警", Audience: "公众", Channel: "广播", MaxSeconds: 30, Pronunciation: map[string]string{}}
	p, _ := app.PreviewBaseline("batch-diff", v.Package.Revision, b)
	v, _ = app.Freeze("batch-diff", CommandMeta{"batch-freeze", "f"}, BaselineInput{ExpectedRevision: v.Package.Revision, Baseline: b, PreviewDigest: p.Digest})
	v, err := app.Validate("batch-diff", CommandMeta{"batch-first", "v1"}, v.Package.Revision)
	if err != nil {
		t.Fatal(err)
	}
	clean := []domain.ScriptSegment{{SegmentID: "s1", Position: 1, SpeakerRole: "播音员", Text: "请立即撤离", EstimatedSeconds: 3}}
	v, err = app.Revise("batch-diff", CommandMeta{"batch-revise", "r"}, ReviseInput{ExpectedRevision: v.Package.Revision, IssueID: v.Issues[0].IssueID, Cause: "措辞不明确", ChangeSummary: "改为明确指令", WriterID: "writer", Segments: clean})
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.Validate("batch-diff", CommandMeta{"batch-second", "v2"}, v.Package.Revision)
	if err != nil {
		t.Fatal(err)
	}
	batches, err := app.ValidationBatches("batch-diff")
	if err != nil || len(batches) != 2 {
		t.Fatalf("批次列表错误: %v %#v", err, batches)
	}
	diff, err := app.ValidationDiff("batch-diff", batches[0].BatchID, batches[1].BatchID)
	if err != nil || diff.Counts["resolved"] != 1 {
		t.Fatalf("存储批次差异错误: %v %#v", err, diff)
	}
}

func TestOverrunContributorOrdering(t *testing.T) {
	p := &domain.BroadcastPackage{Baseline: &domain.Baseline{MaxSeconds: 20}, Segments: []domain.ScriptSegment{{SegmentID: "a", EstimatedSeconds: 5}, {SegmentID: "b", EstimatedSeconds: 5}}}
	evaluation := validation.EvaluateRehearsal(p, []domain.SegmentResult{{SegmentID: "a", ActualSeconds: 10, ReaderID: "r", Audibility: "清晰", Evidence: "e"}, {SegmentID: "b", ActualSeconds: 20, ReaderID: "r", Audibility: "清晰", Evidence: "e"}})
	if evaluation.Statistics.OverrunContributors[0].SegmentID != "b" || evaluation.Statistics.RemainingSeconds != -10 {
		t.Fatalf("超时贡献排序或余量错误: %#v", evaluation.Statistics)
	}
}

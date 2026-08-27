package application

import (
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/store"
	"testing"
)

func TestPublishFlow(t *testing.T) {
	dir := t.TempDir()
	st, e := store.New(dir)
	if e != nil {
		t.Fatal(e)
	}
	app := New(st)
	segments := []domain.ScriptSegment{{SegmentID: "s1", Position: 1, SpeakerRole: "播音员", Text: "请保持冷静", EstimatedSeconds: 8}, {SegmentID: "s2", Position: 2, SpeakerRole: "引导员", Text: "请从安全出口撤离", EstimatedSeconds: 8}}
	v, e := app.Create(CommandMeta{"1", "a"}, CreateInput{PackageID: "p1", Title: "测试", WriterID: "writer", Segments: segments})
	if e != nil {
		t.Fatal(e)
	}
	baseline := domain.Baseline{Scenario: "火警", Audience: "公众", Channel: "广播", MaxSeconds: 30, RequiredPhrases: []string{"请保持冷静", "请从安全出口撤离"}, Pronunciation: map[string]string{}}
	preview, e := app.PreviewBaseline("p1", v.Package.Revision, baseline)
	if e != nil {
		t.Fatal(e)
	}
	v, e = app.Freeze("p1", CommandMeta{"2", "b"}, BaselineInput{ExpectedRevision: v.Package.Revision, Baseline: baseline, PreviewDigest: preview.Digest})
	if e != nil {
		t.Fatal(e)
	}
	v, e = app.Validate("p1", CommandMeta{"3", "c"}, v.Package.Revision)
	if e != nil || !v.Validation.Passed {
		t.Fatalf("validate: %v %#v", e, v.Validation)
	}
	v, e = app.Rehearse("p1", CommandMeta{"4", "d"}, RehearsalInput{ExpectedRevision: v.Package.Revision, RecorderID: "recorder", Results: []domain.SegmentResult{{SegmentID: "s1", ActualSeconds: 8, ReaderID: "reader", Audibility: "清晰", Evidence: "记录"}, {SegmentID: "s2", ActualSeconds: 8, ReaderID: "reader", Audibility: "清晰", Evidence: "记录"}}})
	if e != nil {
		t.Fatal(e)
	}
	review, e := app.Review("p1")
	if e != nil {
		t.Fatal(e)
	}
	v, e = app.Decide("p1", CommandMeta{"5", "e"}, ApproveInput{ExpectedRevision: v.Package.Revision, ApproverID: "approver", Decision: "approve", Statement: "同意", ReviewDigest: review.Digest, ConfirmedItemIDs: []string{"baseline", "script", "rehearsal", "issues", "retest"}})
	if e != nil || v.Bundle == nil {
		t.Fatalf("approve: %v", e)
	}
	ok, _, e := app.VerifyBundle("p1")
	if e != nil || !ok {
		t.Fatalf("verify: %v %v", ok, e)
	}
}

func TestRevisionConflict(t *testing.T) {
	st, _ := store.New(t.TempDir())
	app := New(st)
	_, e := app.Create(CommandMeta{"1", "a"}, CreateInput{PackageID: "p", Title: "x", WriterID: "w", Segments: []domain.ScriptSegment{{SegmentID: "s", Position: 1, SpeakerRole: "r", Text: "t", EstimatedSeconds: 1}}})
	if e != nil {
		t.Fatal(e)
	}
	_, e = app.Freeze("p", CommandMeta{"2", "b"}, BaselineInput{ExpectedRevision: 99, Baseline: domain.Baseline{Scenario: "s", Audience: "a", Channel: "c", MaxSeconds: 1}})
	if e == nil {
		t.Fatal("expected conflict")
	}
}

package verifymissingstoredbundle

import (
	"broadcastdesk/internal/application"
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyRejectsMissingPersistedBundle(t *testing.T) {
	root := t.TempDir()
	st, err := store.New(root)
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	segments := []domain.ScriptSegment{{SegmentID: "s1", SpeakerRole: "播音员", Text: "请撤离", EstimatedSeconds: 5}}
	v, err := app.Create(application.CommandMeta{RequestID: "create", Fingerprint: "create"}, application.CreateInput{PackageID: "bundle", Title: "测试", WriterID: "writer", Segments: segments})
	if err != nil {
		t.Fatal(err)
	}
	baseline := domain.Baseline{Scenario: "火警", Audience: "公众", Channel: "广播", MaxSeconds: 30, RequiredPhrases: []string{"请撤离"}, Pronunciation: map[string]string{}}
	preview, err := app.PreviewBaseline("bundle", v.Package.Revision, baseline)
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.Freeze("bundle", application.CommandMeta{RequestID: "freeze", Fingerprint: "freeze"}, application.BaselineInput{ExpectedRevision: v.Package.Revision, Baseline: baseline, PreviewDigest: preview.Digest})
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.Validate("bundle", application.CommandMeta{RequestID: "validate", Fingerprint: "validate"}, v.Package.Revision)
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.Rehearse("bundle", application.CommandMeta{RequestID: "rehearse", Fingerprint: "rehearse"}, application.RehearsalInput{ExpectedRevision: v.Package.Revision, RecorderID: "recorder", Results: []domain.SegmentResult{{SegmentID: "s1", ActualSeconds: 5, ReaderID: "reader", Audibility: "清晰", Evidence: "记录"}}})
	if err != nil {
		t.Fatal(err)
	}
	review, err := app.Review("bundle")
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.Decide("bundle", application.CommandMeta{RequestID: "approve", Fingerprint: "approve"}, application.ApproveInput{ExpectedRevision: v.Package.Revision, ApproverID: "approver", Decision: "approve", Statement: "同意", ReviewDigest: review.Digest, ConfirmedItemIDs: []string{"baseline", "script", "rehearsal", "issues", "retest"}})
	if err != nil || v.Bundle == nil {
		t.Fatalf("前置条件失败: 发布未完成: view=%#v err=%v", v, err)
	}
	if err = os.Remove(filepath.Join(root, "bundles", "bundle.json")); err != nil {
		t.Fatal(err)
	}
	valid, _, err := app.VerifyBundle("bundle")
	if err == nil && valid {
		t.Fatal("持久化发布包被移除后VerifyBundle仍报告有效")
	}
}

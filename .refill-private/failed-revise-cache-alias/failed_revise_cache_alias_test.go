package failed_revise_cache_alias_test

import (
	"broadcastdesk/internal/application"
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/store"
	"testing"
)

func TestFailedReviseDoesNotLeakIntoCachedView(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	view, err := app.Create(
		application.CommandMeta{RequestID: "create-cache-alias", Fingerprint: "create"},
		application.CreateInput{
			PackageID: "cache-alias",
			Title:     "缓存隔离复现",
			WriterID:  "writer",
			Segments: []domain.ScriptSegment{{
				SegmentID: "segment-1", SpeakerRole: "播音员", Text: "请尽快撤离", EstimatedSeconds: 5,
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	baseline := domain.Baseline{
		Scenario: "火警", Audience: "现场公众", Channel: "场馆广播", MaxSeconds: 30,
		Pronunciation: map[string]string{},
	}
	preview, err := app.PreviewBaseline("cache-alias", view.Package.Revision, baseline)
	if err != nil {
		t.Fatal(err)
	}
	view, err = app.Freeze(
		"cache-alias",
		application.CommandMeta{RequestID: "freeze-cache-alias", Fingerprint: "freeze"},
		application.BaselineInput{ExpectedRevision: view.Package.Revision, Baseline: baseline, PreviewDigest: preview.Digest},
	)
	if err != nil {
		t.Fatal(err)
	}
	view, err = app.Validate(
		"cache-alias",
		application.CommandMeta{RequestID: "validate-cache-alias", Fingerprint: "validate"},
		view.Package.Revision,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Issues) == 0 {
		t.Fatal("复现前提不成立：含混措辞没有生成整改问题")
	}

	_, err = app.Revise(
		"cache-alias",
		application.CommandMeta{RequestID: "revise-cache-alias", Fingerprint: "revise"},
		application.ReviseInput{
			ExpectedRevision: view.Package.Revision,
			IssueID:          view.Issues[0].IssueID,
			Cause:            "准备写入但不应提交的原因",
			ChangeSummary:    "准备写入但不应提交的说明",
			WriterID:         "writer",
			Segments: []domain.ScriptSegment{{
				SegmentID: "segment-1", SpeakerRole: "", Text: "请立即撤离", EstimatedSeconds: 5,
			}},
		},
	)
	if err == nil {
		t.Fatal("复现前提不成立：无效脚本修订应失败")
	}

	live, err := app.View("cache-alias")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := application.New(st).View("cache-alias")
	if err != nil {
		t.Fatal(err)
	}
	if live.Issues[0].Cause != restarted.Issues[0].Cause || live.Issues[0].ChangeSummary != restarted.Issues[0].ChangeSummary {
		t.Fatalf("TestFailedReviseDoesNotLeakIntoCachedView: 失败修订污染当前进程读模型，live=%q/%q restarted=%q/%q",
			live.Issues[0].Cause, live.Issues[0].ChangeSummary,
			restarted.Issues[0].Cause, restarted.Issues[0].ChangeSummary)
	}
}

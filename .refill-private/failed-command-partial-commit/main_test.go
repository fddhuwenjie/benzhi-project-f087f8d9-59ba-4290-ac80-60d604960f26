package failedcommandpartialcommit

import (
	"broadcastdesk/internal/application"
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestFailedCommandDoesNotPartiallyCommitManifest(t *testing.T) {
	root := t.TempDir()
	st, err := store.New(root)
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	audit := filepath.Join(root, "audit.jsonl")
	if err = os.Mkdir(audit, 0755); err != nil {
		t.Fatal(err)
	}
	segments := []domain.ScriptSegment{{SegmentID: "s1", SpeakerRole: "播音员", Text: "不应提交", EstimatedSeconds: 7}}
	if _, err = app.Create(application.CommandMeta{RequestID: "create", Fingerprint: "create"}, application.CreateInput{PackageID: "partial", Title: "测试", WriterID: "writer", Segments: segments}); err == nil {
		t.Fatal("预期注入的持久化失败")
	}
	if err = os.Remove(audit); err != nil {
		t.Fatal(err)
	}
	view, viewErr := app.View("partial")
	if viewErr == nil {
		t.Fatalf("失败命令的变更变为可见状态: %#v", view.Package)
	}
}

package audittailsilentlydropped

import (
	"broadcastdesk/internal/application"
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestMalformedAuditTailIsReportedEverywhere(t *testing.T) {
	root := t.TempDir()
	st, err := store.New(root)
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	segments := []domain.ScriptSegment{{SegmentID: "s1", SpeakerRole: "播音员", Text: "正文", EstimatedSeconds: 5}}
	if _, err = app.Create(application.CommandMeta{RequestID: "create", Fingerprint: "create"}, application.CreateInput{PackageID: "audit", Title: "测试", WriterID: "writer", Segments: segments}); err != nil {
		t.Fatal(err)
	}
	auditPath := filepath.Join(root, "audit.jsonl")
	f, err := os.OpenFile(auditPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("{malformed-tail}\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = st.Verify(); err == nil {
		t.Fatal("Verify静默丢弃了格式错误的审计尾记录")
	}
	if _, err = app.View("audit"); err == nil {
		t.Fatal("应用视图静默丢弃了格式错误的审计尾记录")
	}
}

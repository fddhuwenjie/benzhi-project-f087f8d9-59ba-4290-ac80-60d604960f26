package verifyomitscurrentview

import (
	"broadcastdesk/internal/application"
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyRejectsBrokenCurrentView(t *testing.T) {
	root := t.TempDir()
	st, err := store.New(root)
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	segments := []domain.ScriptSegment{{SegmentID: "s1", SpeakerRole: "播音员", Text: "正文", EstimatedSeconds: 5}}
	if _, err = app.Create(application.CommandMeta{RequestID: "create", Fingerprint: "create"}, application.CreateInput{PackageID: "broken-view", Title: "测试", WriterID: "writer", Segments: segments}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "views", "broken-view.json"), []byte("missing-object-digest"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = st.Verify(); err == nil {
		t.Fatal("Verify接受了会导致应用不可读的损坏当前视图")
	}
}

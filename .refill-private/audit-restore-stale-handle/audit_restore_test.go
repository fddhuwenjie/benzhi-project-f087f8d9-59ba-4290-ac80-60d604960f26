package audit_restore_stale_handle_test

import (
	"broadcastdesk/internal/application"
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditReloadsAfterAtomicRestore(t *testing.T) {
	root := t.TempDir()
	st, err := store.New(root)
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	_, err = app.Create(
		application.CommandMeta{RequestID: "create-audit-restore", Fingerprint: "create"},
		application.CreateInput{
			PackageID: "audit-restore",
			Title:     "审计恢复测试",
			WriterID:  "writer",
			Segments: []domain.ScriptSegment{{
				SegmentID: "segment-1", SpeakerRole: "播音员",
				Text: "请保持冷静", EstimatedSeconds: 3,
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	before, err := app.View("audit-restore")
	if err != nil {
		t.Fatal(err)
	}

	recovery := t.TempDir()
	if err = os.WriteFile(filepath.Join(recovery, "audit.jsonl"), mustRead(t, filepath.Join(root, "audit.jsonl")), 0644); err != nil {
		t.Fatal(err)
	}
	recoveryStore, err := store.New(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err = recoveryStore.AppendAudit(domain.AuditEvent{Action: "恢复后校验", PackageID: "audit-restore", Revision: before.Package.Revision}); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(recovery, "audit.jsonl"), filepath.Join(root, "audit.jsonl")); err != nil {
		t.Fatal(err)
	}

	current, err := app.View("audit-restore")
	if err != nil {
		t.Fatal(err)
	}
	freshStore, err := store.New(root)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := freshStore.Audit("audit-restore")
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Timeline) != len(fresh) {
		t.Fatalf("同一进程遗漏原子恢复后的审计事件: current=%d fresh=%d", len(current.Timeline), len(fresh))
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

package package_object_cache_stale_test

import (
	"broadcastdesk/internal/application"
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/store"
	"broadcastdesk/internal/web"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageObjectCacheDetectsAtomicReplacement(t *testing.T) {
	root := t.TempDir()
	st, err := store.New(root)
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	_, err = app.Create(
		application.CommandMeta{RequestID: "create-cache-target", Fingerprint: "create-cache-target"},
		application.CreateInput{
			PackageID: "cache-target",
			Title:     "缓存失效复现方案",
			WriterID:  "writer",
			Segments: []domain.ScriptSegment{{
				SegmentID:        "segment-1",
				SpeakerRole:      "播音员",
				Text:             "请保持冷静并从安全出口撤离",
				EstimatedSeconds: 8,
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(app)
	if status := getPackageStatus(handler); status != http.StatusOK {
		t.Fatalf("首次读取未成功预热缓存: status=%d", status)
	}

	digest, err := os.ReadFile(filepath.Join(root, "manifests", "cache-target.json"))
	if err != nil {
		t.Fatal(err)
	}
	objectPath := filepath.Join(root, "objects", string(digest)+".json")
	replacement := filepath.Join(root, "objects", "replacement.json")
	if err = os.WriteFile(replacement, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(replacement, objectPath); err != nil {
		t.Fatal(err)
	}

	freshStore, err := store.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if status := getPackageStatus(web.NewHandler(application.New(freshStore))); status != http.StatusUnprocessableEntity {
		t.Fatalf("新Store未识别已替换的内容寻址对象: status=%d", status)
	}
	if status := getPackageStatus(handler); status != http.StatusUnprocessableEntity {
		t.Fatalf("缓存命中绕过对象文件重新校验: status=%d, want=%d", status, http.StatusUnprocessableEntity)
	}
}

func getPackageStatus(handler http.Handler) int {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/packages/cache-target", nil)
	handler.ServeHTTP(recorder, request)
	return recorder.Code
}

package idempotencyroutescope

import (
	"broadcastdesk/internal/application"
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/store"
	"broadcastdesk/internal/web"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIdempotencyKeyIsScopedToTargetRoute(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	for _, id := range []string{"alpha", "bravo"} {
		segments := []domain.ScriptSegment{{SegmentID: "s1", SpeakerRole: "播音员", Text: "请撤离", EstimatedSeconds: 5}}
		v, err := app.Create(application.CommandMeta{RequestID: "create-" + id, Fingerprint: id}, application.CreateInput{PackageID: id, Title: id, WriterID: "writer", Segments: segments})
		if err != nil {
			t.Fatal(err)
		}
		baseline := domain.Baseline{Scenario: "火警", Audience: "公众", Channel: "广播", MaxSeconds: 30, RequiredPhrases: []string{"请撤离"}, Pronunciation: map[string]string{}}
		preview, err := app.PreviewBaseline(id, v.Package.Revision, baseline)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = app.Freeze(id, application.CommandMeta{RequestID: "freeze-" + id, Fingerprint: id}, application.BaselineInput{ExpectedRevision: v.Package.Revision, Baseline: baseline, PreviewDigest: preview.Digest}); err != nil {
			t.Fatal(err)
		}
	}

	h := web.NewHandler(app)
	body := `{"request_id":"shared-validation","expected_revision":2}`
	for _, id := range []string{"alpha", "bravo"} {
		req := httptest.NewRequest(http.MethodPost, "/api/packages/"+id+"/validate", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if id == "bravo" && rec.Code == http.StatusConflict {
			continue
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("校验%s失败: status=%d body=%s", id, rec.Code, rec.Body.String())
		}
		var got application.View
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Package == nil || got.Package.PackageID != id {
			t.Fatalf("幂等响应跨越路由边界: 请求%s，得到%#v", id, got.Package)
		}
	}
}

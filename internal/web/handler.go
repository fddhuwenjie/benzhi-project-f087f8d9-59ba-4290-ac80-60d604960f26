package web

import (
	"broadcastdesk/internal/application"
	"broadcastdesk/internal/domain"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	app *application.Service
	mux *http.ServeMux
}

func NewHandler(app *application.Service) *Handler {
	h := &Handler{app: app, mux: http.NewServeMux()}
	h.routes()
	return h
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }
func (h *Handler) routes() {
	static, _ := fs.Sub(assets, "assets")
	h.mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(static))))
	h.mux.HandleFunc("/", h.Index)
	h.mux.HandleFunc("POST /api/packages", h.CreatePackage)
	h.mux.HandleFunc("GET /api/packages", h.ListPackages)
	h.mux.HandleFunc("GET /api/packages/{id}", h.GetPackage)
	h.mux.HandleFunc("POST /api/packages/{id}/{action}", h.PackageAction)
	h.mux.HandleFunc("GET /api/packages/{id}/verify", h.VerifyBundle)
	h.mux.HandleFunc("GET /api/packages/{id}/validation-batches", h.ValidationBatches)
	h.mux.HandleFunc("GET /api/packages/{id}/validation-batches/{batch}", h.ValidationBatch)
	h.mux.HandleFunc("GET /api/packages/{id}/validation-diff", h.ValidationDiff)
	h.mux.HandleFunc("GET /api/packages/{id}/issues", h.Issues)
	h.mux.HandleFunc("GET /api/packages/{id}/review", h.Review)
	h.mux.HandleFunc("GET /api/packages/{id}/release-manifest", h.ReleaseManifest)
}
func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	b, _ := assets.ReadFile("assets/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}

type createRequest struct {
	RequestID string                 `json:"request_id"`
	PackageID string                 `json:"package_id"`
	Title     string                 `json:"title"`
	WriterID  string                 `json:"writer_id"`
	Segments  []domain.ScriptSegment `json:"segments"`
}

func (h *Handler) CreatePackage(w http.ResponseWriter, r *http.Request) {
	var q createRequest
	b, e := decode(r, &q)
	if e != nil {
		writeError(w, e)
		return
	}
	v, e := h.app.Create(meta(q.RequestID, b), application.CreateInput{PackageID: q.PackageID, Title: q.Title, WriterID: q.WriterID, Segments: q.Segments})
	writeResult(w, v, e)
}
func (h *Handler) GetPackage(w http.ResponseWriter, r *http.Request) {
	v, e := h.app.View(r.PathValue("id"))
	writeResult(w, v, e)
}
func (h *Handler) ListPackages(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("keyword") && strings.TrimSpace(r.URL.Query().Get("keyword")) == "" {
		writeError(w, &application.AppError{Code: "INVALID_COMMAND", Message: "标题关键字不能为空白"})
		return
	}
	page, err := positiveInt(r.URL.Query().Get("page"), 1)
	if err != nil {
		writeError(w, err)
		return
	}
	size, err := positiveInt(r.URL.Query().Get("page_size"), 20)
	if err != nil {
		writeError(w, err)
		return
	}
	v, err := h.app.Worklist(r.URL.Query().Get("package_id"), r.URL.Query().Get("keyword"), r.URL.Query().Get("state"), page, size)
	writeResult(w, v, err)
}
func positiveInt(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, &application.AppError{Code: "INVALID_COMMAND", Message: "分页参数必须为整数"}
	}
	if n > 100000 {
		return 0, &application.AppError{Code: "INVALID_COMMAND", Message: "分页参数超出上限"}
	}
	return n, nil
}

type expectedRequest struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int    `json:"expected_revision"`
}
type freezeRequest struct {
	RequestID        string          `json:"request_id"`
	ExpectedRevision int             `json:"expected_revision"`
	Baseline         domain.Baseline `json:"baseline"`
	PreviewDigest    string          `json:"preview_digest"`
}
type editDraftRequest struct {
	RequestID        string                 `json:"request_id"`
	ExpectedRevision int                    `json:"expected_revision"`
	WriterID         string                 `json:"writer_id"`
	Segments         []domain.ScriptSegment `json:"segments"`
}
type rehearsalRequest struct {
	RequestID        string                 `json:"request_id"`
	ExpectedRevision int                    `json:"expected_revision"`
	RecorderID       string                 `json:"recorder_id"`
	Results          []domain.SegmentResult `json:"results"`
}
type reviseRequest struct {
	RequestID        string                 `json:"request_id"`
	ExpectedRevision int                    `json:"expected_revision"`
	IssueID          string                 `json:"issue_id"`
	Cause            string                 `json:"cause"`
	ChangeSummary    string                 `json:"change_summary"`
	WriterID         string                 `json:"writer_id"`
	Segments         []domain.ScriptSegment `json:"segments"`
}
type decisionRequest struct {
	RequestID        string   `json:"request_id"`
	ExpectedRevision int      `json:"expected_revision"`
	ApproverID       string   `json:"approver_id"`
	Decision         string   `json:"decision"`
	Statement        string   `json:"statement"`
	ReviewDigest     string   `json:"review_digest"`
	ConfirmedItemIDs []string `json:"confirmed_item_ids"`
}
type taskRequest struct {
	RequestID        string                   `json:"request_id"`
	ExpectedRevision int                      `json:"expected_revision"`
	Updates          []application.TaskUpdate `json:"updates"`
}

func meta(id string, b []byte) application.CommandMeta {
	return application.CommandMeta{RequestID: id, Fingerprint: domain.DigestJSON(string(b))}
}
func (h *Handler) PackageAction(w http.ResponseWriter, r *http.Request) {
	id, a := r.PathValue("id"), r.PathValue("action")
	var v *application.View
	var e error
	switch a {
	case "edit-draft":
		var q editDraftRequest
		var b []byte
		b, e = decode(r, &q)
		if e == nil {
			v, e = h.app.EditDraft(id, meta(q.RequestID, b), application.EditDraftInput{ExpectedRevision: q.ExpectedRevision, WriterID: q.WriterID, Segments: q.Segments})
		}
	case "baseline-preview":
		var q freezeRequest
		_, e = decode(r, &q)
		if e == nil {
			var preview domain.BaselinePreview
			preview, e = h.app.PreviewBaseline(id, q.ExpectedRevision, q.Baseline)
			writeResult(w, preview, e)
			return
		}
	case "freeze":
		var q freezeRequest
		var b []byte
		b, e = decode(r, &q)
		if e == nil {
			v, e = h.app.Freeze(id, meta(q.RequestID, b), application.BaselineInput{ExpectedRevision: q.ExpectedRevision, Baseline: q.Baseline, PreviewDigest: q.PreviewDigest})
		}
	case "validate":
		var q expectedRequest
		var b []byte
		b, e = decode(r, &q)
		if e == nil {
			v, e = h.app.Validate(id, meta(q.RequestID, b), q.ExpectedRevision)
		}
	case "rehearse":
		var q rehearsalRequest
		var b []byte
		b, e = decode(r, &q)
		if e == nil {
			v, e = h.app.Rehearse(id, meta(q.RequestID, b), application.RehearsalInput{ExpectedRevision: q.ExpectedRevision, RecorderID: q.RecorderID, Results: q.Results})
		}
	case "revise":
		var q reviseRequest
		var b []byte
		b, e = decode(r, &q)
		if e == nil {
			v, e = h.app.Revise(id, meta(q.RequestID, b), application.ReviseInput{ExpectedRevision: q.ExpectedRevision, IssueID: q.IssueID, Cause: q.Cause, ChangeSummary: q.ChangeSummary, WriterID: q.WriterID, Segments: q.Segments})
		}
	case "retest":
		var q expectedRequest
		var b []byte
		b, e = decode(r, &q)
		if e == nil {
			v, e = h.app.Retest(id, meta(q.RequestID, b), q.ExpectedRevision)
		}
	case "decision":
		var q decisionRequest
		var b []byte
		b, e = decode(r, &q)
		if e == nil {
			v, e = h.app.Decide(id, meta(q.RequestID, b), application.ApproveInput{ExpectedRevision: q.ExpectedRevision, ApproverID: q.ApproverID, Decision: q.Decision, Statement: q.Statement, ReviewDigest: q.ReviewDigest, ConfirmedItemIDs: q.ConfirmedItemIDs})
		}
	case "tasks":
		var q taskRequest
		var b []byte
		b, e = decode(r, &q)
		if e == nil {
			v, e = h.app.BatchTasks(id, meta(q.RequestID, b), application.BatchTasksInput{ExpectedRevision: q.ExpectedRevision, Updates: q.Updates})
		}
	default:
		e = &application.AppError{Code: "NOT_FOUND", Message: "操作不存在"}
	}
	writeResult(w, v, e)
}
func (h *Handler) ValidationBatches(w http.ResponseWriter, r *http.Request) {
	v, e := h.app.ValidationBatches(r.PathValue("id"))
	writeResult(w, v, e)
}
func (h *Handler) ValidationBatch(w http.ResponseWriter, r *http.Request) {
	v, e := h.app.ValidationBatch(r.PathValue("id"), r.PathValue("batch"))
	writeResult(w, v, e)
}
func (h *Handler) ValidationDiff(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v, e := h.app.ValidationDiff(r.PathValue("id"), q.Get("from"), q.Get("to"))
	writeResult(w, v, e)
}
func (h *Handler) Issues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := application.IssueFilter{AssigneeID: q.Get("assignee_id"), Status: q.Get("status"), Priority: q.Get("priority"), Today: time.Now()}
	if raw := q.Get("overdue"); raw != "" {
		if raw != "true" && raw != "false" {
			writeError(w, &application.AppError{Code: "INVALID_COMMAND", Message: "overdue必须为true或false"})
			return
		}
		value := raw == "true"
		filter.Overdue = &value
	}
	v, e := h.app.Issues(r.PathValue("id"), filter)
	writeResult(w, v, e)
}
func (h *Handler) Review(w http.ResponseWriter, r *http.Request) {
	v, e := h.app.Review(r.PathValue("id"))
	writeResult(w, v, e)
}
func (h *Handler) ReleaseManifest(w http.ResponseWriter, r *http.Request) {
	v, e := h.app.ReleaseManifest(r.PathValue("id"))
	if e == nil {
		w.Header().Set("Content-Disposition", "attachment; filename=release-manifest.json")
	}
	writeResult(w, v, e)
}
func (h *Handler) VerifyBundle(w http.ResponseWriter, r *http.Request) {
	ok, d, e := h.app.VerifyBundle(r.PathValue("id"))
	if e != nil {
		writeError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"valid": ok, "digest": d})
}
func decode(r *http.Request, v any) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 1<<20 {
		return nil, errors.New("请求正文超过限制")
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	return b, d.Decode(v)
}
func writeResult(w http.ResponseWriter, v any, e error) {
	if e != nil {
		writeError(w, e)
		return
	}
	writeJSON(w, 200, v)
}
func writeError(w http.ResponseWriter, e error) {
	code := "BAD_REQUEST"
	status := 400
	var ae *application.AppError
	if errors.As(e, &ae) {
		code = ae.Code
		if code == "NOT_FOUND" {
			status = 404
		} else if code == "REVISION_CONFLICT" || code == "IDEMPOTENCY_CONFLICT" {
			status = 409
		} else if code == "INTEGRITY_ERROR" {
			status = 422
		}
		writeJSON(w, status, ae)
		return
	}
	writeJSON(w, status, map[string]string{"code": code, "message": e.Error()})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

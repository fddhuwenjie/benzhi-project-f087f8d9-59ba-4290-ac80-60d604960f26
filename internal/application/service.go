package application

import (
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/store"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Service struct {
	store *store.Store
	locks *packageLocks
}

func New(s *store.Store) *Service { return &Service{store: s, locks: newPackageLocks()} }

func (s *Service) load(id string) (*View, error) {
	var v View
	if err := s.store.LoadJSON("views", id, &v); err != nil {
		if os.IsNotExist(err) {
			return nil, &AppError{Code: "NOT_FOUND", Message: "方案不存在"}
		}
		return nil, err
	}
	manifest, err := s.store.LoadPackage(id)
	if err != nil {
		return nil, &AppError{Code: "INTEGRITY_ERROR", Message: "方案manifest损坏: " + err.Error()}
	}
	if v.Package == nil || domain.DigestJSON(v.Package) != domain.DigestJSON(manifest) {
		return nil, &AppError{Code: "INTEGRITY_ERROR", Message: "方案当前视图与manifest版本不一致"}
	}
	v.Timeline, _ = s.store.Audit(id)
	today := time.Now().Format("2006-01-02")
	for i := range v.Issues {
		v.Issues[i].Overdue = v.Issues[i].DueDate != "" && v.Issues[i].DueDate < today && v.Issues[i].Status != "已关闭"
	}
	v.AllowedActions = actions(v.Package.State)
	return &v, nil
}
func (s *Service) save(v *View, action string) error {
	v.Package.UpdatedAt = time.Now()
	if err := s.store.SavePackage(v.Package); err != nil {
		return err
	}
	if err := s.store.SaveJSON("views", v.Package.PackageID, v); err != nil {
		return err
	}
	e := domain.AuditEvent{At: time.Now(), Action: action, PackageID: v.Package.PackageID, Revision: v.Package.Revision}
	if err := s.store.AppendAudit(e); err != nil {
		return err
	}
	v.Timeline, _ = s.store.Audit(v.Package.PackageID)
	v.AllowedActions = actions(v.Package.State)
	return nil
}
func (s *Service) View(id string) (*View, error) {
	v, err := s.load(id)
	if err != nil {
		return nil, err
	}
	return v, nil
}
func actions(st domain.State) []string {
	switch st {
	case domain.StateDraft:
		return []string{"edit_draft", "baseline_preview", "freeze"}
	case domain.StateReview:
		return []string{"validate"}
	case domain.StateRehearsal:
		return []string{"rehearse"}
	case domain.StateRemediation:
		return []string{"revise", "retest", "validate", "tasks"}
	case domain.StateApproval:
		return []string{"approve", "reject"}
	default:
		return []string{}
	}
}

func (s *Service) Worklist(packageID, keyword, state string, page, pageSize int) (Worklist, error) {
	rawKeyword := keyword
	packageID, keyword, state = strings.TrimSpace(packageID), strings.TrimSpace(keyword), strings.TrimSpace(state)
	if rawKeyword != "" && keyword == "" {
		return Worklist{}, invalid("标题关键字不能为空白")
	}
	validStates := map[domain.State]bool{domain.StateDraft: true, domain.StateReview: true, domain.StateRehearsal: true, domain.StateRemediation: true, domain.StateApproval: true, domain.StatePublished: true, domain.StateRejected: true}
	if state != "" && !validStates[domain.State(state)] {
		return Worklist{}, invalid("未知方案状态")
	}
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return Worklist{}, invalid("分页参数越界，page至少为1且page_size范围为1到100")
	}
	result, err := s.store.QueryPackages(store.PackageQuery{PackageID: packageID, Keyword: keyword, State: domain.State(state), Page: page, PageSize: pageSize})
	if err != nil {
		return Worklist{}, &AppError{Code: "INTEGRITY_ERROR", Message: err.Error()}
	}
	out := Worklist{Items: []WorkItem{}, Page: page, PageSize: pageSize, Total: result.Total}
	if result.Total > 0 {
		out.TotalPages = (result.Total + pageSize - 1) / pageSize
	}
	for _, p := range result.Items {
		v, err := s.load(p.PackageID)
		if err != nil {
			return Worklist{}, &AppError{Code: "INTEGRITY_ERROR", Message: fmt.Sprintf("方案%s视图损坏: %v", p.PackageID, err)}
		}
		open := 0
		for _, issue := range v.Issues {
			if issue.Status != "已关闭" {
				open++
			}
		}
		latest := ""
		if len(v.Timeline) > 0 {
			latest = v.Timeline[len(v.Timeline)-1].Action
		}
		next := "无可用操作"
		if len(v.AllowedActions) > 0 {
			next = v.AllowedActions[0]
		}
		readOnly := p.State == domain.StatePublished || p.State == domain.StateRejected
		out.Items = append(out.Items, WorkItem{PackageID: p.PackageID, Title: p.Title, State: p.State, Revision: p.Revision, OpenIssueCount: open, LatestAction: latest, NextAction: next, ReadOnly: readOnly, UpdatedAt: p.UpdatedAt})
	}
	return out, nil
}
func checkRevision(p *domain.BroadcastPackage, expected int) error {
	if p.Revision != expected {
		return conflict(fmt.Sprintf("页面版本%d已过期，当前版本为%d", expected, p.Revision))
	}
	return p.CanEdit()
}

func (s *Service) idempotent(meta CommandMeta, fn func() (*View, error)) (*View, error) {
	if meta.RequestID == "" {
		return nil, invalid("request_id不能为空")
	}
	if old, err := s.store.LoadIdempotency(meta.RequestID); err == nil {
		if old.Fingerprint != meta.Fingerprint {
			return nil, &AppError{Code: "IDEMPOTENCY_CONFLICT", Message: "request_id已用于不同请求"}
		}
		var v View
		if json.Unmarshal(old.Response, &v) != nil {
			return nil, errors.New("幂等响应损坏")
		}
		return &v, nil
	}
	v, err := fn()
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(v)
	if err = s.store.SaveIdempotency(store.Idempotency{RequestID: meta.RequestID, Fingerprint: meta.Fingerprint, Response: b}); err != nil {
		return nil, err
	}
	return v, nil
}

package application

import (
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/validation"
	"fmt"
	"os"
	"strings"
	"time"
)

func (s *Service) Create(meta CommandMeta, in CreateInput) (*View, error) {
	unlock := s.locks.lock(in.PackageID)
	defer unlock()
	return s.idempotent(meta, func() (*View, error) {
		if _, err := s.store.LoadPackage(strings.TrimSpace(in.PackageID)); err == nil {
			return nil, conflict("方案标识已存在")
		} else if !os.IsNotExist(err) {
			return nil, &AppError{Code: "INTEGRITY_ERROR", Message: "同标识方案清单损坏: " + err.Error()}
		}
		now := time.Now()
		segments, err := domain.NormalizeSegments(in.PackageID, in.Segments)
		if err != nil {
			return nil, invalid(err.Error())
		}
		p := &domain.BroadcastPackage{PackageID: strings.TrimSpace(in.PackageID), Title: strings.TrimSpace(in.Title), State: domain.StateDraft, Revision: 1, Segments: segments, CreatedAt: now, UpdatedAt: now}
		for i := range p.Segments {
			p.Segments[i].PackageID = p.PackageID
			p.AddWriter(p.Segments[i].WriterID)
		}
		p.AddWriter(in.WriterID)
		if err := p.ValidateIdentity(); err != nil {
			return nil, invalid(err.Error())
		}
		v := &View{Package: p, Issues: []domain.RemediationIssue{}}
		if err := s.save(v, "创建方案"); err != nil {
			return nil, err
		}
		return v, nil
	})
}
func (s *Service) EditDraft(id string, meta CommandMeta, in EditDraftInput) (*View, error) {
	unlock := s.locks.lock(id)
	defer unlock()
	return s.idempotent(meta, func() (*View, error) {
		v, err := s.load(id)
		if err != nil {
			return nil, err
		}
		if err = checkRevision(v.Package, in.ExpectedRevision); err != nil {
			return nil, err
		}
		if v.Package.State != domain.StateDraft {
			return nil, invalid("仅草拟状态允许续编脚本")
		}
		segments, err := domain.NormalizeSegments(id, in.Segments)
		if err != nil {
			return nil, invalid(err.Error())
		}
		v.Package.Segments = segments
		v.Package.AddWriter(in.WriterID)
		v.Package.Revision++
		if err = s.save(v, "续编并整理脚本段序"); err != nil {
			return nil, err
		}
		return v, nil
	})
}

func (s *Service) PreviewBaseline(id string, expected int, b domain.Baseline) (domain.BaselinePreview, error) {
	v, err := s.load(id)
	if err != nil {
		return domain.BaselinePreview{}, err
	}
	if err = checkRevision(v.Package, expected); err != nil {
		return domain.BaselinePreview{}, err
	}
	if v.Package.State != domain.StateDraft {
		return domain.BaselinePreview{}, invalid("当前状态不能预检基线")
	}
	return domain.PreviewBaseline(b, v.Package.Segments), nil
}
func (s *Service) Freeze(id string, meta CommandMeta, in BaselineInput) (*View, error) {
	unlock := s.locks.lock(id)
	defer unlock()
	return s.idempotent(meta, func() (*View, error) {
		v, e := s.load(id)
		if e != nil {
			return nil, e
		}
		if e = checkRevision(v.Package, in.ExpectedRevision); e != nil {
			return nil, e
		}
		preview := domain.PreviewBaseline(in.Baseline, v.Package.Segments)
		if !preview.Valid {
			return nil, &AppError{Code: "VALIDATION_ERROR", Message: "基线预检未通过", Details: preview.Errors}
		}
		if in.PreviewDigest == "" || in.PreviewDigest != preview.Digest {
			return nil, conflict("基线预览摘要已变化，请重新预检确认")
		}
		if e = v.Package.FreezeBaseline(preview.Normalized); e != nil {
			return nil, invalid(e.Error())
		}
		if e = s.save(v, "冻结基线"); e != nil {
			return nil, e
		}
		return v, nil
	})
}
func (s *Service) Validate(id string, meta CommandMeta, expected int) (*View, error) {
	unlock := s.locks.lock(id)
	defer unlock()
	return s.idempotent(meta, func() (*View, error) {
		v, e := s.load(id)
		if e != nil {
			return nil, e
		}
		if e = checkRevision(v.Package, expected); e != nil {
			return nil, e
		}
		if v.Package.State != domain.StateReview && v.Package.State != domain.StateRemediation {
			return nil, invalid("当前状态不能执行完整校验")
		}
		r := validation.Validate(v.Package)
		v.Validation = &r
		batch := domain.ValidationBatch{PackageID: id, ScriptRevision: v.Package.Revision, ScriptDigest: domain.DigestJSON(v.Package), BaselineDigest: v.Package.BaselineDigest, Report: r, CheckedAt: r.CheckedAt}
		batch.BatchID = domain.DigestJSON(batch)
		if e = s.store.SaveValidationBatch(batch); e != nil {
			return nil, e
		}
		v.ValidationBatches = append(v.ValidationBatches, batch)
		if v.Package.State == domain.StateRemediation {
			existing := map[string]bool{}
			for _, issue := range v.Issues {
				existing[issue.SourceRef+"|"+strings.Join(issue.AffectedSegmentIDs, ",")] = true
			}
			for n, issue := range r.Issues {
				key := issue.RuleID + "|" + strings.Join(issue.SegmentIDs, ",")
				if existing[key] {
					continue
				}
				v.Issues = append(v.Issues, domain.RemediationIssue{IssueID: fmt.Sprintf("validation-%s-%d", batch.BatchID[:8], n+1), PackageID: id, SourceType: "validation", SourceRef: issue.RuleID, AffectedSegmentIDs: issue.SegmentIDs, Status: "待分派", Priority: "中"})
			}
			v.Package.Revision++
		} else if r.Passed {
			e = v.Package.Transition(domain.StateRehearsal)
		} else {
			e = v.Package.Transition(domain.StateRemediation)
			v.Issues = issuesFromValidation(id, r)
		}
		if e != nil {
			return nil, invalid(e.Error())
		}
		if e = s.save(v, "执行脚本校验"); e != nil {
			return nil, e
		}
		return v, nil
	})
}
func issuesFromValidation(id string, r domain.ValidationReport) []domain.RemediationIssue {
	out := []domain.RemediationIssue{}
	for n, i := range r.Issues {
		out = append(out, domain.RemediationIssue{IssueID: fmt.Sprintf("validation-%d", n+1), PackageID: id, SourceType: "validation", SourceRef: i.RuleID, AffectedSegmentIDs: i.SegmentIDs, Status: "待分派", Priority: "中"})
	}
	return out
}

func (s *Service) Rehearse(id string, meta CommandMeta, in RehearsalInput) (*View, error) {
	unlock := s.locks.lock(id)
	defer unlock()
	return s.idempotent(meta, func() (*View, error) {
		v, e := s.load(id)
		if e != nil {
			return nil, e
		}
		if e = checkRevision(v.Package, in.ExpectedRevision); e != nil {
			return nil, e
		}
		if strings.TrimSpace(in.RecorderID) == "" {
			return nil, invalid("演练记录人不能为空")
		}
		evaluation := validation.EvaluateRehearsal(v.Package, in.Results)
		if len(evaluation.FieldErrors) > 0 {
			return nil, &AppError{Code: "VALIDATION_ERROR", Message: "演练记录完整性预检未通过", Details: evaluation.FieldErrors}
		}
		passed, problems := len(evaluation.Issues) == 0, evaluation.Issues
		run := domain.RehearsalRun{RunID: domain.NewID("run"), PackageID: id, ScriptRevision: v.Package.Revision, ScriptDigest: domain.DigestJSON(v.Package), RecorderID: in.RecorderID, StartedAt: time.Now(), SegmentResults: in.Results, TotalSeconds: evaluation.Statistics.ActualTotalSeconds, Statistics: evaluation.Statistics, EvidenceDigest: domain.DigestJSON(in.Results), Outcome: "通过"}
		if !passed {
			run.Outcome = "需整改"
		}
		v.Rehearsal = &run
		v.Package.AddRecorder(in.RecorderID)
		if passed {
			e = v.Package.Transition(domain.StateApproval)
		} else {
			e = v.Package.Transition(domain.StateRemediation)
			for n, i := range problems {
				v.Issues = append(v.Issues, domain.RemediationIssue{IssueID: fmt.Sprintf("rehearsal-%d", n+1), PackageID: id, SourceType: "rehearsal", SourceRef: i.RuleID, AffectedSegmentIDs: i.SegmentIDs, Status: "待分派", Priority: "中"})
			}
		}
		if e != nil {
			return nil, invalid(e.Error())
		}
		if e = s.store.SaveRehearsal(id, run); e != nil {
			return nil, e
		}
		if e = s.save(v, "登记计时演练"); e != nil {
			return nil, e
		}
		return v, nil
	})
}

func (s *Service) Revise(id string, meta CommandMeta, in ReviseInput) (*View, error) {
	unlock := s.locks.lock(id)
	defer unlock()
	return s.idempotent(meta, func() (*View, error) {
		v, e := s.load(id)
		if e != nil {
			return nil, e
		}
		if e = checkRevision(v.Package, in.ExpectedRevision); e != nil {
			return nil, e
		}
		if v.Package.State != domain.StateRemediation {
			return nil, invalid("仅整改中方案可修订")
		}
		if strings.TrimSpace(in.Cause) == "" || strings.TrimSpace(in.ChangeSummary) == "" {
			return nil, invalid("必须填写原因与修订说明")
		}
		found := false
		for i := range v.Issues {
			if v.Issues[i].IssueID == in.IssueID && v.Issues[i].Status != "已关闭" {
				v.Issues[i].Cause = in.Cause
				v.Issues[i].ChangeSummary = in.ChangeSummary
				found = true
			}
		}
		if !found {
			return nil, invalid("未找到待整改问题")
		}
		if len(in.Segments) > 0 {
			segments, normalizeErr := domain.NormalizeSegments(id, in.Segments)
			if normalizeErr != nil {
				return nil, invalid(normalizeErr.Error())
			}
			v.Package.Segments = segments
			for i := range v.Package.Segments {
				v.Package.Segments[i].PackageID = id
				v.Package.Segments[i].RevisionReason = in.ChangeSummary
			}
		}
		for i := range v.Issues {
			if v.Issues[i].IssueID == in.IssueID {
				v.Issues[i].Status = "待复验"
			}
		}
		v.Package.AddWriter(in.WriterID)
		v.Package.Revision++
		if e = s.save(v, "提交整改修订"); e != nil {
			return nil, e
		}
		return v, nil
	})
}

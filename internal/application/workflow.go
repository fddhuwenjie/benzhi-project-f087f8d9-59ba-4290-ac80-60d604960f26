package application

import (
	"broadcastdesk/internal/domain"
	"fmt"
	"sort"
	"strings"
	"time"
)

func (s *Service) ValidationBatches(packageID string) ([]domain.ValidationBatch, error) {
	v, err := s.load(packageID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ValidationBatch, 0, len(v.ValidationBatches))
	for _, ref := range v.ValidationBatches {
		batch, err := s.store.LoadValidationBatch(packageID, ref.BatchID)
		if err != nil {
			return nil, &AppError{Code: "INTEGRITY_ERROR", Message: err.Error()}
		}
		out = append(out, batch)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CheckedAt.Equal(out[j].CheckedAt) {
			return out[i].BatchID < out[j].BatchID
		}
		return out[i].CheckedAt.Before(out[j].CheckedAt)
	})
	return out, nil
}

func (s *Service) ValidationBatch(packageID, batchID string) (domain.ValidationBatch, error) {
	b, err := s.store.LoadValidationBatch(packageID, batchID)
	if err != nil {
		return b, &AppError{Code: "INTEGRITY_ERROR", Message: err.Error()}
	}
	return b, nil
}

func (s *Service) ValidationDiff(packageID, fromID, toID string) (domain.ValidationDiff, error) {
	from, err := s.ValidationBatch(packageID, fromID)
	if err != nil {
		return domain.ValidationDiff{}, err
	}
	to, err := s.ValidationBatch(packageID, toID)
	if err != nil {
		return domain.ValidationDiff{}, err
	}
	return domain.DiffValidation(from, to), nil
}

func (s *Service) BatchTasks(id string, meta CommandMeta, in BatchTasksInput) (*View, error) {
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
		if v.Package.State != domain.StateRemediation {
			return nil, invalid("仅整改中方案可批量推进任务")
		}
		if len(in.Updates) == 0 {
			return nil, invalid("至少选择一个问题")
		}
		issues := append([]domain.RemediationIssue(nil), v.Issues...)
		seen := map[string]bool{}
		for _, update := range in.Updates {
			if seen[update.IssueID] {
				return nil, invalid("批量操作包含重复问题")
			}
			seen[update.IssueID] = true
			found := -1
			for i := range issues {
				if issues[i].IssueID == update.IssueID {
					found = i
					break
				}
			}
			if found < 0 {
				return nil, invalid("批量操作包含未知问题: " + update.IssueID)
			}
			if err = issues[found].Assign(update.AssigneeID, update.DueDate, update.Priority, update.Status, time.Now()); err != nil {
				return nil, &AppError{Code: "REVISION_CONFLICT", Message: fmt.Sprintf("问题%s无法更新: %v", update.IssueID, err)}
			}
		}
		v.Issues = issues
		v.Package.Revision++
		if err = s.save(v, "批量分派和推进整改任务"); err != nil {
			return nil, err
		}
		return v, nil
	})
}

func (s *Service) Issues(id string, filter IssueFilter) ([]domain.RemediationIssue, error) {
	v, err := s.load(id)
	if err != nil {
		return nil, err
	}
	validStatus := map[string]bool{"": true, "待分派": true, "处理中": true, "待复验": true, "已关闭": true}
	validPriority := map[string]bool{"": true, "低": true, "中": true, "高": true, "紧急": true}
	if !validStatus[filter.Status] || !validPriority[filter.Priority] {
		return nil, invalid("未知任务筛选条件")
	}
	if filter.Today.IsZero() {
		filter.Today = time.Now()
	}
	out := []domain.RemediationIssue{}
	for _, issue := range v.Issues {
		issue.Overdue = issue.DueDate != "" && issue.DueDate < filter.Today.Format("2006-01-02") && issue.Status != "已关闭"
		if filter.AssigneeID != "" && issue.AssigneeID != filter.AssigneeID {
			continue
		}
		if filter.Status != "" && issue.Status != filter.Status {
			continue
		}
		if filter.Priority != "" && issue.Priority != filter.Priority {
			continue
		}
		if filter.Overdue != nil && issue.Overdue != *filter.Overdue {
			continue
		}
		out = append(out, issue)
	}
	return out, nil
}

func buildReview(v *View) domain.ReviewSnapshot {
	closed, open := 0, 0
	for _, issue := range v.Issues {
		if issue.Status == "已关闭" {
			closed++
		} else {
			open++
		}
	}
	baselineSummary := "无冻结基线"
	if v.Package.Baseline != nil {
		baselineSummary = fmt.Sprintf("%s / %s / %s / %d秒 / %s", v.Package.Baseline.Scenario, v.Package.Baseline.Audience, v.Package.Baseline.Channel, v.Package.Baseline.MaxSeconds, v.Package.BaselineDigest)
	}
	rehearsalSummary := "无演练记录"
	if v.Rehearsal != nil {
		rehearsalSummary = fmt.Sprintf("%s，实际%d秒，余量%d秒，证据%s", v.Rehearsal.Outcome, v.Rehearsal.TotalSeconds, v.Rehearsal.Statistics.RemainingSeconds, v.Rehearsal.EvidenceDigest)
	}
	retest := "无整改复验"
	if len(v.Issues) > 0 {
		retest = fmt.Sprintf("已关闭%d项，未关闭%d项", closed, open)
	}
	changes := []string{}
	for _, segment := range v.Package.Segments {
		if strings.TrimSpace(segment.RevisionReason) != "" {
			changes = append(changes, segment.SegmentID+":"+segment.RevisionReason)
		}
	}
	scriptSummary := fmt.Sprintf("revision %d，脚本摘要%s，冻结后无脚本修订", v.Package.Revision, domain.DigestJSON(v.Package.CanonicalScript()))
	if len(changes) > 0 {
		scriptSummary = fmt.Sprintf("revision %d，脚本摘要%s，变更段%s", v.Package.Revision, domain.DigestJSON(v.Package.CanonicalScript()), strings.Join(changes, "；"))
	}
	items := []domain.ReviewItem{
		{ItemID: "baseline", Category: "冻结基线", Title: "冻结规范", Summary: baselineSummary},
		{ItemID: "script", Category: "脚本差异", Title: "当前脚本修订", Summary: scriptSummary},
		{ItemID: "rehearsal", Category: "演练摘要", Title: "完整计时演练", Summary: rehearsalSummary},
		{ItemID: "issues", Category: "问题关闭", Title: "整改问题", Summary: fmt.Sprintf("已关闭%d项，未关闭%d项", closed, open)},
		{ItemID: "retest", Category: "定向复验", Title: "复验结果", Summary: retest},
	}
	snapshot := domain.ReviewSnapshot{PackageID: v.Package.PackageID, PackageRevision: v.Package.Revision, Items: items, CreatedAt: time.Now()}
	snapshot.Digest = reviewDigest(snapshot)
	return snapshot
}

func reviewDigest(snapshot domain.ReviewSnapshot) string {
	return domain.DigestJSON(struct {
		PackageID       string
		PackageRevision int
		Items           []domain.ReviewItem
	}{snapshot.PackageID, snapshot.PackageRevision, snapshot.Items})
}

func (s *Service) Review(id string) (domain.ReviewSnapshot, error) {
	v, err := s.load(id)
	if err != nil {
		return domain.ReviewSnapshot{}, err
	}
	if v.Package.State != domain.StateApproval {
		return domain.ReviewSnapshot{}, invalid("方案尚未进入待批准")
	}
	snapshot := buildReview(v)
	return snapshot, nil
}

func confirmedAll(snapshot domain.ReviewSnapshot, confirmed []string) bool {
	set := map[string]bool{}
	for _, id := range confirmed {
		set[strings.TrimSpace(id)] = true
	}
	for _, item := range snapshot.Items {
		if !set[item.ItemID] {
			return false
		}
	}
	return true
}

func (s *Service) ReleaseManifest(id string) (domain.ReleaseManifest, error) {
	if err := s.store.Verify(); err != nil {
		return domain.ReleaseManifest{}, &AppError{Code: "INTEGRITY_ERROR", Message: err.Error()}
	}
	v, err := s.load(id)
	if err != nil {
		return domain.ReleaseManifest{}, err
	}
	if v.Package.State != domain.StatePublished || v.Bundle == nil || v.Rehearsal == nil || v.SigningSnapshot == nil {
		return domain.ReleaseManifest{}, invalid("已发布方案的清单组成不完整")
	}
	if _, err = s.store.LoadPackageObject(v.Rehearsal.ScriptDigest); err != nil {
		return domain.ReleaseManifest{}, &AppError{Code: "INTEGRITY_ERROR", Message: "演练引用脚本版本不完整: " + err.Error()}
	}
	storedRun, err := s.store.LoadRehearsal(id)
	if err != nil || domain.DigestJSON(storedRun) != domain.DigestJSON(v.Rehearsal) {
		return domain.ReleaseManifest{}, &AppError{Code: "INTEGRITY_ERROR", Message: "演练记录引用不一致"}
	}
	storedBundle, err := s.store.LoadBundle(id)
	if err != nil || domain.DigestJSON(storedBundle) != domain.DigestJSON(v.Bundle) {
		return domain.ReleaseManifest{}, &AppError{Code: "INTEGRITY_ERROR", Message: "发布包引用不一致"}
	}
	if v.Bundle.SigningSnapshot == nil || domain.DigestJSON(v.Bundle.SigningSnapshot) != domain.DigestJSON(v.SigningSnapshot) {
		return domain.ReleaseManifest{}, &AppError{Code: "INTEGRITY_ERROR", Message: "发布包签署快照不一致"}
	}
	if reviewDigest(v.SigningSnapshot.ReviewSnapshot) != v.SigningSnapshot.ReviewSnapshot.Digest {
		return domain.ReleaseManifest{}, &AppError{Code: "INTEGRITY_ERROR", Message: "审阅清单摘要不一致"}
	}
	signingCopy := *v.SigningSnapshot
	signingDigest := signingCopy.Digest
	signingCopy.Digest = ""
	if domain.DigestJSON(signingCopy) != signingDigest {
		return domain.ReleaseManifest{}, &AppError{Code: "INTEGRITY_ERROR", Message: "签署快照摘要不一致"}
	}
	anchor := domain.AuditAnchor{}
	for _, event := range v.Timeline {
		if event.Action == "批准并冻结发布包" {
			anchor = domain.AuditAnchor{Sequence: event.Sequence, Digest: event.Digest}
		}
	}
	if anchor.Sequence == 0 {
		return domain.ReleaseManifest{}, &AppError{Code: "INTEGRITY_ERROR", Message: "发布审计锚点缺失"}
	}
	manifest := domain.ReleaseManifest{FormatVersion: domain.ReleaseManifestVersion, PackageID: id, CanonicalScript: v.Bundle.CanonicalScript, Pronunciation: v.Bundle.PronunciationGlossary, Rehearsal: *v.Rehearsal, Signing: *v.SigningSnapshot, AuditAnchor: anchor}
	manifest.CalculateDigests()
	return manifest, nil
}

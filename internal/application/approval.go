package application

import (
	"broadcastdesk/internal/domain"
	"broadcastdesk/internal/validation"
	"strings"
	"time"
)

func (s *Service) Retest(id string, meta CommandMeta, expected int) (*View, error) {
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
		if v.Package.State != domain.StateRemediation {
			return nil, invalid("当前状态无需复验")
		}
		for _, issue := range v.Issues {
			if issue.Status != "已关闭" && issue.Status != "待复验" {
				return nil, invalid("只有原因、修订说明和受影响范围完整的问题才能复验")
			}
		}
		r := validation.Validate(v.Package)
		v.Validation = &r
		for _, current := range r.Issues {
			for _, issue := range v.Issues {
				if issue.Status == "已关闭" {
					continue
				}
				matched := current.RuleID == issue.SourceRef
				for _, a := range issue.AffectedSegmentIDs {
					for _, b := range current.SegmentIDs {
						if a == b {
							matched = true
						}
					}
				}
				if matched {
					return nil, invalid("定向复验仍有未通过规则: " + current.RuleID)
				}
			}
		}
		for i := range v.Issues {
			if strings.TrimSpace(v.Issues[i].Cause) == "" || strings.TrimSpace(v.Issues[i].ChangeSummary) == "" {
				return nil, invalid("问题原因和修订说明不完整")
			}
			v.Issues[i].Status = "已关闭"
			v.Issues[i].RetestResult = "通过"
		}
		requiresRehearsal := v.Rehearsal == nil
		for _, issue := range v.Issues {
			if issue.SourceType == "rehearsal" {
				requiresRehearsal = true
			}
		}
		if requiresRehearsal {
			e = v.Package.Transition(domain.StateRehearsal)
		} else {
			e = v.Package.Transition(domain.StateApproval)
		}
		if e != nil {
			return nil, invalid(e.Error())
		}
		if e = s.save(v, "完成定向复验"); e != nil {
			return nil, e
		}
		return v, nil
	})
}

func (s *Service) Decide(id string, meta CommandMeta, in ApproveInput) (*View, error) {
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
		if v.Package.State != domain.StateApproval {
			return nil, invalid("方案尚未进入待批准")
		}
		if e = v.Package.IndependentApprover(in.ApproverID); e != nil {
			return nil, invalid(e.Error())
		}
		snapshot := buildReview(v)
		if in.ReviewDigest == "" || snapshot.Digest != in.ReviewDigest {
			return nil, conflict("审阅清单摘要已变化，请重新载入证据")
		}
		if in.Decision == "reject" {
			if strings.TrimSpace(in.Statement) == "" {
				return nil, invalid("拒绝理由不能为空")
			}
			signing := domain.SigningSnapshot{ReviewSnapshot: snapshot, ApproverID: in.ApproverID, Decision: in.Decision, Statement: strings.TrimSpace(in.Statement), SignedAt: time.Now()}
			signing.Digest = domain.DigestJSON(signing)
			v.SigningSnapshot = &signing
			if e = v.Package.Transition(domain.StateRejected); e != nil {
				return nil, e
			}
			if e = s.save(v, "拒绝发布"); e != nil {
				return nil, e
			}
			return v, nil
		}
		if in.Decision != "approve" {
			return nil, invalid("决定必须为approve或reject")
		}
		for _, issue := range v.Issues {
			if issue.Status != "已关闭" {
				return nil, invalid("存在未关闭问题，不能批准")
			}
		}
		if !confirmedAll(snapshot, in.ConfirmedItemIDs) {
			return nil, invalid("批准前必须逐项确认全部审阅清单")
		}
		if strings.TrimSpace(in.Statement) == "" {
			return nil, invalid("签署意见不能为空")
		}
		signing := domain.SigningSnapshot{ReviewSnapshot: snapshot, ApproverID: in.ApproverID, Decision: in.Decision, Statement: strings.TrimSpace(in.Statement), SignedAt: time.Now()}
		signing.Digest = domain.DigestJSON(signing)
		v.SigningSnapshot = &signing
		if e = v.Package.Transition(domain.StatePublished); e != nil {
			return nil, e
		}
		summary := ""
		if v.Rehearsal != nil {
			summary = validation.Summary(domain.ValidationReport{Passed: v.Rehearsal.Outcome == "通过"})
		}
		material := v.Package.PublishMaterial(summary, in.ApproverID, in.Statement)
		bundle := domain.ReleaseBundle{BundleID: domain.DigestJSON(material), PackageID: id, PackageRevision: v.Package.Revision, CanonicalScript: material.CanonicalScript, PronunciationGlossary: material.Glossary, RehearsalSummary: summary, ApproverID: in.ApproverID, ApprovalStatement: in.Statement, IssuedAt: time.Now(), SHA256Digest: domain.DigestJSON(material), SigningSnapshot: &signing}
		v.Bundle = &bundle
		if e = s.store.SaveBundle(id, bundle); e != nil {
			return nil, e
		}
		if e = s.save(v, "批准并冻结发布包"); e != nil {
			return nil, e
		}
		return v, nil
	})
}

func (s *Service) VerifyBundle(id string) (bool, string, error) {
	v, e := s.load(id)
	if e != nil {
		return false, "", e
	}
	if v.Package.State != domain.StatePublished || v.Bundle == nil || v.Rehearsal == nil || v.SigningSnapshot == nil {
		return false, "", invalid("已发布方案的清单组成不完整")
	}
	if _, err := s.store.LoadPackageObject(v.Rehearsal.ScriptDigest); err != nil {
		return false, "", &AppError{Code: "INTEGRITY_ERROR", Message: "演练引用脚本版本不完整: " + err.Error()}
	}
	storedRun, err := s.store.LoadRehearsal(id)
	if err != nil || domain.DigestJSON(storedRun) != domain.DigestJSON(v.Rehearsal) {
		return false, "", &AppError{Code: "INTEGRITY_ERROR", Message: "演练记录引用不一致"}
	}
	storedBundle, err := s.store.LoadBundle(id)
	if err != nil || domain.DigestJSON(storedBundle) != domain.DigestJSON(v.Bundle) {
		return false, "", &AppError{Code: "INTEGRITY_ERROR", Message: "发布包引用不一致"}
	}
	if v.Bundle.SigningSnapshot == nil || domain.DigestJSON(v.Bundle.SigningSnapshot) != domain.DigestJSON(v.SigningSnapshot) {
		return false, "", &AppError{Code: "INTEGRITY_ERROR", Message: "发布包签署快照不一致"}
	}
	if reviewDigest(v.SigningSnapshot.ReviewSnapshot) != v.SigningSnapshot.ReviewSnapshot.Digest {
		return false, "", &AppError{Code: "INTEGRITY_ERROR", Message: "审阅清单摘要不一致"}
	}
	signingCopy := *v.SigningSnapshot
	signingDigest := signingCopy.Digest
	signingCopy.Digest = ""
	if domain.DigestJSON(signingCopy) != signingDigest {
		return false, "", &AppError{Code: "INTEGRITY_ERROR", Message: "签署快照摘要不一致"}
	}
	m := v.Package.PublishMaterial(v.Bundle.RehearsalSummary, v.Bundle.ApproverID, v.Bundle.ApprovalStatement)
	d := domain.DigestJSON(m)
	return d == v.Bundle.SHA256Digest, d, nil
}

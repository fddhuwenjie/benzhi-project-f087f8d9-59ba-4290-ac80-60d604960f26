package store

import (
	"broadcastdesk/internal/domain"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// verifyView mirrors application.View for integrity verification. It keeps the
// same JSON shape so that strictUnmarshal (DisallowUnknownFields) applies
// identically during startup checks and during the normal reads performed by
// Service.load, ensuring the startup verification rejects any view corruption
// that would otherwise make a later read fail.
type verifyView struct {
	Package           *domain.BroadcastPackage  `json:"package"`
	Validation        *domain.ValidationReport  `json:"validation,omitempty"`
	Rehearsal         *domain.RehearsalRun      `json:"rehearsal,omitempty"`
	Issues            []domain.RemediationIssue `json:"issues"`
	Bundle            *domain.ReleaseBundle     `json:"bundle,omitempty"`
	Timeline          []domain.AuditEvent       `json:"timeline"`
	AllowedActions    []string                  `json:"allowed_actions"`
	ValidationBatches []domain.ValidationBatch  `json:"validation_batches"`
	ReviewSnapshot    *domain.ReviewSnapshot    `json:"review_snapshot,omitempty"`
	SigningSnapshot   *domain.SigningSnapshot   `json:"signing_snapshot,omitempty"`
}

func (s *Store) Verify() error {
	_, err := os.Stat(filepath.Join(s.root, "objects"))
	if err != nil {
		return err
	}
	if _, err = os.Stat(filepath.Join(s.root, "manifests")); err != nil {
		return err
	}
	entries, _ := os.ReadDir(filepath.Join(s.root, "manifests"))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fileName := e.Name()
		if filepath.Ext(fileName) != ".json" {
			continue
		}
		pkgID := strings.TrimSuffix(fileName, ".json")
		manifestBytes, er := os.ReadFile(filepath.Join(s.root, "manifests", fileName))
		if er != nil {
			return er
		}
		manifestDigest := string(manifestBytes)
		if manifestDigest == "" {
			return fmt.Errorf("清单摘要为空: %s", pkgID)
		}
		obj, er := os.ReadFile(filepath.Join(s.root, "objects", manifestDigest+".json"))
		if er != nil {
			return er
		}
		var p domain.BroadcastPackage
		if strictUnmarshal(obj, &p) != nil {
			return fmt.Errorf("对象损坏: %s", pkgID)
		}
		if domain.DigestJSON(&p) != manifestDigest {
			return fmt.Errorf("对象摘要不一致: %s", pkgID)
		}
		// Strictly validate the current view for this package: the
		// content-addressed pointer, the view object, its digest, the
		// package identity inside the view, and consistency with the
		// manifest's current version. This mirrors the checks performed
		// by Service.load so any view corruption that would make a normal
		// read fail is rejected at startup.
		var v verifyView
		if verr := s.LoadJSON("views", pkgID, &v); verr != nil {
			if os.IsNotExist(verr) {
				return fmt.Errorf("方案视图缺失: %s", pkgID)
			}
			return fmt.Errorf("方案视图损坏: %s: %w", pkgID, verr)
		}
		if v.Package == nil {
			return fmt.Errorf("方案视图缺少方案身份: %s", pkgID)
		}
		if domain.DigestJSON(v.Package) != manifestDigest {
			return fmt.Errorf("方案视图与清单当前版本不一致: %s", pkgID)
		}
	}
	audit, er := s.Audit("")
	if er != nil {
		return er
	}
	prev := ""
	for i, e := range audit {
		if e.Sequence != i+1 || e.PreviousDigest != prev {
			return fmt.Errorf("审计链不连续")
		}
		check := e
		check.Digest = ""
		if domain.DigestJSON(check) != e.Digest {
			return fmt.Errorf("审计摘要不一致")
		}
		prev = e.Digest
	}
	return nil
}

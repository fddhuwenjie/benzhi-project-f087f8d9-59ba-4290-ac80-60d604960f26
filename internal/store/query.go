package store

import (
	"broadcastdesk/internal/domain"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type PackageQuery struct {
	PackageID string
	Keyword   string
	State     domain.State
	Page      int
	PageSize  int
}

type PackagePage struct {
	Items []*domain.BroadcastPackage
	Total int
}

func (s *Store) QueryPackages(q PackageQuery) (PackagePage, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "manifests"))
	if err != nil {
		return PackagePage{}, err
	}
	items := make([]*domain.BroadcastPackage, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		p, loadErr := s.LoadPackage(id)
		if loadErr != nil {
			return PackagePage{}, fmt.Errorf("方案清单项%s损坏: %w", id, loadErr)
		}
		if q.PackageID != "" && p.PackageID != q.PackageID {
			continue
		}
		if q.Keyword != "" && !strings.Contains(strings.ToLower(p.Title), strings.ToLower(q.Keyword)) {
			continue
		}
		if q.State != "" && p.State != q.State {
			continue
		}
		items = append(items, p)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].PackageID < items[j].PackageID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	total := len(items)
	start := (q.Page - 1) * q.PageSize
	if start >= total {
		return PackagePage{Items: []*domain.BroadcastPackage{}, Total: total}, nil
	}
	end := start + q.PageSize
	if end > total {
		end = total
	}
	return PackagePage{Items: items[start:end], Total: total}, nil
}

func (s *Store) LoadPackageObject(digest string) (*domain.BroadcastPackage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.objectPath(digest))
	if err != nil {
		return nil, err
	}
	var p domain.BroadcastPackage
	if err := strictUnmarshal(b, &p); err != nil {
		return nil, err
	}
	if domain.DigestJSON(&p) != digest {
		return nil, fmt.Errorf("历史方案对象摘要不一致")
	}
	return &p, nil
}

func (s *Store) SaveValidationBatch(batch domain.ValidationBatch) error {
	return s.SaveJSON("validation_batches", batch.BatchID, batch)
}

func (s *Store) LoadValidationBatch(packageID, batchID string) (domain.ValidationBatch, error) {
	var batch domain.ValidationBatch
	if err := s.LoadJSON("validation_batches", batchID, &batch); err != nil {
		return batch, err
	}
	if batch.PackageID != packageID || batch.BatchID != batchID {
		return batch, fmt.Errorf("校验批次不属于方案%s", packageID)
	}
	p, err := s.LoadPackageObject(batch.ScriptDigest)
	if err != nil {
		return batch, fmt.Errorf("校验批次%s引用版本不完整: %w", batchID, err)
	}
	if p.PackageID != packageID || p.Revision != batch.ScriptRevision || p.BaselineDigest != batch.BaselineDigest {
		return batch, fmt.Errorf("校验批次%s引用版本或基线摘要不匹配", batchID)
	}
	return batch, nil
}

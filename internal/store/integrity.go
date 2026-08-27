package store

import (
	"broadcastdesk/internal/domain"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
		id := e.Name()
		b, er := os.ReadFile(filepath.Join(s.root, "manifests", id))
		if er != nil {
			return er
		}
		obj, er := os.ReadFile(filepath.Join(s.root, "objects", string(b)+".json"))
		if er != nil {
			return er
		}
		var p domain.BroadcastPackage
		if json.Unmarshal(obj, &p) != nil {
			return fmt.Errorf("对象损坏: %s", id)
		}
		if domain.DigestJSON(&p) != string(b) {
			return fmt.Errorf("对象摘要不一致: %s", id)
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

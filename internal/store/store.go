package store

import (
	"broadcastdesk/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Store struct {
	root string
	mu   sync.Mutex
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(root, "objects"), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0755); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) objectPath(digest string) string {
	return filepath.Join(s.root, "objects", digest+".json")
}
func (s *Store) manifestPath(id string) string { return filepath.Join(s.root, "manifests", id+".json") }

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) SavePackage(p *domain.BroadcastPackage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(p)
	d := domain.DigestJSON(p)
	if err := os.WriteFile(s.objectPath(d), b, 0644); err != nil {
		return err
	}
	return writeAtomic(s.manifestPath(p.PackageID), []byte(d))
}

func (s *Store) LoadPackage(id string) (*domain.BroadcastPackage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := os.ReadFile(s.manifestPath(id))
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.objectPath(string(m)))
	if err != nil {
		return nil, err
	}
	var p domain.BroadcastPackage
	if err = strictUnmarshal(b, &p); err != nil {
		return nil, err
	}
	if domain.DigestJSON(&p) != string(m) {
		return nil, errors.New("方案摘要校验失败")
	}
	return &p, nil
}

func (s *Store) SaveJSON(kind, id string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(v)
	d := domain.DigestJSON(v)
	dir := filepath.Join(s.root, kind)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, d+".json"), b, 0644); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, id+".json"), []byte(d))
}

func (s *Store) LoadJSON(kind, id string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := os.ReadFile(filepath.Join(s.root, kind, id+".json"))
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(s.root, kind, string(m)+".json"))
	if err != nil {
		return err
	}
	if err := strictUnmarshal(b, v); err != nil {
		return err
	}
	if domain.DigestJSON(v) != string(m) {
		return errors.New("内容寻址对象摘要不一致")
	}
	return nil
}

func strictUnmarshal(b []byte, v any) error {
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("JSON包含多余内容")
		}
		return err
	}
	return nil
}

func (s *Store) AppendAudit(e domain.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.root, "audit.jsonl")
	prev := ""
	lines := [][]byte{}
	if b, err := os.ReadFile(path); err == nil {
		lines = splitLines(b)
		for i, line := range lines {
			var last domain.AuditEvent
			if err := json.Unmarshal(line, &last); err != nil {
				return fmt.Errorf("审计记录损坏: 第%d行: %w", i+1, err)
			}
			prev = last.Digest
		}
	}
	e.Sequence = len(lines) + 1
	e.PreviousDigest = prev
	e.Digest = ""
	e.Digest = domain.DigestJSON(e)
	line, _ := json.Marshal(e)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

func (s *Store) Audit(id string) ([]domain.AuditEvent, error) {
	b, err := os.ReadFile(filepath.Join(s.root, "audit.jsonl"))
	if os.IsNotExist(err) {
		return []domain.AuditEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []domain.AuditEvent{}
	for i, line := range splitLines(b) {
		var e domain.AuditEvent
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("审计记录损坏: 第%d行: %w", i+1, err)
		}
		if id == "" || e.PackageID == id {
			out = append(out, e)
		}
	}
	return out, nil
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

func (s *Store) String() string { return fmt.Sprintf("Store(%s)", s.root) }

package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Idempotency struct {
	RequestID   string          `json:"request_id"`
	Fingerprint string          `json:"fingerprint"`
	Response    json.RawMessage `json:"response"`
}

func (s *Store) SaveIdempotency(v Idempotency) error {
	b, _ := json.Marshal(v)
	dir := filepath.Join(s.root, "idempotency")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, v.RequestID+".json"), b)
}
func (s *Store) LoadIdempotency(id string) (*Idempotency, error) {
	b, err := os.ReadFile(filepath.Join(s.root, "idempotency", id+".json"))
	if err != nil {
		return nil, err
	}
	var v Idempotency
	if json.Unmarshal(b, &v) != nil {
		return nil, errors.New("幂等记录损坏")
	}
	return &v, nil
}

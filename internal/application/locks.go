package application

import "sync"

type packageLocks struct {
	mu     sync.Mutex
	values map[string]*sync.Mutex
}

func newPackageLocks() *packageLocks { return &packageLocks{values: map[string]*sync.Mutex{}} }
func (l *packageLocks) lock(id string) func() {
	l.mu.Lock()
	m := l.values[id]
	if m == nil {
		m = &sync.Mutex{}
		l.values[id] = m
	}
	l.mu.Unlock()
	m.Lock()
	return m.Unlock
}

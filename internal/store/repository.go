package store

import "broadcastdesk/internal/domain"

func (s *Store) SaveValidation(id string, r domain.ValidationReport) error {
	return s.SaveJSON("validation", id, r)
}
func (s *Store) LoadValidation(id string) (domain.ValidationReport, error) {
	var r domain.ValidationReport
	err := s.LoadJSON("validation", id, &r)
	return r, err
}
func (s *Store) SaveRehearsal(id string, r domain.RehearsalRun) error {
	return s.SaveJSON("rehearsal", id, r)
}
func (s *Store) LoadRehearsal(id string) (domain.RehearsalRun, error) {
	var r domain.RehearsalRun
	err := s.LoadJSON("rehearsal", id, &r)
	return r, err
}
func (s *Store) SaveBundle(id string, b domain.ReleaseBundle) error {
	return s.SaveJSON("bundles", id, b)
}
func (s *Store) LoadBundle(id string) (domain.ReleaseBundle, error) {
	var b domain.ReleaseBundle
	err := s.LoadJSON("bundles", id, &b)
	return b, err
}

package domain

import "errors"

func (p *BroadcastPackage) CanEdit() error {
	if p.State == StatePublished || p.State == StateRejected {
		return errors.New("终态方案只读")
	}
	return nil
}

func (p *BroadcastPackage) AddWriter(id string) {
	if id != "" {
		for _, v := range p.Writers {
			if v == id {
				return
			}
		}
		p.Writers = append(p.Writers, id)
	}
}

func (p *BroadcastPackage) AddRecorder(id string) {
	if id != "" {
		for _, v := range p.RehearsalRecorderIDs {
			if v == id {
				return
			}
		}
		p.RehearsalRecorderIDs = append(p.RehearsalRecorderIDs, id)
	}
}

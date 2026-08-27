package domain

import (
	"encoding/json"
	"sort"
)

type PublishMaterial struct {
	PackageID        string            `json:"package_id"`
	Revision         int               `json:"revision"`
	CanonicalScript  string            `json:"canonical_script"`
	Glossary         map[string]string `json:"glossary"`
	RehearsalSummary string            `json:"rehearsal_summary"`
	ApproverID       string            `json:"approver_id"`
	Statement        string            `json:"statement"`
}

func (p *BroadcastPackage) PublishMaterial(summary, approver, statement string) PublishMaterial {
	g := map[string]string{}
	if p.Baseline != nil {
		for k, v := range p.Baseline.Pronunciation {
			g[k] = v
		}
	}
	return PublishMaterial{p.PackageID, p.Revision, p.CanonicalScript(), g, summary, approver, statement}
}

func CanonicalJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

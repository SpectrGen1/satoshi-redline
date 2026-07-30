package v1

import (
	"encoding/json"
	"fmt"
	"github.com/satoshi-redline/redline/internal/domain"
)

// Report is an explicit external contract; no internal structs are serialized directly.
type Report struct {
	Version      string        `json:"version"`
	Scan         Scan          `json:"scan"`
	Observations []Observation `json:"observations"`
	Findings     []Finding     `json:"findings"`
}
type Scan struct {
	ID         string `json:"id"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	Health     string `json:"health"`
	Coverage   string `json:"coverage"`
}
type Observation struct {
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	ComponentID          string   `json:"component_id"`
	ObservedAt           string   `json:"observed_at"`
	RPCReachable         *bool    `json:"rpc_reachable,omitempty"`
	Blocks               *int64   `json:"blocks,omitempty"`
	Headers              *int64   `json:"headers,omitempty"`
	VerificationProgress *float64 `json:"verification_progress,omitempty"`
	InitialBlockDownload *bool    `json:"initial_block_download,omitempty"`
	TipTime              *string  `json:"tip_time,omitempty"`
	TotalBytes           *int64   `json:"total_bytes,omitempty"`
	AvailableBytes       *int64   `json:"available_bytes,omitempty"`
	ReadOnly             *bool    `json:"read_only,omitempty"`
	MountPresent         *bool    `json:"mount_present,omitempty"`
}
type Finding struct {
	Code          string   `json:"code"`
	Status        string   `json:"status"`
	Applicability string   `json:"applicability"`
	Reasons       []string `json:"reasons"`
	Provenance    string   `json:"provenance"`
	Required      bool     `json:"required"`
}

func FromDomain(x domain.Scan) Report {
	r := Report{Version: "v1", Scan: Scan{x.ID, x.StartedAt.Format("2006-01-02T15:04:05Z"), x.FinishedAt.Format("2006-01-02T15:04:05Z"), string(x.Summary.Health), string(x.Summary.Coverage.State)}}
	for _, o := range x.Observations {
		v := Observation{o.ID, o.Kind, o.ComponentID, o.ObservedAt.Format("2006-01-02T15:04:05Z"), o.RPCReachable, o.Blocks, o.Headers, o.VerificationProgress, o.InitialBlockDownload, nil, o.TotalBytes, o.AvailableBytes, o.ReadOnly, o.MountPresent}
		if o.TipTime != nil {
			s := o.TipTime.Format("2006-01-02T15:04:05Z")
			v.TipTime = &s
		}
		r.Observations = append(r.Observations, v)
	}
	for _, f := range x.Findings {
		rs := make([]string, len(f.Reasons))
		for i, a := range f.Reasons {
			rs[i] = string(a)
		}
		r.Findings = append(r.Findings, Finding{f.Code, string(f.Status), string(f.Applicability), rs, f.Provenance, f.Required})
	}
	return r
}
func Marshal(x domain.Scan) ([]byte, error) {
	r := FromDomain(x)
	if r.Version != "v1" || r.Scan.ID == "" {
		return nil, fmt.Errorf("invalid report")
	}
	return json.MarshalIndent(r, "", "  ")
}

// Package probes contains compiled first-party collectors and their static catalog.
package probes

import (
	"context"
	"fmt"
	"github.com/satoshi-redline/redline/internal/capabilities"
	"sort"
	"time"
)

type ScanMode string

const Safe ScanMode = "safe"

type SideEffect string

const ReadOnly SideEffect = "read_only"

type Sensitivity string

const SensitiveNone Sensitivity = "none"

type Descriptor struct {
	ID, Version        string
	ComponentKinds     []string
	Required, Optional []capabilities.ID
	Mode               ScanMode
	Timeout            time.Duration
	SideEffects        []SideEffect
	Sensitivity        Sensitivity
	ObservationKinds   []string
}

func (d Descriptor) Validate() error {
	if d.ID == "" || d.Version == "" || len(d.ComponentKinds) == 0 || len(d.Required) == 0 || d.Timeout <= 0 || len(d.ObservationKinds) == 0 {
		return fmt.Errorf("invalid probe descriptor %q", d.ID)
	}
	return nil
}

type Probe interface {
	Descriptor() Descriptor
	Collect(context.Context, capabilities.Set) error
}
type Catalog struct{ probes []Probe }

func NewCatalog(items ...Probe) (Catalog, error) {
	seen := map[string]bool{}
	for _, p := range items {
		d := p.Descriptor()
		if e := d.Validate(); e != nil {
			return Catalog{}, e
		}
		if seen[d.ID] {
			return Catalog{}, fmt.Errorf("duplicate probe %q", d.ID)
		}
		seen[d.ID] = true
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Descriptor().ID < items[j].Descriptor().ID })
	return Catalog{items}, nil
}
func (c Catalog) Probes() []Probe { return append([]Probe(nil), c.probes...) }

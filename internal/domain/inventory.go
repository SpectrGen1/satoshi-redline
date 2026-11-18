package domain

import "time"

// Inventory describes components and dependency edges for one scan.
type Inventory struct {
	Components []Component
	Edges      []DependencyEdge
}

// BuildInventory constructs configured inventory from enabled bitcoin nodes and electrum backends.
// bitcoinNodes and electrumBackends must already be filtered to enabled entries.
func BuildInventory(bitcoinNodes []InventoryBitcoin, electrumBackends []InventoryElectrum, now time.Time) Inventory {
	inv := Inventory{Components: []Component{SystemLocal()}}
	for _, node := range bitcoinNodes {
		alias := node.DisplayAlias
		if alias == "" {
			alias = node.ID
		}
		inv.Components = append(inv.Components, Component{
			ID: node.ID, Kind: ComponentBitcoinNode, ImplementationFamily: ImplementationBitcoin,
			DisplayAlias: alias, Provenance: ProvenanceConfiguration, State: InventoryConfigured,
		})
	}
	for _, backend := range electrumBackends {
		alias := backend.DisplayAlias
		if alias == "" {
			alias = backend.ID
		}
		inv.Components = append(inv.Components, Component{
			ID: backend.ID, Kind: ComponentElectrumBackend, ImplementationFamily: ImplementationElectrum,
			DisplayAlias: alias, Provenance: ProvenanceConfiguration, State: InventoryConfigured,
		})
		inv.Edges = append(inv.Edges, DependencyEdge{
			ID:                     backend.ID + "-uses-chain-from-" + backend.BitcoinComponentID,
			SourceComponentID:      backend.ID,
			DestinationComponentID: backend.BitcoinComponentID,
			Relationship:           UsesChainFrom,
			Provenance:             ProvenanceConfiguration,
			Confidence:             ConfidenceHigh,
			RecordedAt:             now,
		})
	}
	return inv
}

type InventoryBitcoin struct {
	ID, DisplayAlias string
}

type InventoryElectrum struct {
	ID, DisplayAlias, BitcoinComponentID string
}

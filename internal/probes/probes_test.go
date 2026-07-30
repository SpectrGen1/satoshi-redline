package probes

import (
	"context"
	"testing"
	"time"

	"github.com/satoshi-redline/redline/internal/capabilities"
	"github.com/satoshi-redline/redline/internal/capabilities/electrum"
	"github.com/satoshi-redline/redline/internal/domain"
)

func TestCompiledCatalog(t *testing.T) {
	c, e := CompiledCatalog()
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Probes()) != 3 {
		t.Fatalf("got %d probes", len(c.Probes()))
	}
	if _, e = NewCatalog(BitcoinChainState{}, BitcoinChainState{}); e == nil {
		t.Fatal("duplicate accepted")
	}
}

type fakeElectrum struct {
	result electrum.Result
	reason domain.UnknownReasonCode
}

func (f fakeElectrum) Collect(context.Context) (electrum.Result, domain.UnknownReasonCode) {
	return f.result, f.reason
}

func TestElectrumProbeEmitsOnlyDeclaredNormalizedValues(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	probe := ElectrumProtocolState{SideEffect: LocalConnect}
	result := probe.Collect(context.Background(), capabilities.Set{Electrum: fakeElectrum{result: electrum.Result{
		Transport: electrum.Transport{Reached: true, Kind: domain.ElectrumTransportTCP, TLSVerification: domain.TLSNotApplicable, ProtocolNegotiation: domain.ProtocolAccepted, Category: "valid_protocol_response"},
		Identity:  &electrum.Identity{Family: domain.ElectrumElectrs, ProtocolVersion: "1.4.3"},
		Tip:       &electrum.Tip{Height: 10, BlockHash: "00" + "11111111111111111111111111111111111111111111111111111111111111", HeaderTime: now},
	}}}, "scan", "execution", now)
	if result.Reason != "" || len(result.Values) != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	declared := map[string]bool{}
	for _, kind := range probe.Descriptor().ObservationKinds {
		declared[kind] = true
	}
	for _, observation := range result.Values {
		if !declared[observation.Kind] {
			t.Fatalf("undeclared observation: %s", observation.Kind)
		}
		if err := observation.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestElectrumProbePreservesSafeTransportOnProtocolFailure(t *testing.T) {
	probe := ElectrumProtocolState{}
	result := probe.Collect(context.Background(), capabilities.Set{Electrum: fakeElectrum{
		result: electrum.Result{Transport: electrum.Transport{
			Reached: true, Kind: domain.ElectrumTransportTCP, TLSVerification: domain.TLSNotApplicable,
			ProtocolNegotiation: domain.ProtocolUnknown, Category: string(domain.InvalidResponse),
		}},
		reason: domain.InvalidResponse,
	}}, "scan", "execution", time.Now().UTC())
	if result.Reason != domain.InvalidResponse || len(result.Values) != 1 || result.Values[0].Kind != "electrum.protocol.transport" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

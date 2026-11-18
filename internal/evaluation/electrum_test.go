package evaluation

import (
	"testing"
	"time"

	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
)

const testBlockHash = "0011111111111111111111111111111111111111111111111111111111111111"

func electrumObservation(id, kind string, at time.Time, value domain.ObservationValue) *domain.Observation {
	return &domain.Observation{
		ID: id, ScanID: "scan", ProbeExecutionID: "execution", ComponentID: "electrum.electrs",
		Kind: kind, SchemaVersion: "v1", ObservedAt: at, CapabilityID: "electrum.protocol.read/v1",
		ProviderID: "test", Sensitivity: "local_sensitive", Value: value,
	}
}

func bitcoinObservation(id, kind string, at time.Time, value domain.ObservationValue) *domain.Observation {
	return &domain.Observation{
		ID: id, ScanID: "scan", ProbeExecutionID: "execution", ComponentID: "bitcoin.knots",
		Kind: kind, SchemaVersion: "v1", ObservedAt: at, CapabilityID: "bitcoin.rpc.read/v1",
		ProviderID: "test", Sensitivity: "none", Value: value,
	}
}

func completeElectrumEvidence(now time.Time) ElectrumEvidence {
	return ElectrumEvidence{
		Transport: electrumObservation("transport", "electrum.protocol.transport", now, domain.ElectrumProtocolTransport{
			Reached: true, Transport: domain.ElectrumTransportTCP, TLSVerification: domain.TLSNotApplicable,
			ProtocolNegotiation: domain.ProtocolAccepted, Category: "valid_protocol_response",
		}),
		Identity: electrumObservation("identity", "electrum.server.identity", now, domain.ElectrumServerIdentity{
			ImplementationFamily: domain.ElectrumElectrs, ProtocolVersion: "1.4.3",
		}),
		Tip: electrumObservation("tip", "electrum.chain.tip", now, domain.ElectrumChainTip{
			Height: 100, BlockHash: testBlockHash, HeaderTime: now,
		}),
		Bitcoin: bitcoinObservation("bitcoin", "bitcoin.chain.state", now, domain.BitcoinChainState{
			Blocks: 100, Headers: 100, VerificationProgress: 1, TipTime: now,
		}),
		BitcoinHash: bitcoinObservation("hash", "bitcoin.chain.block-hash", now, domain.BitcoinBlockHash{
			Height: 100, BlockHash: testBlockHash,
		}),
	}
}

func findingByCode(t *testing.T, findings []domain.Finding, code string) domain.Finding {
	t.Helper()
	for _, finding := range findings {
		if finding.Code == code {
			return finding
		}
	}
	t.Fatalf("finding %s missing", code)
	return domain.Finding{}
}

func TestElectrumHealthyEvidencePassesAllFindings(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	findings := Electrum("scan", "electrum.electrs", completeElectrumEvidence(now), app.DefaultConfig().ElectrumThresholds, now)
	if len(findings) != 5 {
		t.Fatalf("got %d findings", len(findings))
	}
	for _, finding := range findings {
		if finding.Status != domain.Pass || len(finding.Evidence) == 0 {
			t.Fatalf("finding is not supported pass: %+v", finding)
		}
		if err := finding.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestElectrumTransportAndCompatibilityRemainDistinct(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	config := app.DefaultConfig().ElectrumThresholds
	tests := []struct {
		name          string
		transport     domain.ElectrumProtocolTransport
		limitation    domain.UnknownReasonCode
		reachability  domain.Status
		compatibility domain.Status
	}{
		{"refused", domain.ElectrumProtocolTransport{Transport: domain.ElectrumTransportTCP, TLSVerification: domain.TLSNotApplicable, ProtocolNegotiation: domain.ProtocolUnknown, Category: string(domain.ConnectionRefused)}, domain.ConnectionRefused, domain.Failure, domain.Unknown},
		{"malformed reached", domain.ElectrumProtocolTransport{Reached: true, Transport: domain.ElectrumTransportTCP, TLSVerification: domain.TLSNotApplicable, ProtocolNegotiation: domain.ProtocolUnknown, Category: string(domain.InvalidResponse)}, domain.InvalidResponse, domain.Pass, domain.Unknown},
		{"TLS verification failed", domain.ElectrumProtocolTransport{Reached: true, Transport: domain.ElectrumTransportTLS, TLSVerification: domain.TLSVerificationFailed, ProtocolNegotiation: domain.ProtocolUnknown, Category: string(domain.TLSFailure)}, domain.TLSFailure, domain.Pass, domain.Unknown},
		{"unsupported protocol", domain.ElectrumProtocolTransport{Reached: true, Transport: domain.ElectrumTransportTCP, TLSVerification: domain.TLSNotApplicable, ProtocolNegotiation: domain.ProtocolUnsupported, Category: string(domain.UnsupportedProtocol)}, domain.UnsupportedProtocol, domain.Pass, domain.Failure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := ElectrumEvidence{
				Transport:  electrumObservation("transport", "electrum.protocol.transport", now, test.transport),
				Limitation: test.limitation,
			}
			findings := Electrum("scan", "electrum.electrs", evidence, config, now)
			if got := findingByCode(t, findings, "electrum.protocol.reachable"); got.Status != test.reachability {
				t.Fatalf("reachability=%s", got.Status)
			}
			if got := findingByCode(t, findings, "electrum.protocol.compatible"); got.Status != test.compatibility {
				t.Fatalf("compatibility=%s", got.Status)
			}
		})
	}
}

func TestElectrumTipFreshnessBoundaries(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	config := app.DefaultConfig().ElectrumThresholds
	tests := []struct {
		name   string
		tipAge time.Duration
		want   domain.Status
	}{
		{"fresh", 0, domain.Pass},
		{"warning boundary", time.Duration(config.TipWarningAgeSeconds+1) * time.Second, domain.Warning},
		{"failure boundary", time.Duration(config.TipFailureAgeSeconds+1) * time.Second, domain.Failure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := completeElectrumEvidence(now)
			value := evidence.Tip.Value.(domain.ElectrumChainTip)
			value.HeaderTime = now.Add(-test.tipAge)
			evidence.Tip.Value = value
			got := findingByCode(t, Electrum("scan", "electrum.electrs", evidence, config, now), "electrum.chain.tip-freshness")
			if got.Status != test.want {
				t.Fatalf("got %s", got.Status)
			}
		})
	}
	evidence := completeElectrumEvidence(now)
	value := evidence.Tip.Value.(domain.ElectrumChainTip)
	value.HeaderTime = now.Add(time.Duration(config.MaxFutureSkewSeconds+1) * time.Second)
	evidence.Tip.Value = value
	if got := findingByCode(t, Electrum("scan", "electrum.electrs", evidence, config, now), "electrum.chain.tip-freshness"); got.Status != domain.Unknown || got.Reasons[0].Code != domain.TemporallyInconsistent {
		t.Fatalf("future tip: %+v", got)
	}
	evidence = completeElectrumEvidence(now)
	evidence.Tip.ObservedAt = now.Add(-time.Duration(config.MaxObservationAgeSeconds+1) * time.Second)
	if got := findingByCode(t, Electrum("scan", "electrum.electrs", evidence, config, now), "electrum.chain.tip-freshness"); got.Status != domain.Unknown || got.Reasons[0].Code != domain.StaleEvidence {
		t.Fatalf("stale observation: %+v", got)
	}
}

func TestElectrumLagBoundariesAndUnknowns(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	config := app.DefaultConfig().ElectrumThresholds
	tests := []struct {
		name string
		lag  int64
		want domain.Status
	}{
		{"same height", 0, domain.Pass},
		{"warning boundary", config.LagWarningBlocks, domain.Warning},
		{"failure boundary", config.LagFailureBlocks, domain.Failure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := completeElectrumEvidence(now)
			bitcoin := evidence.Bitcoin.Value.(domain.BitcoinChainState)
			bitcoin.Blocks = 100 + test.lag
			bitcoin.Headers = bitcoin.Blocks
			evidence.Bitcoin.Value = bitcoin
			got := findingByCode(t, Electrum("scan", "electrum.electrs", evidence, config, now), "electrum.chain.sync-lag")
			if got.Status != test.want || (got.Status != domain.Unknown && len(got.Evidence) != 2) {
				t.Fatalf("got %+v", got)
			}
		})
	}
	evidence := completeElectrumEvidence(now)
	evidence.Bitcoin = nil
	if got := findingByCode(t, Electrum("scan", "electrum.electrs", evidence, config, now), "electrum.chain.sync-lag"); got.Status != domain.Unknown || got.MissingEvidence[0].ObservationKind != "bitcoin.chain.state" {
		t.Fatalf("missing Bitcoin evidence: %+v", got)
	}
	evidence = completeElectrumEvidence(now)
	evidence.Bitcoin.ObservedAt = now.Add(-time.Duration(config.MaxObservationSkewSeconds+1) * time.Second)
	if got := findingByCode(t, Electrum("scan", "electrum.electrs", evidence, config, now), "electrum.chain.sync-lag"); got.Status != domain.Unknown {
		t.Fatalf("skew accepted: %+v", got)
	}
}

func TestElectrumChainMatchRequiresHashEvidence(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	config := app.DefaultConfig().ElectrumThresholds
	evidence := completeElectrumEvidence(now)
	if got := findingByCode(t, Electrum("scan", "electrum.electrs", evidence, config, now), "electrum.chain.matches-bitcoin"); got.Status != domain.Pass || len(got.Evidence) != 2 {
		t.Fatalf("match: %+v", got)
	}
	hash := evidence.BitcoinHash.Value.(domain.BitcoinBlockHash)
	hash.BlockHash = "ff11111111111111111111111111111111111111111111111111111111111111"
	evidence.BitcoinHash.Value = hash
	if got := findingByCode(t, Electrum("scan", "electrum.electrs", evidence, config, now), "electrum.chain.matches-bitcoin"); got.Status != domain.Failure {
		t.Fatalf("mismatch: %+v", got)
	}
	evidence.BitcoinHash = nil
	if got := findingByCode(t, Electrum("scan", "electrum.electrs", evidence, config, now), "electrum.chain.matches-bitcoin"); got.Status != domain.Unknown || got.MissingEvidence[0].ObservationKind != "bitcoin.chain.block-hash" {
		t.Fatalf("missing hash: %+v", got)
	}
}

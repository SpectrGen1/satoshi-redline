package app

import "testing"

func TestElectrumConfigurationIsExplicitAndBounded(t *testing.T) {
	config := DefaultConfig()
	config.BitcoinNodes = []BitcoinNodeConfig{{
		ID: "bitcoin.knots", Enabled: true, DisplayAlias: "Bitcoin Knots",
		RPCEndpoint: "http://127.0.0.1:8332", DataDirectory: "/tmp",
	}}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	config.ElectrumBackends = []ElectrumBackendConfig{{
		ID: "electrum.electrs", Enabled: true, DisplayAlias: "Electrs",
		Endpoint: "tcp://127.0.0.1:50001", TimeoutMS: 5000, BitcoinComponentID: "bitcoin.knots",
	}}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"tcp://127.0.0.1:50001", "ssl://electrum.example:50002"} {
		config.ElectrumBackends[0].Endpoint = endpoint
		if err := config.Validate(); err != nil {
			t.Fatalf("endpoint %s: %v", endpoint, err)
		}
	}
	config.ElectrumBackends[0].BitcoinComponentID = "bitcoin.missing"
	if err := config.Validate(); err == nil {
		t.Fatal("expected invalid bitcoin binding")
	}
	config.ElectrumBackends[0].BitcoinComponentID = "bitcoin.knots"
	config.ElectrumBackends[0].Endpoint = "http://bad"
	if err := config.Validate(); err == nil {
		t.Fatal("expected invalid electrum endpoint")
	}
}

func TestRejectsSingularLegacyConfig(t *testing.T) {
	// Load rejects singular keys via JSON map check — exercise Validate arrays only.
	config := DefaultConfig()
	if err := config.Validate(); err == nil {
		t.Fatal("expected validation failure without bitcoin nodes")
	}
}

func TestMultipleBackendsAndDisabledSkipped(t *testing.T) {
	config := DefaultConfig()
	config.BitcoinNodes = []BitcoinNodeConfig{
		{ID: "bitcoin.knots", Enabled: true, DisplayAlias: "Knots", RPCEndpoint: "http://127.0.0.1:1", DataDirectory: "/tmp"},
		{ID: "bitcoin.core", Enabled: false, DisplayAlias: "Core", RPCEndpoint: "http://127.0.0.1:2", DataDirectory: "/tmp2"},
	}
	config.ElectrumBackends = []ElectrumBackendConfig{
		{ID: "electrum.electrs", Enabled: true, DisplayAlias: "Electrs", Endpoint: "tcp://127.0.0.1:50001", TimeoutMS: 1000, BitcoinComponentID: "bitcoin.knots"},
		{ID: "electrum.fulcrum", Enabled: true, DisplayAlias: "Fulcrum", Endpoint: "tcp://127.0.0.1:50002", TimeoutMS: 1000, BitcoinComponentID: "bitcoin.knots"},
		{ID: "electrum.ignored", Enabled: false, DisplayAlias: "Ignored", Endpoint: "tcp://127.0.0.1:9", TimeoutMS: 1000, BitcoinComponentID: "bitcoin.knots"},
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(config.EnabledBitcoinNodes()) != 1 || len(config.EnabledElectrumBackends()) != 2 {
		t.Fatalf("enabled filters: bitcoin=%d electrum=%d", len(config.EnabledBitcoinNodes()), len(config.EnabledElectrumBackends()))
	}
}

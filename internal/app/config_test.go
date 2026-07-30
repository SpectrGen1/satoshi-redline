package app

import "testing"

func TestElectrumConfigurationIsExplicitAndBounded(t *testing.T) {
	config := DefaultConfig()
	if config.Electrum.Enabled {
		t.Fatal("Electrum enabled by default")
	}
	config.Electrum.Enabled = true
	for _, endpoint := range []string{"", "example:50001", "http://example:50001", "tcp://user:secret@example:50001", "tcp://example:50001/path", "tcp://example:50001?x=1", "tcp://example:50001#x"} {
		config.Electrum.Endpoint = endpoint
		config.Electrum.BitcoinComponentID = "bitcoin.primary"
		if err := config.Validate(); err == nil {
			t.Fatalf("invalid endpoint accepted: %q", endpoint)
		}
	}
	for _, endpoint := range []string{"tcp://127.0.0.1:50001", "ssl://electrum.example:50002"} {
		config.Electrum.Endpoint = endpoint
		config.Electrum.BitcoinComponentID = "bitcoin.primary"
		if err := config.Validate(); err != nil {
			t.Fatalf("valid endpoint rejected: %v", err)
		}
	}
	config.Electrum.BitcoinComponentID = ""
	if err := config.Validate(); err == nil {
		t.Fatal("missing dependency binding accepted")
	}
}

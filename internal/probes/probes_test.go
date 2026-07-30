package probes

import "testing"

func TestCompiledCatalog(t *testing.T) {
	c, e := CompiledCatalog()
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Probes()) != 2 {
		t.Fatalf("got %d probes", len(c.Probes()))
	}
	if _, e = NewCatalog(BitcoinChainState{}, BitcoinChainState{}); e == nil {
		t.Fatal("duplicate accepted")
	}
}

package main

import (
	"strings"
	"testing"
)

func TestRootGuard(t *testing.T) {
	if e := run("unused", func() int { return 0 }); e == nil || !strings.Contains(e.Error(), "refuses to run as root") {
		t.Fatalf("got %v", e)
	}
}

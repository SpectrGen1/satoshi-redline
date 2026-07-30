package filesystem

import (
	"context"
	"github.com/satoshi-redline/redline/internal/domain"
	"math"
	"syscall"
	"testing"
)

func TestCancellationBeforeCollection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, r := Stat(ctx, "/"); r != domain.Timeout {
		t.Fatalf("got %s", r)
	}
}
func TestMissingPath(t *testing.T) {
	if _, r := Stat(context.Background(), "/definitely/not/redline"); r == "" {
		t.Fatal("missing path accepted")
	}
}
func TestPermissionAndOverflowClassification(t *testing.T) {
	permission := func(string, *syscall.Statfs_t) error { return syscall.EACCES }
	if _, r := statWith(context.Background(), "/x", permission); r != domain.PermissionDenied {
		t.Fatalf("got %s", r)
	}
	overflow := func(_ string, s *syscall.Statfs_t) error {
		s.Bsize = 4096
		s.Blocks = math.MaxUint64
		s.Bavail = 1
		return nil
	}
	if _, r := statWith(context.Background(), "/x", overflow); r != domain.InvalidResponse {
		t.Fatalf("got %s", r)
	}
}

package filesystem

import (
	"context"
	"github.com/satoshi-redline/redline/internal/domain"
	"math"
	"os"
	"syscall"
)

type Capacity struct {
	Total, Available       int64
	ReadOnly, MountPresent bool
}

// Stat is scoped to the configured path and never lists directory contents.
func Stat(ctx context.Context, path string) (Capacity, domain.UnknownReasonCode) {
	return statWith(ctx, path, syscall.Statfs)
}
func statWith(ctx context.Context, path string, statfs func(string, *syscall.Statfs_t) error) (Capacity, domain.UnknownReasonCode) {
	if path == "" {
		return Capacity{}, domain.NotConfigured
	}
	select {
	case <-ctx.Done():
		return Capacity{}, domain.Timeout
	default:
	}
	var s syscall.Statfs_t
	if e := statfs(path, &s); e != nil {
		if os.IsPermission(e) {
			return Capacity{}, domain.PermissionDenied
		}
		return Capacity{}, domain.RequiredObservationMissing
	}
	if s.Bsize <= 0 || uint64(s.Blocks) > uint64(math.MaxInt64/int64(s.Bsize)) || uint64(s.Bavail) > uint64(math.MaxInt64/int64(s.Bsize)) {
		return Capacity{}, domain.InvalidResponse
	}
	// Linux statfs ST_RDONLY is bit 0. This capability is currently Linux-scoped.
	return Capacity{int64(s.Blocks) * int64(s.Bsize), int64(s.Bavail) * int64(s.Bsize), s.Flags&1 != 0, true}, ""
}

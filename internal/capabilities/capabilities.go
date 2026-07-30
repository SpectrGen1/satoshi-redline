// Package capabilities defines narrow component-scoped access contracts.
package capabilities

import (
	"context"
	"github.com/satoshi-redline/redline/internal/capabilities/bitcoinrpc"
	"github.com/satoshi-redline/redline/internal/domain"
	"time"
)

type ID string

const (
	BitcoinRPCRead ID = "bitcoin.rpc.read/v1"
	FilesystemStat ID = "filesystem.stat/v1"
	ClockRead      ID = "clock.read/v1"
)

type BitcoinRPCReadAccess interface {
	Chain(context.Context) (bitcoinrpc.ChainInfo, domain.UnknownReason, bool)
}
type FilesystemStatAccess interface {
	Stat(context.Context) (FilesystemCapacity, domain.UnknownReason)
}
type FilesystemCapacity struct {
	Total, Available       int64
	ReadOnly, MountPresent bool
}
type Clock interface{ Now() time.Time }
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Set struct {
	Bitcoin    BitcoinRPCReadAccess
	Filesystem FilesystemStatAccess
	Clock      Clock
}

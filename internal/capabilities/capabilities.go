// Package capabilities defines narrow component-scoped access contracts.
package capabilities

import (
	"context"
	"github.com/satoshi-redline/redline/internal/capabilities/bitcoinrpc"
	"github.com/satoshi-redline/redline/internal/capabilities/electrum"
	"github.com/satoshi-redline/redline/internal/domain"
	"time"
)

type ID string

const (
	BitcoinRPCRead ID = "bitcoin.rpc.read/v1"
	FilesystemStat ID = "filesystem.stat/v1"
	ClockRead      ID = "clock.read/v1"
	ElectrumRead   ID = "electrum.protocol.read/v1"
)

type BitcoinRPCReadAccess interface {
	Chain(context.Context) (bitcoinrpc.ChainInfo, domain.UnknownReasonCode, bool)
}
type FilesystemStatAccess interface {
	Stat(context.Context) (FilesystemCapacity, domain.UnknownReasonCode)
}
type ElectrumReadAccess interface {
	Collect(context.Context) (electrum.Result, domain.UnknownReasonCode)
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
	Electrum   ElectrumReadAccess
}

func (s Set) Scope(required, optional []ID) Set {
	var out Set
	for _, id := range append(append([]ID{}, required...), optional...) {
		switch id {
		case BitcoinRPCRead:
			out.Bitcoin = s.Bitcoin
		case FilesystemStat:
			out.Filesystem = s.Filesystem
		case ClockRead:
			out.Clock = s.Clock
		case ElectrumRead:
			out.Electrum = s.Electrum
		}
	}
	return out
}

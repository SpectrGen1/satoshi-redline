# Live Bitcoin smoke test

The live Bitcoin smoke test is an explicit opt-in integration check. It uses the normal Redline scan-and-persist workflow against an already configured Bitcoin Core or Bitcoin Knots node, reloads the normalized evidence from SQLite, exercises the local API projection, and validates report v1.

The test never starts, stops, or reconfigures Bitcoin. It does not print cookie contents or include credentials in diagnostic output. Unhealthy findings do not necessarily mean that the integration failed: a node may be catching up, have a stale tip, or have storage pressure while Redline still collected and represented its evidence correctly.

All four variables are required and have no defaults:

```text
REDLINE_LIVE_TEST=1
REDLINE_LIVE_RPC_ENDPOINT=http://127.0.0.1:PORT
REDLINE_LIVE_COOKIE_FILE=/path/to/active/network/.cookie
REDLINE_LIVE_DATA_DIRECTORY=/path/to/bitcoin/data
```

Run the test explicitly:

```sh
REDLINE_LIVE_TEST=1 \
REDLINE_LIVE_RPC_ENDPOINT="$RPC_ENDPOINT" \
REDLINE_LIVE_COOKIE_FILE="$COOKIE_FILE" \
REDLINE_LIVE_DATA_DIRECTORY="$BITCOIN_DATADIR" \
go test -v ./testlab/livebitcoin
```

With the opt-in variables unset, the ordinary `go test ./...` suite skips this test without contacting or inspecting any Bitcoin node.

## Lab helper

Against [bitcoin-vm-lab](https://github.com/SpectrGen1/bitcoin-vm-lab), prefer the operator scripts (Tier 1 host-side smoke; see [lab-integration.md](lab-integration.md) and [ADR-011](ADRs/ADR-011.md)):

```sh
./scripts/lab/prepare.sh --vm umbrel
./scripts/lab/smoke.sh --bitcoin-only
```

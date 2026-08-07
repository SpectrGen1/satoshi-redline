# Live Electrum smoke test

The live Electrum smoke test is an explicit opt-in integration check for an already running Electrs or Fulcrum backend and its explicitly bound Bitcoin Core or Bitcoin Knots node. Redline does not install, configure, start, or stop either service.

All variables are required and have no defaults:

```text
REDLINE_LIVE_ELECTRUM_TEST=1
REDLINE_LIVE_RPC_ENDPOINT=http://127.0.0.1:PORT
REDLINE_LIVE_COOKIE_FILE=/path/to/active/network/.cookie
REDLINE_LIVE_DATA_DIRECTORY=/path/to/bitcoin/data
REDLINE_LIVE_ELECTRUM_ENDPOINT=tcp://127.0.0.1:PORT
```

Run:

```sh
REDLINE_LIVE_ELECTRUM_TEST=1 \
REDLINE_LIVE_RPC_ENDPOINT="$RPC_ENDPOINT" \
REDLINE_LIVE_COOKIE_FILE="$COOKIE_FILE" \
REDLINE_LIVE_DATA_DIRECTORY="$BITCOIN_DATADIR" \
REDLINE_LIVE_ELECTRUM_ENDPOINT="$ELECTRUM_ENDPOINT" \
go test -v ./testlab/liveelectrum
```

The test uses the normal scan-and-persist service, reloads the evidence graph, validates API and report contracts, and checks privacy boundaries. It never prints credential contents. Unhealthy findings can be an accurate test result and do not by themselves mean the integration failed.

With any opt-in variable unset, the ordinary test suite skips this package without making an Electrum connection.

## Lab helper

Against [bitcoin-vm-lab](https://github.com/SpectrGen1/bitcoin-vm-lab):

```sh
./scripts/lab/prepare.sh --vm umbrel --electrum electrs
./scripts/lab/smoke.sh --electrum-only
```

See [lab-integration.md](lab-integration.md) and [ADR-011](ADRs/ADR-011.md).

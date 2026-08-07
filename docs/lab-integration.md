# Lab integration (bitcoin-vm-lab)

This document is the operator guide for exercising SATOSHI//REDLINE against the sibling [bitcoin-vm-lab](https://github.com/SpectrGen1/bitcoin-vm-lab) repository. Architectural rules are in [ADR-011](ADRs/ADR-011.md) and [ADR-010](ADRs/ADR-010.md).

## Model summary

| Tier | What | How |
| ---- | ---- | --- |
| 0 | Offline engine | `go test ./...` (no lab) |
| 1 | Protocol live smoke | Host `redline` / live tests → guest RPC and Electrum |
| 2 | Package or binary in guest | Same release artifact (or labeled dev binary) on the guest |
| 3 | User publish | Store / channel; lab may re-check the RC |

**One packaging line.** Official packages live under `packaging/` when they exist. There is no lab product fork. The lab is a **consumer** of those artifacts and of protocol endpoints.

**Lab lifecycle stays in `bvml`.** Product tests and binaries do not start, stop, or reconfigure lab VMs. Opt-in scripts under `scripts/lab/` may read `bvml info` and copy cookies or binaries; they do not manage overlays or Bitcoin packages.

## Prerequisites

1. bitcoin-vm-lab installed and storage prepared (see that repo’s `README.md` and `docs/OPERATIONS.md`).
2. A promoted signet checkpoint and a running guest for the platform under test.
3. From the lab repo (examples):

```sh
cd "${BITCOIN_VM_LAB:-$HOME/projects/bitcoin-vm-lab}"
./bin/bvml status
./bin/bvml resume umbrel          # or ubuntu / startos
./bin/bvml index-adapter-setup umbrel
./bin/bvml adapter-setup umbrel   # if Bitcoin app not yet running
./bin/bvml info umbrel
```

Default lab path: `$BITCOIN_VM_LAB` or `$HOME/projects/bitcoin-vm-lab`.

## Tier 1 — host-side protocol smoke (preferred starting point)

### One-shot prepare

From this repository:

```sh
./scripts/lab/prepare.sh --vm umbrel
# options: --vm ubuntu|umbrel|startos
#          --electrum electrs|fulcrum|none   (default electrs)
#          --runtime-dir .lab/runtime/<vm>   (default per VM)
```

`prepare.sh` will:

* resolve guest IPv4 via `bvml info` (or `REDLINE_LAB_IP`)
* start an SSH tunnel when required (all three platforms)
* fetch the signet RPC cookie to a mode-`600` file (StartOS uses `sudo cat` on the package volume)
* create a host-local filesystem probe directory (capacity findings describe the **host**, not the guest datadir)
* write `redline.json`, a sourceable `env` file, and `meta.json` (no cookie contents)

**Platform access caveats**

| VM | RPC | Electrum | Cookie |
| -- | --- | -------- | ------ |
| **umbrel** | Tunnel `127.0.0.1:9332` → guest loopback. LAN `:9332` returns HTTP 403 (`rpcallowip` is docker+loopback only). | Guest IP `:50001`/`:50002` | scp signet `.cookie` under app-data |
| **ubuntu** | Tunnel `38332` → guest loopback RPC | Tunnel `50001`/`50002` | scp `/srv/bitcoin/signet/.cookie` |
| **startos** | Tunnel `18332` → bitcoind LXC `:8332` (discovered at prepare time) | Tunnel `25001`/`25002` → electrs/fulcrum LXC `:50001` | `sudo cat` package volume signet `.cookie` |

Host tunnel ports differ so umbrel, ubuntu, and startos tunnels can coexist.

### Live Go tests

```sh
./scripts/lab/smoke.sh --runtime-dir .lab/runtime/umbrel
# or: ./scripts/lab/smoke.sh --bitcoin-only | --electrum-only
./scripts/lab/smoke-all.sh    # umbrel + ubuntu + startos sequentially
```

Equivalent to setting `REDLINE_LIVE_*` and running `go test` on `./testlab/livebitcoin` and `./testlab/liveelectrum`. With those variables unset, ordinary `go test ./...` stays offline.

### Manual scan / daemon

```sh
./scripts/lab/scan.sh
./scripts/lab/daemon.sh
# uses .lab/runtime/redline.json from prepare
```

### Ubuntu (loopback services)

Ubuntu Knots and indexers listen on **guest loopback**. Prefer either:

1. **SSH tunnel** (host-side Tier 1; `prepare.sh` starts it by default):

```sh
./scripts/lab/prepare.sh --vm ubuntu
./scripts/lab/smoke.sh
./scripts/lab/tunnel.sh --vm ubuntu stop
```

2. **In-guest binary** (Tier 2-style shortcut): `./scripts/lab/sideload-binary.sh --vm ubuntu`

### Interpreting results

* Live smoke **success** means transport auth, graph integrity, API/report contracts, and privacy canaries — not that every finding is `pass`.
* IBD, tip lag, or storage pressure may yield `warning` / `failure` while integration is correct.
* Never commit cookies, credentials, guest IPs, or `.lab/runtime/`.

## Tier 2 — same artifact on the guest

### Dev binary sideload (available now)

Builds `redline` for linux/amd64, copies it and a guest-local config into the VM, and runs a scan. This is a **development shortcut**, not package-store proof.

```sh
./scripts/lab/sideload-binary.sh --vm umbrel
./scripts/lab/sideload-binary.sh --vm ubuntu --electrum fulcrum
```

### Official packages (when `packaging/` exists)

1. Build the **same** package the release pipeline would publish.
2. Install with `./scripts/lab/sideload-package.sh --vm <platform> --artifact <path>` (stub until package formats land) or the platform-native installer.
3. Exercise the packaged daemon/CLI as a user would.
4. Confirm semantic-manifest equivalence with the canonical engine build (ADR-010).

Do not maintain a parallel lab Compose file, lab-only thresholds, or lab-only privacy exceptions.

## Tier 3 — publish

Promote the artifact that passed the intended gate. Lab re-runs on the RC are fine; a rebuilt “lab special” is not a substitute for the published bytes.

## Endpoint map (signet lab)

Confirm with `bvml info <vm>` and guest listeners. Typical values:

| Platform | Bitcoin RPC (from host) | Cookie (inside guest) | Electrs | Fulcrum |
| -------- | ----------------------- | --------------------- | ------- | ------- |
| Ubuntu | Tunnel `127.0.0.1:38332` | `/srv/bitcoin/signet/.cookie` | Tunnel `50001` | Tunnel `50002` |
| Umbrel | Tunnel `127.0.0.1:9332` (not guest LAN IP) | `…/app-data/bitcoin-knots/data/bitcoin/signet/.cookie` | `tcp://<guest-ip>:50001` | `tcp://<guest-ip>:50002` |
| StartOS | Tunnel `127.0.0.1:18332` → bitcoind LXC `:8332` | package volume `…/main/signet/.cookie` (sudo) | Tunnel `25001` → electrs LXC | Tunnel `25002` → fulcrum LXC `:50001` |

Signet cookie path is `$datadir/signet/.cookie`, not `$datadir/.cookie`.

## Boundary rules

1. Product Go code and `go test` packages do not drive `bvml` lifecycle.
2. Live tests remain opt-in via `REDLINE_LIVE_*` (or `scripts/lab/*`).
3. Scripts may read `bvml info` and use SSH/scp; they must not promote, discard, or destroy lab state.
4. Do not log or assert cookie contents.
5. Platform packages must not encode lab VM names as production identity.

## Related docs

* [ADR-011](ADRs/ADR-011.md) — lab as packaging consumer
* [ADR-010](ADRs/ADR-010.md) — platform packages wrap the engine
* [live-bitcoin-smoke.md](live-bitcoin-smoke.md) / [live-electrum-smoke.md](live-electrum-smoke.md) — raw env-var interface for the Go tests
* bitcoin-vm-lab `AGENTS.md` and `docs/OPERATIONS.md` — lab safety and recovery

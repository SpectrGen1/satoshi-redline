# Lab operator scripts

Opt-in helpers for Tier 1 / Tier 2 work against [bitcoin-vm-lab](https://github.com/SpectrGen1/bitcoin-vm-lab). Full documentation: [docs/lab-integration.md](../../docs/lab-integration.md). Architecture: [ADR-011](../../docs/ADRs/ADR-011.md).

| Script | Purpose |
| ------ | ------- |
| `prepare.sh` | Per-VM tunnel, cookie, `.lab/runtime/<vm>/{env,redline.json,meta.json}` |
| `smoke.sh` | `go test` livebitcoin / liveelectrum |
| `smoke-all.sh` | Sequential Tier 1 on umbrel + ubuntu + startos |
| `scan.sh` | One-shot `redline scan --json` |
| `daemon.sh` | Run `redlined` against runtime config |
| `tunnel.sh` | SSH forwards for umbrel / ubuntu / startos |
| `tunnel-ubuntu.sh` | Wrapper for `tunnel.sh --vm ubuntu` |
| `sideload-binary.sh` | Dev binary on guest (not package proof) |
| `sideload-package.sh` | Tier 2 package install (stub until packaging exists) |

**Access notes**

| VM | RPC path | Electrum path | SSH key |
| -- | -------- | ------------- | ------- |
| umbrel | tunnel `127.0.0.1:9332` → guest loopback (LAN `:9332` is 403) | guest IP `:50001`/`:50002` | `~/.config/bitcoin-vm-lab/umbrel_ed25519` |
| ubuntu | tunnel `38332`/`50001`/`50002` → guest loopback | same tunnel | `UBUNTU_SSH_PRIVATE_KEY` or common `~/.ssh/id_*` |
| startos | tunnel `18332` → bitcoind LXC `:8332`; cookie via `sudo cat` | tunnel `25001`/`25002` → electrs/fulcrum LXC | `~/.config/bitcoin-vm-lab/startos_ed25519` |

These scripts **do not** start, stop, promote, or discard lab VMs. Bring the lab up with `bvml` first.

```sh
# One platform
./scripts/lab/prepare.sh --vm umbrel
./scripts/lab/smoke.sh --runtime-dir .lab/runtime/umbrel

# All three consumers
./scripts/lab/smoke-all.sh
```

Runtime state lives in `.lab/runtime/<vm>/` (gitignored). Never commit cookies or `env` files.

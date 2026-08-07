# Packaging

Official SATOSHI//REDLINE platform packages will live here when implemented:

* generic Linux (for example Debian/Ubuntu units and packages)
* Docker / OCI
* Umbrel application wrapper
* StartOS 0.4 package

Rules (see [ADR-010](../docs/ADRs/ADR-010.md) and [ADR-011](../docs/ADRs/ADR-011.md)):

1. **Build once, package many.** Canonical engine binaries are attached to each wrapper; packages do not fork diagnostic semantics.
2. **No lab package fork.** bitcoin-vm-lab consumes these same artifacts (or a labeled dev binary sideload). Do not add `packaging/lab` with different topology, thresholds, or privacy behavior.
3. **Tier 2 verification.** When a package format lands, install that artifact into the matching lab guest via `scripts/lab/sideload-package.sh` or the platform-native installer, then scan as packaged.

Until wrappers exist, operators use Tier 1 host-side smoke and optional `scripts/lab/sideload-binary.sh` for in-guest iteration. See [docs/lab-integration.md](../docs/lab-integration.md).

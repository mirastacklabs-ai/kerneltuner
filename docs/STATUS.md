# Status

## v1.0.0: released 2026-11-17
- Release: https://github.com/mirastacklabs-ai/kerneltuner/releases/tag/v1.0.0 (commit `db86051`)
- Contents:
  - 137 rules and 148 sources, every source with a verbatim quote checked against the live page
  - 398 golden tests
  - 8 e2e scenarios
- Verified after publishing:
  - sha256 checksums
  - cosign keyless signature on `checksums.txt`
  - SLSA v1 build provenance (`gh attestation verify`)
  - SPDX SBOMs
  - the deb installs and runs
  - `install.sh` resolves `latest` and verifies the signature and sha256
  - `scan` on a real kernel reports 0 warnings
- `v1.0.0-rc.1` is kept as the rehearsal pre-release.

## Post-v1 (tracked as issues, label `post-v1`)
- #2: cgroup line selection for unit names
- #3: `O_NONBLOCK` on managed-path opens
- #4: preserve owner and SELinux label on atomic replace
- #5: review the areas not reached in the v1.0.0 review
- #6: `--etc-root` / `--var-root`
- Larger ideas from `docs/COVERAGE.md`:
  - a benchmark harness
  - config readers for nginx, PostgreSQL and Kafka
  - real-host fixture contributions, especially GPU hosts

## Release process (what worked)
1. Run `make build test lint validate test-nonroot e2e`. All must pass.
2. Push `main` and wait for CI to go green.
3. Tag `vX.Y.Z-rc.N` and approve the `release` environment.
4. Verify the rc: checksums, cosign, attestation, SBOM, deb/rpm and `install.sh`.
5. Tag `vX.Y.Z` on the same commit and approve. `GORELEASER_CURRENT_TAG` makes the run publish the tag that triggered it.

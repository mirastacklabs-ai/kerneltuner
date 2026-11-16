# Status — 2026-11-16 (v1.0.0 release candidate on `main`)

## Done (all on `main`, pushed)
- **Engine:**
  - facts, including accelerators, RDMA, mounts, kernel config, and per-process limits/OOM
  - workload detection: 34 apps, plus `--workload`
  - metrics, rules engine, advisor with reconciliation and conflicts
  - renderers with citations
  - commands: `scan`, `explain`, `generate`, `sources [--check]`, `validate`, `capture`, `agent`, `status`, `apply`, `rollback`
- **Knowledge base:** 137 rules and 148 sources. Every source carries a verbatim quote verified live by `sources --check`, and there are 398 golden tests.
- **Docs:** README, COVERAGE, RULE-AUTHORING, CONTRIBUTING, SECURITY, NOTICE, CHANGELOG.
- **Tests:** `make e2e` covers 7 scenarios and all pass. CI is green.
- **Security reviews:** two passes found 0 P0, 5 P1, 12 P2 and 21 P3. Every P1, P2 and P3 is fixed (Q18 deferred), each with a regression test.
- **Workflows:**
  - least-privilege permissions on every job
  - drift job fails properly
  - syft and cosign installer
  - build provenance
  - dependabot cooldown
- **Repo settings:**
  - `release` environment, with the maintainer as required reviewer and `v*` tags only
  - tag ruleset `protect-release-tags`
- **Approval:** the maintainer approved the rc and final release runs (2026-11-16).

## Remaining to v1.0.0
1. **Follow-ups from the security-fix agent:**
   - `release.yml`: pin `sigstore/cosign-installer` to `cosign-release: v2.5.3`, because the goreleaser `signs:` block uses `--output-signature`/`--output-certificate`, which cosign v3 rejects. `id-token: write` is already present.
   - `ci.yml`: the validate step needs `env: KERNELTUNER_FIXTURES: testdata/fixtures`. Without it, validate skips the golden checks.
   - `internal/cli/validate.go`: make a missing fixtures dir an error when golden tests are requested.
   - `docs/RULE-AUTHORING.md` (validate command) and any other doc that runs validate: set `KERNELTUNER_FIXTURES`, or use `--fixtures-dir testdata/fixtures`.
2. **Focused read-only re-review** of the security-fix merge (`ce48fe4`): `internal/apply/safefs.go`, snapshot validation, the reconcile floors/ceilings, `--fixture` safety, and capture redaction.
3. **Rehearsal:** tag `v1.0.0-rc.1`, then approve the `release` environment deployment via the API. Verify:
   - assets
   - `checksums.txt` plus `.sig`/`.pem`
   - provenance (`gh attestation verify`)
   - SBOMs
   - deb/rpm
   - `install.sh` against the rc
4. **Final:** tag `v1.0.0`, approve, verify, then `docker rmi goreleaser/goreleaser:v2.20.2`.

## Housekeeping
- No worktrees, no `dist/`, and no leftover e2e volumes. The Go build cache is in the Docker volume `kerneltuner-gocache`; trim it with `find /go/cache -type f -mmin +60 -delete`.
- Downloaded upstream pages live in the session scratchpad archive. They aren't needed, because `sources --check` refetches.

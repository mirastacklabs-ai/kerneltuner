# Contributing to kerneltuner

Thanks for helping. The most valuable contribution is a rule: a tuning recommendation backed by an official source. Bug reports, fixtures from real hosts and engine fixes are welcome too.

## Code of Conduct

This project follows the [Contributor Covenant 2.1](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). By taking part you agree to uphold it. Report unacceptable behaviour to the maintainers via GitHub Security Advisories or issues; if the report is sensitive, use a private security advisory.

## Development setup

Everything runs in Docker. Do not install Go or any tool on your host. You need a Docker CLI and a runtime (Docker Desktop, Colima or similar). The Go build cache lives in the `kerneltuner-gocache` volume.

| Target | What it does |
|---|---|
| `make build` | Build the static linux binary to `bin/kerneltuner` |
| `make test` | Run `go test -race ./...` |
| `make lint` | Run golangci-lint (pinned image) |
| `make fmt` | Format the code with `gofmt -s` |
| `make tidy` | Tidy `go.mod` and `go.sum` |
| `make vet` | Run `go vet` |
| `make cover` | Run tests with coverage (`coverage.out`) |
| `make release-check` | Validate `.goreleaser.yaml` |
| `make release-snapshot` | Build a local snapshot release into `dist/` (no publish) |

Run `make help` for the current list. To validate the rules and run every golden test:

```sh
docker run --rm -v "$PWD":/src -v kerneltuner-gocache:/go -w /src -e GOCACHE=/go/cache \
  golang:1.27 go run ./cmd/kerneltuner validate
```

CI runs gofmt, `go vet`, `go test -race`, golangci-lint, a build plus `kerneltuner validate`, and `goreleaser check` on every pull request.

Tests must never touch the host's real sysctls, `/sys` or `/proc`. `apply` and `rollback` tests use fixtures or disposable containers only. Read [docs/DESIGN.md](docs/DESIGN.md) before changing the engine; it is the contract for types, the rule schema and ownership.

## Branches and pull requests

1. Fork the repository and branch from `main`. Use a short topic name such as `rules-nats` or `fix-conflict-block`.
2. Keep each pull request to one change: one pack, one rule group, or one fix.
3. Make sure `make test`, `make lint` and `kerneltuner validate` pass.
4. Open a pull request against `main` and describe what changed and why. For a rule, paste the source URL and the verbatim quote.
5. Maintainers review and merge. Expect requests for a quote, a negative test or a smaller scope.

### Commit messages

Use a conventional commit prefix. The release changelog is generated from commit messages and grouped by prefix:

| Prefix | Changelog group |
|---|---|
| `feat:` | Features |
| `fix:` | Bug fixes |
| `perf:` | Performance |
| `docs:` | Documentation |
| anything else (`rules:`, `build:`, `test:`, `chore:`) | Other |

A scope is optional: `feat(apply): ...`, `fix(rules): ...`. Merge commits are left out of the changelog. Write the subject in the imperative, for example `fix: handle empty /proc/swaps`.

### Sign-off and licensing

There is no CLA and no DCO sign-off requirement. Contributions are licensed inbound=outbound: by submitting a pull request you agree that your contribution is licensed under the [Apache License 2.0](LICENSE), the same license as the project, as described in section 5 of that license. Do not submit code or text you do not have the right to license this way, and do not copy text from sources with an incompatible license. Short verbatim quotes from official documentation are fine and are required for citations.

## The rule bar

Rules are the main asset, so the bar is strict. A rule needs:

- an official source (the project's own docs, source tree or shipped files),
- a verbatim quote from that page that supports the claim,
- at least one positive and one negative golden test, using fixtures.

Where the upstream docs give no host-level guidance, we ship notes and no rule. If a threshold is a heuristic, the rationale says so. The full guide, with fixtures, validation, capturing a fixture from a real host and common pitfalls, is [docs/RULE-AUTHORING.md](docs/RULE-AUTHORING.md). Check [docs/COVERAGE.md](docs/COVERAGE.md) for what is covered and what is a known gap.

## Fixtures from real hosts

`kerneltuner capture --fixture` records a redacted snapshot you can contribute. Read the file before sharing it. The steps and the privacy guarantees are in [docs/RULE-AUTHORING.md](docs/RULE-AUTHORING.md#contributing-a-fixture-from-a-real-host).

## Reporting bugs and security issues

Open a GitHub issue for bugs and wrong recommendations; include the output of `kerneltuner version` and, if you can, a `capture` archive you have reviewed. Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md), not in a public issue.

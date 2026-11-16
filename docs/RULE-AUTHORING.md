# Writing rules and packs

A step-by-step guide to adding a rule, or a whole pack, to the knowledge base. The reference material stays in two places and is not repeated here:

- [rules/README.md](../rules/README.md): pack layout, sources registry, conventions per kind, expression helpers, engine behaviour.
- [docs/DESIGN.md](DESIGN.md): the rule schema, the expression environment and the fixture format. It wins over everything else.

Read both once before writing a rule. This guide is the order of work and the pitfalls.

## The bar

A rule is accepted when it has all of these:

1. **An official source.** The URL is the project's own documentation, source tree or shipped files (kernel docs, the vendor's guide, the app's repository). Blog posts, forum answers and third-party mirrors do not count. If the official site cannot be fetched, cite the docs source in the project's repository, as the Kafka and RabbitMQ packs do.
2. **A verbatim quote.** Copy the sentence that supports the rule from the page and put it in the rule's `sources` entry as `{url, section, quote}` (at most 300 characters, plain text). The format and the rules of thumb are in the "Citations" section of [rules/README.md](../rules/README.md). `kerneltuner sources --check` fetches each page and fails if the quote is gone, so a paraphrase breaks the check. If you cannot find text that says what the rule says, there is no rule: do not write one from memory. Today no existing rule carries a quote yet, and `validate` only warns; new rules should include one, and `validate --strict-citations` is the check to run.
3. **At least one positive and one negative golden test.** The positive test uses a fixture where the rule fires (`recommend` or `firing`). The negative test uses a fixture where it does not (`ok` or `not-applicable`). Both assert the status; assert `proposed` too when the rule computes a value.

Also required:

- Every URL in `sources` is registered in the `sources.yaml` of some pack, with `project`, `covers` and `last_reviewed`.
- Where the docs give no number, the rationale says the threshold is a heuristic and why you picked it. Never present an invented number as upstream advice.
- Claims you could not verify go in `NOTES.md` under a "not said by the docs" heading, and no rule is written for them.

## Adding a rule to an existing pack

1. Find the pack: `rules/<area>/<pack>/*.yaml`. Areas are `kernel`, `ai`, `apps`, `antipatterns` and `ecosystem`.
2. Fetch the source page and copy the quote. Add the page to the pack's `sources.yaml` if it is new:

   ```yaml
   - url: https://example.org/docs/tuning.html
     project: example
     covers: what this page is used for
     last_reviewed: 2026-11-16
   ```

3. Write the rule. A minimal sysctl rule with evidence:

   ```yaml
   - id: app.example.somaxconn
     title: Listen backlog for Example
     kind: sysctl
     param: net.core.somaxconn
     domain: network
     applies:
       when: 'hasApp("example")'
     recommend: "max(current, 4096)"
     compare: gte
     evidence:
       - metric: TcpExt.ListenOverflows
         rate_gt: 0
     namespaced: true
     risk: low
     rationale: >-
       Why, in the project's own terms, including kernel-version history.
     verify: "TcpExt.ListenOverflows rate stays 0"
     sources:
       - url: https://example.org/docs/tuning.html
         section: Listen backlog
         quote: >-
           Verbatim sentence copied from the page.   # at most 300 characters
     tests:
       - fixture: example-small-somaxconn
         expect: { status: recommend, proposed: "4096" }
       - fixture: example-stock
         expect: { status: ok }
   ```

   Ids are dotted, unique and stable forever. Pick the `kind` from the list in DESIGN.md; the per-kind conventions are in `rules/README.md`.
4. Make the fixtures (next section), run `validate` and `scan --fixture` (see [Running it](#running-it)).
5. Add any unverified claims to the pack's `NOTES.md`.

## Adding a pack

1. Create `rules/<area>/<pack>/` with a `sources.yaml`, one or more YAML files of rules, and a `NOTES.md`. The embed directive in `rules/embed.go` picks up the whole tree, so there is nothing to register.
2. In `NOTES.md`, record what you fetched and verified (with the date), and what the docs do not say. Follow the structure of an existing pack such as `rules/apps/kafka/NOTES.md`.
3. If the upstream docs contain no host-level guidance, ship `NOTES.md` with no rules. That is a valid result; see `rules/apps/nats/NOTES.md`.
4. Add a row to [docs/COVERAGE.md](COVERAGE.md).
5. If the pack is for an app, add a workload matcher (see [Workload ids](#workload-ids)).

## Fixtures

A fixture is a fake `/proc`, `/sys` and `/etc` tree described in `testdata/fixtures/<name>/fixture.yaml`. Name new ones `<pack>-<variant>`, for example `kafka-tuned`. Never edit a shared fixture such as `base-linux` that other packs use; extend it.

Use `extends` to start from a base fixture and override only what differs:

```yaml
description: Example server with a small somaxconn
synthetic: true
extends: base-linux
files:
  "proc/sys/net/core/somaxconn": "128\n"
  "proc/4200/comm": "example\n"
  "proc/4200/cmdline": "/usr/bin/example\u0000--serve\u0000"
  "proc/4200/cgroup": "0::/system.slice/example.service\n"
  "proc/4200/limits": "Limit                     Soft Limit           Hard Limit           Units\nMax open files            1024                 524288               files     \n"
values: { conntrack.count: 1200 }        # injected gauges
rates: { TcpExt.ListenOverflows: 3.2 }   # injected per-second rates
```

- `files` maps a path under the fake root to its content. Files in the child override files in the parent.
- `values` and `rates` inject metric gauges and per-second rates, so evidence can fire without a live sample.
- Set `synthetic: true` for anything you wrote by hand. Fixtures from `capture --fixture` carry `synthetic: false`.
- Make the negative fixture the smallest change that makes the rule quiet. A fixture that differs from the positive one in only the value under test proves the rule reads that value.
- Keep fixtures small. Only add the files the rule reads.

## Running it

Everything runs in Docker. The Go build cache lives in the `kerneltuner-gocache` volume.

```sh
# Build the binary once (static, linux)
make build

# Validate schema, source registration, fixtures and every golden test.
# Run it in a container (the binary is linux/amd64 or arm64 as built).
docker run --rm -v "$PWD":/src -v kerneltuner-gocache:/go -w /src -e GOCACHE=/go/cache \
  golang:1.27 go run ./cmd/kerneltuner validate

# Run the golden tests only
docker run --rm -v "$PWD":/src -v kerneltuner-gocache:/go -w /src -e GOCACHE=/go/cache \
  golang:1.27 go test ./rules/...

# See what a rule does against a fixture, before writing the golden test
docker run --rm -v "$PWD":/src -v kerneltuner-gocache:/go -w /src -e GOCACHE=/go/cache \
  golang:1.27 go run ./cmd/kerneltuner --rules-dir rules --pack apps/example --json \
  scan --all --fixture example-small-somaxconn
```

`--fixture NAME` is a hidden developer flag on `scan`, `generate` and `apply`. It materializes `testdata/fixtures/NAME` into a temp dir, uses it as `--root` and injects its `values` and `rates`. Run it from inside the repository, because fixtures are found by walking up to `go.mod`. `--rules-dir` replaces the embedded packs and `--pack` limits the run to one pack. Use `explain <rule-id>` to read the rule as a user sees it.

`validate` must pass with no problems before you open a pull request. CI runs it. Run `validate --strict-citations` too: it flags sources without a quote, and the existing rules will still show up there until they are backfilled. Check your quotes against the live page with `sources --check --only <pack>`; this makes outbound HTTP requests and sends no host data.

## Contributing a fixture from a real host

Synthetic fixtures only prove the rule logic. A capture from a real host proves the paths and formats are real, which matters most for GPU, Gaudi and RDMA hosts.

```sh
sudo kerneltuner capture --fixture -o my-host.yaml
```

Run it as root if you can; without root, per-process limits and some NIC data are missing. Without `--fixture`, `capture -o FILE` writes a tar.gz archive for bug reports.

What the capture does and does not read:

- It reads an explicit allowlist of `/proc`, `/sys` and `/etc` paths, nothing else.
- From each command line it keeps the process name and `argv[0..1]` only. Arguments that look like secrets (`pass`, `secret`, `token`, `key`, `auth`, `cred`), email addresses and similar are replaced with `[redacted]`.
- The host name is replaced with `redacted-host` wherever it appears in captured values. `kernel.hostname`, `tcp_fastopen_key`, serial numbers, MAC and IP addresses are never read.
- `/proc/<pid>/environ` is never read, with one exception: the `container=` entry of PID 1. File descriptor targets are not read.
- Remote mount devices are recorded as `[remote]`, and mount credentials and overlay paths are dropped.

Redaction is pattern-based, so check the file before you share it. Read it in full, search for your host name, domain, usernames and internal addresses, and delete anything you do not want public. Open the pull request with the file saved as `testdata/fixtures/<pack>-<variant>/fixture.yaml`, and say what hardware and distribution it came from in `description`. Do not include a capture you are not allowed to publish.

## Workload ids

Rules reference apps by canonical id. `workload.KnownApps()` returns them and `--workload NAME[,NAME...]` accepts them. The 34 ids are:

`vllm sglang tgi triton llama.cpp ollama pytorch cassandra kafka pulsar zookeeper elasticsearch opensearch jvm redis valkey memcached nginx haproxy envoy httpd postgres mariadb mysql clickhouse rabbitmq nats kubelet etcd containerd cilium coredns prometheus fluent-bit`

`jvm` is the fallback for any `java` process that no specific matcher claims.

To add an app matcher:

1. Add an entry to the `matchers` table in `internal/workload/detect.go`:

   ```go
   {App: "example", Comms: []string{"example-server"}, Cmd: []string{"org.example.Main"}},
   ```

   `Comms` matches the process comm or the `argv[0]` basename. `Cmd` matches a substring of the space-joined command line. All non-empty conditions must hold. The table is checked in order and the first match wins, so put a specific JVM app before the generic `jvm` entry, and `mariadb` before `mysql`.
2. Add the id to the list in `TestKnownApps` in `internal/workload/detect_test.go`, and add a detection test with a fake `/proc` under `internal/workload/testdata`.
3. Update the id list in `docs/DESIGN.md` and in this guide.
4. Use `hasApp("example")` or `foreach: app` with `filter: item.Name == "example"` in your rules.

Remember that the comm is truncated to 15 characters by the kernel, and that a containerised app has no systemd unit, so `foreach: app` targets the app name.

## Common pitfalls

These come from [ENGINE-ISSUES.md](ENGINE-ISSUES.md), reported by earlier pack authors. The behaviour is described in `rules/README.md`.

- **String values in `recommend` must be quoted expr strings.** `recommend` is an expression. A bare `recommend: running` is an identifier and fails to compile. Write `recommend: '"running"'` (or `"\"running\""`), and `'"pt"'` for a cmdline value.
- **`runtime` rules without `detect` always say `recommend`.** A `runtime` rule has no `ok` state unless you give it a `detect` expression: true means the advice is needed, false means `ok`. Without `detect`, it fires whenever `applies` holds. The same optional `detect` works on `service` rules. If the advice cannot be checked from facts, say so in the rule, and gate it with `applies.when`.
- **Evidence only raises the score.** A matching `evidence` entry multiplies the score; it never changes the status. A rule with `evidence` still reports `recommend` on a quiet host. To make the status depend on a counter, set `require_evidence: true` (it needs at least one `evidence` entry) or put the condition in `applies.when` or `detect`.
- **Large numbers.** The expression parser rejects integer literals above `9223372036854775807`. Use the `unlimited` constant and `isUnlimited(x)`, not `18446744073709551615`.
- **`foreach` items.** `foreach: accel` also iterates NVSwitch bridges; add `filter: item.Kind == "gpu"` when only GPUs matter. `foreach: cpu` yields int ids and `{item}` is `cpu0`. A test's `expect.target` must use the collapsed name when `foreach_summary` is on.
- **Several rules for one setting.** Rules that share a `(kind, param, target)` get merged in `generate` and `apply`. Give them the same `compare` mode and compatible thresholds, or `generate` will print a conflict.
- **Facts that do not exist.** There is no process environment, no application config parsing, no `ionice` and no per-process CPU affinity. A rule that needs one of these is `runtime` advice, or it is not written. See [What kerneltuner does not do](COVERAGE.md#what-kerneltuner-does-not-do).
- **Kernel config can be unknown.** `kconfig("CONFIG_X")` returns `""` when there is no config file. Test `== "y"`, not `!= "n"`.
- **Container hosts.** Set `namespaced: false` for settings that cannot be changed inside a container; the status becomes `host-only` there.
- **Tuned and bpftune.** A parameter that an active `tuned` profile or a running `bpftune` already manages is reported as `managed`, not `recommend`. Keep `rules/ecosystem/*.yaml` accurate when a profile changes.
- **Unknown YAML fields are an error.** The loader is strict, and its messages name the file and line.

## Before you open the pull request

- `validate` reports no problems and `make test` passes.
- Every rule has the quote, an official source, one positive test and one negative test.
- `sources.yaml` and `NOTES.md` are updated, and a row is updated in [COVERAGE.md](COVERAGE.md).
- Commit messages use the prefixes in [CONTRIBUTING.md](../CONTRIBUTING.md).

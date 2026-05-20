# CI & Testing — knuckle

> The contract between contributors, CI, and the principal-engineer review.
> If something here disagrees with reality, **reality wins** and this doc gets
> fixed.

## Test Pyramid

```
                ┌────────────────────────────┐
                │   VM e2e  (manual / Ghostty)│        just e2e / just vm
                └──────────────┬─────────────┘
                 ┌─────────────┴──────────────┐
                 │  Headless e2e  (dry-run)   │        just headless-test
                 └────────────┬───────────────┘
              ┌───────────────┴───────────────┐
              │  Integration  (network, off-CI)│       go test -tags=integration
              └──────────────┬─────────────────┘
              ┌──────────────┴──────────────────┐
              │  Golden  (internal/ignition)     │       go test ... -update
              └──────────────┬───────────────────┘
        ┌────────────────────┴────────────────────┐
        │  Unit  (internal/**)                     │     go test -race ./...
        └─────────────────────────────────────────┘
```

| Layer       | Runs in CI? | What it covers                                                 |
| ----------- | :---------: | -------------------------------------------------------------- |
| Unit        |     ✅      | All packages, race-clean                                       |
| Golden      |     ✅      | Ignition output stability (regenerate with `-update`)          |
| Headless e2e|     ✅      | `just headless-test` — build + canned JSON + `--dry-run`       |
| Integration |     ❌      | Tagged `//go:build integration`. Real HTTP to GitHub + Flatcar |
| VM e2e      |     ❌      | Requires QEMU + KVM; dev-machine only                          |

## Coverage Gate

`just cover-check` enforces per-package thresholds. Current numbers as of
2026-05-19:

| Package              | Now  | Gate | Aspiration (TEST-PLAN.md) |
| -------------------- | ---- | ---- | ------------------------- |
| `internal/model`     | 100% | 90%  | ≥ 90%                     |
| `internal/iso`       | 100% | 70%  | (n/a)                     |
| `internal/ignition`  |  92% | 85%  | ≥ 90%                     |
| `internal/github`    |  90% | 85%  | (n/a)                     |
| `internal/validate`  |  88% | 85%  | ≥ 95%                     |
| `internal/bakery`    |  84% | 80%  | ≥ 85%                     |
| `internal/probe`     |  81% | 80%  | ≥ 85%                     |
| `internal/runner`    |  81% | 80%  | ≥ 80%                     |
| `internal/wizard`    |  80% | 70%  | ≥ 85%                     |
| `internal/install`   |  76% | 70%  | ≥ 80%                     |
| `internal/headless`  |  75% | 70%  | (n/a)                     |
| `internal/tui`       |  46% | 40%  | ≥ 70%                     |

Gates are set conservatively below current numbers so CI fails on
**regression**, not on aspirational drift. When a package's actual coverage
rises and stays there, raise the gate in `Justfile :: cover-check`.

## CI Workflows

### `.github/workflows/ci.yml`

| Job            | What it does                                                            | Required to merge |
| -------------- | ----------------------------------------------------------------------- | :---------------: |
| `build-test`   | `go mod tidy` (clean), `gofmt`, `go vet`, `go build`, `go test -race`  |        ✅         |
| `lint`         | `golangci-lint run` (v2.11.4 via GHA action)                           |        ✅         |
| `vuln`         | `go tool govulncheck ./...` (version pinned in `go.mod`)               |        ✅         |
| `coverage`     | `just cover-check` + uploads `cover.out` artifact (14-day retention)   |        ✅         |
| `headless-e2e` | `just headless-test` — build + canned JSON config + `--dry-run`        |        ✅         |

**Tool version pinning:** `govulncheck` is pinned in `go.mod` via `go tool`.
`golangci-lint` is pinned in `Justfile::GOLANGCI_LINT_VERSION` (local) and
`ci.yml::golangci-lint-action version` (CI). Bump both together.

Concurrency: per-ref, with `cancel-in-progress: true` — pushes to the same
branch cancel earlier in-flight runs.

Permissions: `contents: read` at workflow scope. Each job specifies its own
needs. `persist-credentials: false` on every `actions/checkout` — keeps the
`GITHUB_TOKEN` out of the working directory.

### `.github/workflows/security.yml`

| Job                 | When                          | What                                          |
| ------------------- | ----------------------------- | --------------------------------------------- |
| `codeql`            | push, PR, weekly cron         | CodeQL Go scan, `security-and-quality` suite  |
| `dependency-review` | PR only                       | Block PRs that introduce high-severity CVEs   |
| `scorecard`         | push to main, weekly cron     | OSSF Scorecard → SARIF upload to code scanning |

Weekly cron is Mondays at 06:37 UTC — odd minute on purpose, avoids the
00/30 GitHub Actions stampede.

### `.github/workflows/release.yml`

Builds the binary + installer ISO on `v*` tags, publishes a GitHub Release
with SHA256 sidecars. See `scripts/build-iso-ci.sh` for the ISO recipe used
in CI (`grub-mkstandalone` path). Local builds use `scripts/build-iso.sh`.

## Local Reproduction

Everything CI does is reachable from `just`:

```bash
just ci          # full pre-push gate
just fmt-check   # mirrors CI gofmt step
just vuln        # govulncheck (installs to $GOBIN)
just cover-check # per-package thresholds
```

If `just ci` passes locally but fails in CI, the gap is one of:
- Go version drift (CI pins `1.26`; bump locally with `go env GOROOT`)
- A network-dependent test running unintentionally (check for missing
  `//go:build integration` tag)
- An untracked file in your checkout that CI doesn't see

## Adding a Test

- Unit tests live next to the code (`foo.go` → `foo_test.go`).
- Fixtures go in the package's local `testdata/` (compiler ignores it).
- Golden files use the `-update` pattern: `go test ./internal/ignition -update`.
  Commit the rewritten `*.golden.json` deliberately; review the diff.
- Don't reach for the network in a unit test. Use `httptest.NewServer` or a
  `SpyRunner` stub. Integration tests that hit real APIs go behind
  `//go:build integration`.

## Adding a CI Job

- Pin the action by version, not `@main`.
- Set `permissions:` at job scope, request the minimum.
- Set `persist-credentials: false` on `actions/checkout`.
- Add it to the matrix in this doc and in `AGENTS.md`'s checklist.

## Roadmap

Tracked in `docs/REVIEW-2026-05-19.md`. Highlights:

- ~~Promote `just headless-test` into a CI job~~ — **done** (2026-05-19,
  `headless-e2e` job in `ci.yml`).
- Land `FuzzHostname`, `FuzzCIDR`, `FuzzSSHKey` and run with `-fuzztime=30s`
  in a nightly job.
- Sign release artifacts with `cosign` and publish an SBOM via `syft`.
- Verify GPG signatures on Flatcar `.DIGESTS.asc` (currently presence-only —
  see `internal/bakery/channels.go` — blocker B1).
- Add runner-routed `Reboot()` method; remove bare `exec.Command("systemctl",
  "reboot")` in `tui.go` and `main.go` — blocker B2.
- Call `validate.DiskPath()` in `headless.Validate()` — blocker B3.
- Raise `tui` coverage by extracting more pure logic into `wizard` / `validate`.
- Add fixture gaps: `lsblk-empty.json`, `lsblk-all-removable.json`,
  `ip_addr-ipv6-only.json`, `bakery-malformed-digests` (from QA review).

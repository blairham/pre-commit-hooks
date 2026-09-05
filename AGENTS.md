# AGENTS.md — pre-commit-hooks

Guidance for AI coding agents (Claude Code, Cursor, Copilot, Codex, OpenCode, …) working in this repository. This is the **cross-tool single source of truth** — `CLAUDE.md` imports it.

## Project Overview

Go-based [pre-commit](https://pre-commit.com) hooks, published for others to consume by `rev`. Each hook is a real Go binary declared with `language: golang`, so pre-commit builds it with `go install ./...` — no shell scripts, no Python.

Three hooks: `check-go-version-sync`, which keeps a module's `go` directive and its governing `.tool-versions` `golang` pin from drifting apart; `check-license-headers`, which requires an SPDX header on every source file; and `check-conflict-markers`, which fails on a committed conflict marker.

## The constraint that shapes this repo

**`go.mod` must stay buildable on an old toolchain, with zero dependencies.**

pre-commit's golang backend installs hooks with `GOTOOLCHAIN=local`, so the module is compiled by whatever Go the *consumer* already has on PATH. If `go.mod` requires a newer Go than they have, the hook fails to install and their commit is blocked.

Consequences, all deliberate:

- The `go` directive is low (`go 1.22.0`) and should only be raised for a real need.
- **No dependencies.** Everything is stdlib.
- **No `tool` block in `go.mod`** — that alone would require Go 1.24+. This is why the Makefile uses plain `gofmt` rather than `go tool gofumpt`, unlike the sibling repos under this tree. golangci-lint runs from CI (or from PATH if installed), never as a `go tool`.
- `.tool-versions` here pins the *development* toolchain and is deliberately **newer** than the `go` directive — the `min` case the tool itself models.

## Quick Reference

```bash
make build          # Build binaries to build/
make test           # go test -race ./...
make test-cover     # Tests + coverage.html
make vet            # go vet ./...
make fmt            # gofmt -s -w .   (NOT gofumpt — see above)
make lint           # golangci-lint if on PATH, else a no-op with a note
make check-versions # Dogfood: run this repo's own hook on itself (-mode=min)
make sync           # Rewrite .tool-versions from go.mod
make check          # fmt + vet + lint + test + check-versions
```

## Project Structure

```
.pre-commit-hooks.yaml         # Hook manifest consumers resolve by rev
cmd/check-go-version-sync/     # CLI: flag parsing, exit codes, messaging
internal/versionsync/          # The logic: parsing, module discovery, compare, fix
cmd/check-license-headers/     # CLI
internal/licenseheader/        # The logic: header detection, insertion
cmd/check-conflict-markers/    # CLI
internal/conflictmarkers/      # The logic: marker recognition, the separator rule
```

## Design notes

- **Modes.** `exact` (pin == directive) is right for applications; `min` (pin >= directive) is right for libraries, where the directive is a compatibility floor. Defaulting to `exact` and offering `min` is the whole design — using `exact` on a library forces the floor up on every local Go upgrade.
- **Governing pin.** For each `go.mod`, the pin is the nearest ancestor `.tool-versions` carrying a `golang` line. Nested modules work; a nested pin overrides the root.
- **Skip, don't fail.** A module with no `go` directive or no governing pin is skipped. Not every repo uses asdf, and a hook that fails on repos it doesn't apply to is a hook nobody adopts.
- **Version equality ignores a missing patch**: `go 1.22` == `golang 1.22.0`.
- **`-fix` rewrites only the `golang` line**, preserving comments, spacing, and file mode — and still exits non-zero, so the change gets staged deliberately.
- **Exit codes are API**: `0` in sync, `1` drift, `2` the check itself failed. Don't collapse `1` and `2`; pre-commit treats both as failure but a human debugging needs the difference.

## Adding a hook

1. `cmd/<hook-name>/main.go` — `go install ./...` must produce a binary whose name matches the manifest `entry`.
2. Logic in `internal/<pkg>/`, with tests. The CLI layer stays thin: flags, exit codes, messages.
3. Add an entry to `.pre-commit-hooks.yaml` (`language: golang`, a `files` regex, `pass_filenames: false` unless the hook actually takes filenames).
4. Document it in the README with a copy-pasteable `repos:` block.
5. Keep the stdlib-only, low-`go`-directive constraint intact.

## Testing

- `go test -race ./...`. Tests build fixture trees with `t.TempDir()` and never touch the real filesystem outside it.
- CI runs the matrix `{ubuntu, macos} × {go 1.22, go 1.26}` — the oldest supported toolchain and a current one — plus a `hook` job that installs and runs the hook through real pre-commit, which is the only test that proves the consumer path works.

## Releasing

Consumers pin a tag (`rev: v0.1.0`), so **tags are the release mechanism** — there is no binary artifact and no GoReleaser here. Tag `vX.Y.Z` on green CI. A change to a hook's behavior or its manifest needs a new tag before anyone sees it.

## Code Conventions

- Formatter: `gofmt -s`. Not gofumpt (no `tool` block — see the constraint above).
- Linter: golangci-lint v2, config in `.golangci.yml`, enforced in CI.
- Commits/PRs: no AI-attribution trailers (see the tree-level AGENTS.md).

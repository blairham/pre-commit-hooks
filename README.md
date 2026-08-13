# pre-commit-hooks

Go-based [pre-commit](https://pre-commit.com) hooks. Each hook is a real Go
binary — `language: golang`, so pre-commit builds it with `go install` and there
is no shell script, no Python, and nothing to install by hand.

## Hooks

### `check-go-version-sync`

Fails when a module's `go` directive and the `golang` pin that governs it drift
apart, so that asdf and CI don't quietly build with different toolchains.

```yaml
repos:
  - repo: https://github.com/blairham/pre-commit-hooks
    rev: v0.1.0
    hooks:
      - id: check-go-version-sync
```

```
ERROR: go.mod declares 'go 1.26.1' but .tool-versions pins 'golang 1.26.2'

go.mod is authoritative — its `go` directive gets pulled up by the
`tool` block during `go mod tidy`, and .tool-versions follows.
Fix with: check-go-version-sync -fix   (or edit both in one commit)
```

#### Modes

Which rule is correct depends on what the module *is*:

| Mode | Rule | Use for |
| --- | --- | --- |
| `exact` (default) | pin **equals** the `go` directive | Applications and CLIs. The pinned toolchain builds the artifact you ship, so any difference means local and CI disagree. |
| `min` | pin is **>=** the `go` directive | Libraries and reusable modules. The `go` directive is a compatibility floor for consumers; developing on a newer toolchain is correct, not drift. |

```yaml
      - id: check-go-version-sync
        args: [--mode=min]
```

Using `exact` on a library is the common mistake — it forces you to raise your
compatibility floor every time you upgrade your local Go. This repo is itself a
library, and checks itself with `--mode=min`.

#### Auto-fixing

To rewrite `.tool-versions` from `go.mod` instead of just reporting:

```yaml
      - id: check-go-version-sync
        args: [--fix]
```

Like every auto-fixing pre-commit hook, `--fix` still fails the run so the
rewritten file gets staged deliberately rather than slipping into the commit.

**Rules**

- Every `go.mod` in the tree is checked. `vendor/`, `node_modules/`, and
  dot-directories are skipped.
- The **governing pin** for a module is the nearest ancestor `.tool-versions`
  carrying a `golang` line — so nested modules (`apps/<svc>/go.mod`) work, and a
  nested `.tool-versions` overrides the root one.
- A module with no governing pin, or no `go` directive, is **skipped** rather
  than failed. Not every repo uses asdf.
- **A `go` directive without a patch is a language version, not a pin.**
  `go 1.26` says nothing about which patch to build with, so in `exact` mode it
  accepts any `1.26.x` pin — only a different *minor* is a mismatch. A directive
  written with a patch (`go 1.26.1`) is held to exact equality.
- For the same reason `--fix` **refuses** a patchless directive rather than
  writing `golang 1.26`, which asdf cannot resolve. Set that pin by hand.
- `go 1.22` and `golang 1.22.0` are treated as equal — a missing patch component
  is zero.
- Comments in `.tool-versions` are respected, and `--fix` rewrites only the
  `golang` line, preserving everything else and the file mode.

**Why this drifts.** `go mod tidy` pulls the `go` directive up to satisfy a
dependency — most often something in a `tool` block like `golangci-lint` — and
`.tool-versions` silently stays behind. Nothing fails until a build behaves
differently locally than in CI.

## Standalone use

The hook is an ordinary CLI:

```sh
go install github.com/blairham/pre-commit-hooks/cmd/check-go-version-sync@latest

check-go-version-sync              # report drift, exit 1 if any
check-go-version-sync -mode=min    # libraries: pin need only satisfy the floor
check-go-version-sync -fix         # rewrite .tool-versions from go.mod
check-go-version-sync -root ./path # scan a tree other than the cwd
```

Exit codes: `0` in sync, `1` drift found, `2` the check itself failed.

## Compatibility

Works with both [pre-commit](https://pre-commit.com) and
[go-pre-commit](https://github.com/blairham/go-pre-commit).

`go.mod` here targets an old Go release on purpose and has **no dependencies**:
pre-commit's golang backend installs hooks with `GOTOOLCHAIN=local`, so this
module has to build with whatever Go a consumer already has on PATH.

## License

MIT

# CLAUDE.md

@AGENTS.md

<!--
AGENTS.md (imported above) is the cross-tool single source of truth for this repo
— project overview, the GOTOOLCHAIN=local constraint that shapes it, build/test
commands, design notes, and how to add a hook. Claude Code does not read
AGENTS.md natively, so this file imports it and holds only Claude Code-specific
extras. Put repo guidance in AGENTS.md, not here.
-->

## Claude Code-specific notes

- Before changing `go.mod`, re-read the constraint section in AGENTS.md: raising the `go` directive or adding **any** dependency can break hook installation for consumers on older toolchains. This is the single easiest way to break this repo.
- The permission allowlist is in `.claude/settings.json`; the tree-level `~/Developer/github.com/blairham/.claude/settings.json` applies too.
- Verify consumer-facing changes the way CI does — `pre-commit try-repo . check-go-version-sync --all-files` against a scratch repo — not just `go test`.

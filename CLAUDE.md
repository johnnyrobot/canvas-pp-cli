# CLAUDE.md

`canvas-pp-cli` is a **generated** Go CLI ("printed CLI") produced by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). It mirrors the whole Canvas LMS REST API as typed, agent-native commands, keeps a local SQLite mirror for offline join/search, and adds cross-resource commands Canvas never returns from a single endpoint (`roster`, `at-risk`, `to-grade`, `since`, `standings`, `audit-enrollments`). Ships a companion MCP server (`canvas-pp-mcp`, ~1,042 tools).

> This tree is generated output. Prefer runtime discovery and upstream fixes over hand-edits — see Gotchas and `AGENTS.md`.

## Architecture

Go 1.26, module `canvas-pp-cli`. Two binaries, thin `main.go` each:

- `cmd/canvas-pp-cli/` → the CLI (Cobra-based)
- `cmd/canvas-pp-mcp/` → the MCP server exposing the same surface as tools

`internal/` (kept high-level — the command tree is very large, do **not** enumerate it):

- `cli/` — the generated command tree: ~1,180 Go files, one area per Canvas resource plus the novel cross-resource commands. This is the bulk of the code; navigate it with runtime discovery, not by reading files.
- `client/` — Canvas HTTP client (bearer auth, pagination)
- `store/` — local SQLite mirror (`modernc.org/sqlite`, CGO-free) that powers offline joins/search
- `mcp/` — MCP server wiring (`mark3labs/mcp-go`)
- `config/` — config/token resolution (`~/.config/canvas-pp-cli/config.toml`, TOML)
- `cliutil/`, `cache/`, `types/` — shared helpers, response cache, shared types

Auth is a bearer token from `CANVAS_ACCESS_TOKEN` or `CANVAS_API_TOKEN` (also settable in the config file). Release version is injected at build time via ldflags into `internal/cli.version`.

## Commands

Development (Makefile):

```bash
make build        # go build -o bin/canvas-pp-cli ./cmd/canvas-pp-cli
make build-mcp    # go build -o bin/canvas-pp-mcp ./cmd/canvas-pp-mcp
make build-all
make test         # go test ./...
make lint         # golangci-lint run
make install      # go install ./cmd/canvas-pp-cli
```

Runtime discovery — ask the built CLI for current truth instead of trusting a copied command list:

```bash
canvas-pp-cli doctor --json                # environment/auth/runtime status
canvas-pp-cli agent-context --pretty        # machine-oriented overview
canvas-pp-cli which "<capability>" --json   # find the command for a capability
canvas-pp-cli <command> --help
canvas-pp-cli <command> --agent             # JSON, compact, non-interactive, no color
canvas-pp-cli <command> --dry-run --agent   # preview before mutating remote state
```

Use `--yes --no-input` only once the target, args, and side effects are clear.

## Conventions

- Cobra command style, one area per Canvas resource; the generator owns the shape of these files.
- Agent-facing invocations pass `--agent`; anything that may mutate remote state should be `--help`-inspected and `--dry-run`'d first.
- `README.md` and `SKILL.md` are the long-form product/agent docs (install, auth, examples). This file and `AGENTS.md` stay small and cover repo-local invariants only.

## Gotchas & Constraints

- **Generated output.** A fresh print can overwrite the entire tree, so ad-hoc hand-edits don't survive. If you must change generated code, record the intent under `.printing-press-patches/` (parallel to `.printing-press.json`) so a regen carries it forward. Treat systemic problems as upstream Printing Press fixes first.
- **Release ledger.** `CHANGELOG.md` and `.printing-press-release.json` are the public library's per-CLI release ledger. Do not hand-bump them and do not edit `var version = ...` for release bookkeeping — the `mvanhorn/printing-press-library` publish workflow stamps the next `YYYY.M.N` version. Fresh prints may carry blank skeletons; preserve them on reprint.
- Do not enumerate every command file when reasoning about the codebase; use `doctor` / `which` / `--help` discovery.
- SQLite driver is `modernc.org/sqlite` (pure Go); release builds set `CGO_ENABLED=0`. Requires Go 1.26+.
- Never commit tokens. Bearer tokens come from `CANVAS_ACCESS_TOKEN` / `CANVAS_API_TOKEN` or the user config file. Apache-2.0.

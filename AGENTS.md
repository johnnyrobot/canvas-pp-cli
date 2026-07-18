# AGENTS.md

This directory is a **generated** `canvas-pp-cli` printed CLI, produced by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). It mirrors the whole Canvas LMS REST API as agent-native Go commands, keeps a local SQLite mirror for offline join/search, and adds cross-resource commands no single Canvas endpoint returns (`roster`, `at-risk`, `to-grade`, `since`, `standings`, `audit-enrollments`). It ships a companion MCP server binary (`canvas-pp-mcp`). Stack: Go 1.26, Cobra, `modernc.org/sqlite` (pure Go), `mark3labs/mcp-go`.

Because the tree is generated, **treat systemic fixes as upstream Printing Press fixes first**, and keep any local edits narrow and documented (see Commit & PR Conventions).

## Setup

```bash
# End users (recommended): CLI + agent skill in one shot
npx -y @mvanhorn/printing-press-library install canvas

# Go fallback (CLI only; needs Go 1.26+)
go install github.com/mvanhorn/printing-press-library/library/productivity/canvas/cmd/canvas-pp-cli@latest
```

Working in this repo directly requires **Go 1.26+**. Auth is a bearer token via `CANVAS_ACCESS_TOKEN` or `CANVAS_API_TOKEN` (or the config file `~/.config/canvas-pp-cli/config.toml`).

## Build & Run

```bash
make build        # go build -o bin/canvas-pp-cli ./cmd/canvas-pp-cli
make build-mcp    # go build -o bin/canvas-pp-mcp ./cmd/canvas-pp-mcp
make build-all
make install      # go install ./cmd/canvas-pp-cli
make clean
```

Start from runtime truth rather than a copied command list:

```bash
canvas-pp-cli doctor --json           # environment, auth, runtime status
canvas-pp-cli agent-context --pretty  # overview for agents
canvas-pp-cli which "<capability>" --json
canvas-pp-cli <command> --help
```

Add `--agent` for JSON output, compact formatting, non-interactive defaults, no color, and confirmation-safe scripting:

```bash
canvas-pp-cli <command> --agent
```

Before any command that may mutate remote Canvas state, inspect help and prefer a dry run; use `--yes --no-input` only once the target, arguments, and side effects are clear:

```bash
canvas-pp-cli <command> --help
canvas-pp-cli <command> --dry-run --agent
```

For install, auth, and worked examples, read `README.md` and `SKILL.md` — this file stays small so repo-local agents get invariant guidance without duplicating the generated docs.

## Testing

```bash
make test     # go test ./...
make lint     # golangci-lint run
```

A change is done when `go test ./...` and `golangci-lint run` both pass and the affected command still behaves correctly under `--help` / `--dry-run --agent`. Release binaries build with `CGO_ENABLED=0` (see `.goreleaser.yaml`), so keep the code CGO-free.

## Code Style

- Idiomatic Go with Cobra command definitions; one area per Canvas resource. The generator owns the shape of `internal/cli/*` — match existing patterns rather than inventing new ones.
- The command tree is very large (~1,180 files under `internal/cli`); do **not** enumerate it. Navigate by `doctor` / `which` / `--help` discovery.
- Keep the version string generator-controlled: it is injected via ldflags into `internal/cli.version` at build time.

## Commit & PR Conventions

- Git repo (`main`), Apache-2.0, created by [@johnnyrobot](https://github.com/johnnyrobot).
- **This is generated output** — a fresh print can overwrite the whole tree, so ad-hoc hand-edits don't survive on their own. If you modify generated code, record each change under `.printing-press-patches/` (parallel to `.printing-press.json`) so a regen carries the intent forward. The entry shape and the altitude to write it at (a durable reprint-guard, not a changelog) live in the source catalog's `AGENTS.md`, which is the single source of truth.
- **Release ledger:** `CHANGELOG.md` and `.printing-press-release.json` are the public library's per-CLI release ledger. Fresh prints may carry blank skeletons; the final `YYYY.M.N` version is assigned only after a publish PR merges in `mvanhorn/printing-press-library`. Do not hand-bump these files or edit `var version = ...` for release bookkeeping — preserve existing ledger files on reprint and let the library workflow stamp the next release.

## Security & Data

- Never commit tokens. Bearer credentials come from `CANVAS_ACCESS_TOKEN` / `CANVAS_API_TOKEN` or `~/.config/canvas-pp-cli/config.toml` (see `agentcookie.toml` for the secrets-bus manifest).
- Commands can read and mutate real Canvas data for the token's account — always `--dry-run` unfamiliar mutating commands first, and scope tokens to least privilege.

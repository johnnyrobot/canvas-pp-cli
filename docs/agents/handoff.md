# Handoff — canvas-pp-cli

**Last updated:** 2026-08-07
**Canonical location:** `docs/agents/handoff.md` in this repo. Mirror at `~/.claude/handoffs/canvas-pp-cli-handoff.md`.
Any copy under `$TMPDIR` / `/var/folders/…/T/` is **stale and disposable** — macOS wipes it on reboot. Edit the repo copy.
**Repo:** `/Users/laccd/code/cli-tools/canvas-pp-cli` ← **note: moved 2026-08-06**, no longer at `~/code/canvas-pp-cli`
**State:** `main` @ `f5e126f`, clean tree, all four session PRs merged, full suite green. Upstream filings complete; no open decisions.

---

## Where things stand

Four PRs merged into `main`, all green. Don't re-derive their content — read the PRs:

| PR | What |
|---|---|
| [#1](https://github.com/johnnyrobot/canvas-pp-cli/pull/1) | fetch → analyze → render seam across all six novel commands; two ordering bugs fixed |
| [#2](https://github.com/johnnyrobot/canvas-pp-cli/pull/2) | `version: "2"` in `.golangci.yml` — `make lint` was erroring before it linted anything |
| [#3](https://github.com/johnnyrobot/canvas-pp-cli/pull/3) | `docs/agents/*` — issue tracker, triage labels, domain-doc layout |
| [#4](https://github.com/johnnyrobot/canvas-pp-cli/pull/4) | six architecture-review candidates: `which`, MCP bounding, ID dedup, credential tests, mirror warnings, dead-code removal |

The architecture review that produced #1 and #4 was an HTML report at
`$TMPDIR/architecture-review-20260806-231602.html` (7 candidates, ranked). It may have been cleaned up; the candidates are all reflected in #1/#4 commit messages and the two patch entries.

Design decisions behind #1 are **not** in a doc — they were a 20-question grilling session. The commit messages carry the reasoning; `.printing-press-patches/canvas-novel-command-analyze-seam.json` carries the intent.

---

## Filed upstream (mvanhorn/cli-printing-press)

- **[#4016](https://github.com/mvanhorn/cli-printing-press/issues/4016)** — three `which.go.tmpl` scoring defects, verified against v4.30.1
- **[#3370](https://github.com/mvanhorn/cli-printing-press/issues/3370)** — commented with the hidden-area-group traversal detail; a naive tree walk finds 52 commands instead of ~1,000
- **[#4025](https://github.com/mvanhorn/cli-printing-press/issues/4025)** (2026-08-07) — `AuthHeader()` duplicate unreachable `AccessToken` guard. Root cause pinned to `config.go.tmpl:728-750` (per_call env-var range) colliding with `:775-786` (trailing minted-token block) because `resolveEnvVarField("CANVAS_ACCESS_TOKEN")` aliases onto the reserved `AccessToken` field. Related: #773 (closed, same aliasing hazard), #3838 (closed, same function).
- **[#3778](https://github.com/mvanhorn/cli-printing-press/issues/3778)** (2026-08-07) — commented with our reprint reproduction. **It is the inverse of the filed mechanism:** theirs is preserved old *callers* + overwritten *definer*; ours is preserved old *definer* + fresh new *callers*. Argued the durable fix must be symmetric ("does the merged tree resolve", not "did we drop a helper").

---

## The reprint decision — RESOLVED 2026-08-07: **deferred**

User chose option 2. Upstream filings done (#3778 comment, #4025). `main` untouched, still green at `f5e126f`. Nothing further is pending; a future reprint starts from scratch against whatever version is current then.

**A reprint at 4.30.1 was attempted in a sandbox and does not build.**

```
Force regen merged 39 preserved files / 1 AddCommand calls
Error: go build ./... failed
  undefined: resolveReadWithStrategyAndResponsePath   (523 call sites, 523 files)
```

Cause, confirmed: 4.30.1 **introduces** that helper in `data_source.go:145` and calls it from 523 command files; it does not exist at all in our 4.25.0 tree. regen-merge **preserved our `data_source.go`** because PR #4's mirror warning made it drift, and the preserved copy predates the helper. The callers were regenerated fresh. This is a live reproduction of open upstream issue **#3778**, in the inverse direction from the one filed there.

> **Number corrections made 2026-08-07** (the originals in this file were wrong; re-verified against the trees):
> - **523** call sites, not 1,042. `go build` stops at 10 errors per package and prints `too many errors` — the earlier figure came from a bad grep. Never quote a Go error count without checking for truncation.
> - **49** files new in 4.30.1, not 48; plus **32** that exist only in our tree. The 1,197-differ figure is correct.
> - The `AuthHeader` "two consecutive `if`" holds in *generated output only*. The three copies in `config.go.tmpl` are mutually-exclusive `else if` branches; the actual second emitter is a fourth site at `:775-786`. Checking the template instead of the output would have mislabeled the bug.

A clean 4.30.1 baseline was generated and **does** build: `/tmp/fresh-430`. The failed merge attempt is at `/tmp/reprint-test`. Both are disposable; regenerate with:

```bash
cli-printing-press generate --spec spec.yaml --output /tmp/fresh-430
```

### What 4.30.1 already fixes vs. what our patches still carry

| Fix | 4.30.1 | On reconcile |
|---|---|---|
| MCP response bounding | ✅ `bound.EndpointResponse` | drop ours, take upstream |
| `which` WhyItMatters unscored | ❌ | re-apply |
| `which` group substring match | ❌ | re-apply |
| `which` command-tree fallback | ❌ | re-apply |
| ID extraction duplicated store↔cli | ❌ | re-apply |
| Mirror errors swallowed | ❌ | re-apply |
| `AuthHeader` unreachable branch | ❌ (filed #4025) | re-apply unless fixed upstream |
| `internal/config` tests | additive | keep |

Scale: **1,197 files differ**, 49 new in 4.30.1, 32 only in ours. Whole-tree replacement, not a merge.

### Options, for the record

1. Reconcile now — build on `/tmp/fresh-430`, re-apply six fixes, validate. ~half a day, 1,197-file PR.
2. **Defer the reprint** ← **CHOSEN 2026-08-07.** Both filings complete.
3. Stop — `main` is green and shipped.

If a reprint is revisited: the table above (what 4.30.1 fixes vs. what we re-apply) still holds, but re-verify it against the then-current version rather than trusting it. `AuthHeader` is now tracked as #4025 — if upstream fixes it, drop our local removal on reconcile and take theirs.

---

## Gotchas this session cost real time to learn

**`.printing-press-patches/` is write-only.** The generated `AGENTS.md` says recording a patch means "a regen carries the intent forward". It does not — `regenmerge/*.go` has zero references to patches (upstream #3955). What actually preserves work is regen-merge's per-file verdicts: `NOVEL` files survive cleanly; hand-edits to generated files become `TEMPLATED-BODY-DRIFT` and preserve the **whole old file**, which is what broke the reprint. Do not rely on patch entries as insurance.

**golangci-lint default caps hide 85% of findings.** `max-issues-per-linter: 50` / `max-same-issues: 3`. Repo reports 130 issues; the real number is 743. Worse, truncation makes findings *appear to move between files* as unrelated code shifts, which reads as a regression. Always run with both caps disabled when comparing branches:

```bash
golangci-lint run --config <cfg-with-issues.max-*=0> ./...
```

**zsh does not word-split unquoted parameter expansions.** `for c in "roster 12345"; do cmd $c; done` passes one argument, not two. Bit me twice; both times it looked like a product failure. Use arrays or explicit args.

**Piping masks exit codes.** `golangci-lint run | tail` reported exit 0 while the linter had errored on config version and never ran. Capture to a file and check `$?`.

**Six other Claude sessions run on this machine.** One moved the repo mid-session. If paths break, check `~/code/cli-tools/` and siblings before assuming deletion.

---

## Working conventions established — please keep

**Mutation-test every rule.** After writing a test, break the rule it covers and confirm the test goes red, then revert. This caught four tests in this session that passed and proved nothing — including two where the fixture let a different code path answer first, and one where the "whole-query" bonus turned out to be a substring test in disguise. Green tests were the reason both shipped bugs went unnoticed for so long; do not trust a green test you haven't broken.

**Verify subagent claims.** Explorer agents overstated twice (a "byte-identical" block that differed on 21 of 134 lines; call-site counts taken from a buggy grep). Re-run the decisive greps yourself.

**Check upstream before filing.** Two of three "live bugs" found here were already fixed in the current generator; this CLI is printed at 4.25.0 and the generator is at 4.30.1. Templates are in the module cache at
`~/go/pkg/mod/github.com/mvanhorn/cli-printing-press/v4@v4.30.1/internal/generator/templates/`.

**Prove equivalence before deleting a duplicate.** The ID-dedup in #4 ran both implementations over 250 combinations first. The corpus survives as `internal/store/extract_resource_id_test.go`.

**ADHD output mode is ON** for this user (`/i-have-adhd`). Lead with the next action, number multi-step work, restate state every turn, concrete time estimates, no preamble or closers. It persists until they say "stop adhd mode".

---

## Repo facts worth knowing

- Generated tree. `internal/cli` is **one Go package**, 1,181 files, ~171k lines. Never enumerate it; use `doctor` / `which` / `--help`.
- The six novel commands (`roster`, `at-risk`, `to-grade`, `since`, `standings`, `audit-enrollments`) are hand-authored and marked *"Survives `generate --force` regen-merge"*. Everything else says `DO NOT EDIT`.
- `make lint` now runs but **exits 1** — 130 pre-existing issues, untouched by this session and documented in PR #2.
- `internal/cache` and `internal/types` were deleted in #4; don't be surprised by their absence.

---

## Suggested skills

- **`superpowers:verification-before-completion`** — this session's whole thesis. Evidence before assertions; never claim green without showing output.
- **`mattpocock-skills:diagnosing-bugs`** — if picking up any of the remaining review candidates or a reprint failure.
- **`mattpocock-skills:grilling`** — before committing to the reprint. It is a 1,197-file irreversible-ish change; the decision tree deserves the same treatment PR #1 got.
- **`printing-press-reprint`** — only if option 1 is chosen. Note it hands off to `/printing-press` (full research→generate→build→shipcheck), which is much larger than the bare `generate --force` used here.
- **`mattpocock-skills:code-review`** — if reviewing the merged PRs after the fact.
- Avoid `printing-press-amend` — it is disabled for model invocation in this user's `skillOverrides`; the user must run it.

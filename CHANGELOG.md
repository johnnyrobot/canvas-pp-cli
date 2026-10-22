# Changelog

Release notes for canvas-pp-cli. Versions follow the git tags on this repository; release artifacts are published by goreleaser.


## Unreleased

### Canvas Builder mode

The CLI can now run inside a course workspace opened by the Canvas Builder macOS app, against that app's own copy of Canvas. The mode is on when `sandbox = true` is in the workspace's `config/config.toml`, or when the binary was stamped for it at build time; `doctor` reports it.

- **Added** `canvas-pp-cli check <a11y|dates|placeholders|links|all>`: builds a snapshot of the course through the gateway and reports what is wrong with it — the content students read, every date, text left over from a previous term, and the links. In Canvas Builder the course's closure days come from the workspace.
- **Added** `canvas-pp-cli upload <path>` and `canvas-pp-cli download <course> <file>`: the whole file-upload exchange in one command, confined to the course workspace folder. `upload` hands back `embed_src`, the address a page has to use to show the file; `courses create-file` only ever did the first of the three steps.
- **Added** `canvas-pp-cli recipes [<name>]`: the order the commands go in for each of the jobs an agent is sent to a course to do — moving a course to a new term, accessibility fixes, a design review, an AI-use map, building from materials, and inserting an image. Each is a subcommand, and all of them are in the `which` index along with `check`, `upload` and `download`.
- **Added** `--job "<name>"`, sent as `X-Job-Label`, so Canvas Builder groups a job's changes for the instructor to review and undo together. A write without one still goes out, with a note on stderr.
- **Added** exit codes 8 (the outcome of a change is unknown) and 9 (Canvas Builder stopped it, with its own message passed through unchanged).
- **Changed** `assignments bulk-update` to send the JSON list Canvas expects, so a term's due dates move in one approved, undoable change instead of one per assignment.
- **Changed** retries: no write is retried after an ambiguous failure in Canvas Builder mode, because through the gateway a retry is a second recorded change and a retried delete asks the instructor a second time. POST and PATCH are no longer retried in any mode.
- **Changed** the student-data commands (`roster`, `at-risk`, `to-grade`, `standings`, `audit-enrollments`, `since`) and commands that reach other courses: in Canvas Builder they are absent from `--help` and from `which`, rather than offered and then refused. Outside Canvas Builder nothing about them changed.

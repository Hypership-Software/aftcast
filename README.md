# Aftcast

Aftcast is a local observability layer for AI coding agents. It watches Claude
Code's tool calls, records a tamper-evident metadata trail, and turns that trail
into a project-first view of what shipped, how long the work took, what changed,
where the agent needed recovery, and what keeps failing often enough to deserve
a permanent fix.

Aftcast observes; it never blocks a tool call. Prompts and file contents are not
persisted. The local record contains operational metadata such as timings,
repository-relative paths, invoked skills, risk classifications, and numeric
line-change counts.

The product is Aftcast; the CLI is currently named `gated`.

## Install from source

Release artifacts are not yet published; building from source is the supported
path. You need Git, Go 1.25+, and Claude Code.

```bash
git clone https://github.com/Hypership-Software/aftcast.git
cd aftcast
mkdir -p dist
cd core
CGO_ENABLED=0 go build -trimpath -o ../dist/gated ./cmd/gated
cd ..
./dist/gated init
```

`gated init` prints each action it takes and verifies the local hook endpoint.
Open a new terminal after it finishes (or reload your shell), then verify:

```bash
gated status
gated doctor
```

`status` should report a running daemon and wired Claude Code hooks. Every check
in `doctor` should report `ok`.

## Start using it

Start a new Claude Code session after installation so it loads the hooks.
Aftcast observes sessions from that point forward.

From any Git repository, run:

```bash
gated
```

That opens the current repository's workspace. `gated insights --all` browses
every observed repository. To see what keeps failing across your sessions —
the same kind of failure in three or more sessions on two or more days:

```bash
gated coach
```

`gated coach export <id>` writes a plain-English evidence bundle for one of
those recurring failures: counts, dates, and session references only, never
command content. Hand it to your agent to encode a permanent fix.

## What `gated init` changes

All changes are local and reversible:

- copies the running binary to `~/.gated/bin/gated` and adds that directory to
  PATH using a marked block in your shell profile;
- starts the Aftcast daemon in the background on localhost;
- merges Aftcast hooks into `~/.claude/settings.json` without removing your
  other settings or hooks, backing the file up first;
- stores the local audit log, policies, daemon state, and logs under `~/.gated`.

`gated uninstall` stops the daemon and removes the hooks and PATH block. It
leaves `~/.gated` in place so uninstalling never silently destroys your local
audit history.

## What Aftcast records — and what it never records

Recorded (metadata only, local only): timings, tool classes, risk
classifications, repository-relative paths, command verbs and exit codes,
invoked skills, and numeric line-change counts.

Never recorded: prompts, file contents, command text, code, or credentials.
Nothing is exported and nothing phones home; the record lives entirely in
`~/.gated` on your machine.

## About this repository

This is the open-source core of Aftcast (Apache-2.0), mirrored from a private
monorepo where development happens. `core/` is the Go binary — adapters, audit
log, telemetry read-model, analytics, and the terminal UI. History is mirrored;
issues are welcome here.

## Development

```bash
cd core
go build ./...
go test ./...
go vet ./...
```

The binary is CGO-free and cross-compiles for Windows, Linux, and macOS. SQLite
uses the pure-Go `modernc.org/sqlite` driver.

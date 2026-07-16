# aftcast

Aftcast is a local observability layer for AI coding agents. It watches Claude
Code's tool calls, records a tamper-evident metadata trail, and turns that
trail into a project-first view of what shipped, how long the work took, where
the agent needed recovery, and what keeps failing often enough to deserve a
permanent fix.

Aftcast observes; it never blocks a tool call. Prompts and file contents are
never recorded, nothing phones home, and the record lives entirely in
`~/.aftcast` on your machine.

## Install

```bash
npx aftcast@latest init
```

or, for a global install:

```bash
npm install -g aftcast
aftcast init
```

`init` installs the binary to `~/.aftcast/bin`, adds it to PATH, starts the
local daemon, and wires the Claude Code hooks. Start a new Claude Code session
afterwards and Aftcast observes it from there. `aftcast uninstall` reverses
everything except your local history.

This package is a thin launcher: it pulls the prebuilt binary for your
platform (macOS, Linux, Windows; x64 and arm64) via an optional dependency and
executes it. No install scripts run.

Full documentation: https://github.com/Hypership-Software/aftcast

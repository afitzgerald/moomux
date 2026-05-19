# curral — Design Spec

**Date:** 2026-05-19  
**Status:** Approved  
**Repo:** github.com/erickgnclvs/curral

---

## What it is

A general-purpose TUI for managing Claude Code agent sessions across git worktrees. Anyone can install and configure it for their own repos. Built as a single Go binary — no daemon, no background process.

Replaces claude-squad's workflow (create / resume / delete sessions) without the lag, using tmux as the session backend and iTerm2 for tab management.

---

## Architecture

### Components

**TUI (Bubbletea + Lip Gloss)**
- Split-pane layout: session list (left), detail panel (right)
- Keyboard-driven interaction
- Renders at terminal frame rate, zero polling overhead in render path
- Subscribes to state updates from Status Watcher via Go channel

**Session Manager**
- CRUD operations for sessions (create, open, delete)
- Runs `git worktree add/remove` against the configured repo
- Creates and kills `tmux` sessions
- Opens new iTerm2 tab via `osascript` when a session is opened

**Status Watcher**
- Polls `~/.claude/sessions/*.json` every 2 seconds
- Maps Claude process PID → curral session by matching `cwd` to known worktree paths
- Emits status change events to TUI via channel
- Three states: `working`, `waiting`, `parked`

**Config**
- Reads/writes `~/.config/curral/config.toml`
- Sessions persisted to `~/.config/curral/sessions.json`
- Worktrees created at `~/.local/share/curral/worktrees/<project>/<session>` by default (overridable per project)

### Data flow — open session

```
User presses enter
  → Session Manager checks tmux session exists (creates if parked)
  → osascript opens new iTerm2 tab
  → Tab attaches to tmux session
  → claude is already running (or launched fresh)
```

### Data flow — status update

```
Status Watcher polls ~/.claude/sessions/*.json every 2s
  → matches cwd field to known worktree paths
  → determines state from busy/status fields
  → sends update over channel to TUI model
  → TUI re-renders affected row only
```

---

## Config format

`~/.config/curral/config.toml`:

```toml
[projects.eg_system]
repo          = "~/Development/eg_system"
branch_prefix = "erickgoncalves"   # optional — prepended to session name as branch
base_branch   = "main"

[projects.other_repo]
repo        = "~/Development/other_repo"
base_branch = "main"
```

`~/.config/curral/sessions.json`:

```json
{
  "version": 1,
  "sessions": {
    "eg_system:hash-password": {
      "id": "eg_system:hash-password",
      "project": "eg_system",
      "name": "hash-password",
      "branch": "erickgoncalves/hash-password",
      "worktree_path": "~/.local/share/curral/worktrees/eg_system/hash-password",
      "tmux_session": "curral-hash-password",
      "created_at": "2026-05-17T10:00:00Z"
    }
  }
}
```

---

## TUI layout

```
┌─────────────────────────────────────────────────────────────────┐
│ curral                                    eg_system  other_repo  │
├──────────────────────────┬──────────────────────────────────────┤
│ SESSIONS                 │ DETAIL                               │
│                          │                                      │
│ ▶ hash-password  ⬤ work │  status:   ⬤ working               │
│   helping-tati   ⬤ wait │  branch:   erickgoncalves/hash-…    │
│   json-password  ○ park  │  worktree: ~/.local/share/curral/…  │
│   mfa-password   ○ park  │  created:  2 days ago               │
│                          │                                      │
├──────────────────────────┴──────────────────────────────────────┤
│ n:new  enter:open  d:delete  r:refresh  tab:project  q:quit     │
└─────────────────────────────────────────────────────────────────┘
```

---

## Status states

| Indicator | Label    | Condition                                                           |
|-----------|----------|---------------------------------------------------------------------|
| `⬤` green  | working  | Claude session file exists with `status != "idle"` (e.g. `"busy"`) |
| `⬤` amber  | waiting  | Claude session file exists with `status == "idle"` + tmux running  |
| `○` dark   | parked   | No tmux session — worktree exists, can be resumed                  |

---

## Keyboard shortcuts

| Key         | Action                                        |
|-------------|-----------------------------------------------|
| `↑` / `k`   | Move selection up                             |
| `↓` / `j`   | Move selection down                           |
| `enter`     | Open session in new iTerm2 tab                |
| `n`         | New session (inline form overlay)             |
| `d`         | Delete session (confirmation prompt)          |
| `r`         | Force refresh status                          |
| `tab`       | Switch between registered projects            |
| `q` / `ctrl+c` | Quit                                       |

---

## New session flow

1. User presses `n` → inline form overlay appears (project pre-selected if only one)
2. User types session name → presses enter
3. `git fetch origin <base_branch>`
4. `git worktree add <path> -b <branch_prefix>/<name> origin/<base_branch>` (if `branch_prefix` is unset, branch name = `<name>` directly)
5. `tmux new-session -d -s curral-<name> -c <worktree>`
6. `tmux send-keys "claude" Enter`
7. `osascript` opens new iTerm2 tab attached to tmux session
8. Session appears in list as `⬤ waiting`

---

## Delete flow

1. User presses `d` → confirmation overlay: "Delete `<name>`? This kills the tmux session and removes the worktree. The branch is kept."
2. User presses `y` → Session Manager:
   - `tmux kill-session -t curral-<name>` (if running)
   - `git worktree remove <path> --force`
   - Removes entry from `sessions.json`
3. Session disappears from list

---

## Go project structure

```
curral/
├── main.go
├── go.mod
├── internal/
│   ├── config/       # config.toml reading/writing
│   ├── session/      # session CRUD, sessions.json
│   ├── tmux/         # tmux shell commands
│   ├── iterm/        # osascript iTerm2 tab opener
│   ├── watcher/      # ~/.claude/sessions/*.json poller
│   └── tui/          # Bubbletea model, views, keys
│       ├── model.go
│       ├── list.go
│       ├── detail.go
│       ├── form.go   # new session + delete overlays
│       └── styles.go # Lip Gloss styles
└── docs/
    └── superpowers/specs/
```

---

## Dependencies

| Package                          | Purpose                    |
|----------------------------------|----------------------------|
| `github.com/charmbracelet/bubbletea` | TUI event loop         |
| `github.com/charmbracelet/lipgloss` | TUI styling             |
| `github.com/charmbracelet/bubbles`  | List, text input components |
| `github.com/BurntSushi/toml`        | Config parsing          |

No daemon. No network calls. No external services.

---

## Out of scope (v1)

- Windows / Linux iTerm2 replacement (terminal opener is macOS-only in v1)
- PR/issue integration
- Session search / filter
- Branch status (ahead/behind remote)
- Multiple Claude instances per session

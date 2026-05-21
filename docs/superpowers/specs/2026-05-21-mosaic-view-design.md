# Mosaic View — Design Spec

**Date:** 2026-05-21
**Branch:** tabs-or-tmux
**Status:** Approved

---

## Summary

Press `m` in the moomux TUI session list. moomux creates (or refreshes) a tmux window named
`moomux-mosaic` in the current tmux session, tiling all live agent sessions as panes. Each pane
runs `tmux attach -t moomux-<name>`, giving the user a real interactive multi-pane view. Detaching
with `Ctrl+b p` (or any window-navigation key) returns them to the moomux TUI window.

This is a pure tmux-orchestration feature: no in-process terminal rendering, no polling, no
live-sync state. moomux creates the layout once; tmux owns the pane lifecycle.

---

## Requirements

- Requires moomux to be running inside a tmux session (`$TMUX` must be set). If not → flash error.
- Only sessions where the underlying tmux session is alive are tiled. Parked/dead sessions are skipped.
- 0 live sessions → flash error "no live sessions to tile".
- The mosaic window is always rebuilt from scratch: kill any existing `moomux-mosaic` window, then recreate.
- Pressing `m` again while the mosaic already exists rebuilds it (picks up new/deleted sessions).
- The feature is opt-in and invisible to users who don't press `m`.

---

## Architecture

### New package: `internal/mosaic/mosaic.go`

```go
type Client struct {
    Tmux *tmux.Client
}

func (c *Client) Open(sessions []session.Session) error
```

`Open` executes this tmux command sequence:

```
1. tmux kill-window -t moomux-mosaic           (best-effort, ignore "not found" error)
2. tmux new-window -d -n moomux-mosaic
3. for sessions[0]:
     tmux send-keys -t moomux-mosaic "tmux attach -t <TmuxSession>" Enter
4. for sessions[1..N]:
     tmux split-window -t moomux-mosaic
     tmux send-keys -t moomux-mosaic "tmux attach -t <TmuxSession>" Enter
5. tmux select-layout -t moomux-mosaic tiled
6. tmux set-option -t moomux-mosaic pane-border-status top
7. for i, s := range sessions:
     tmux select-pane -t moomux-mosaic.<i> -T "<s.Name>"
8. tmux select-window -t moomux-mosaic
```

Step 8 switches the user's tmux client to the mosaic window. The moomux TUI stays in its original
window; the user returns with standard tmux window navigation.

All tmux commands run without an explicit `-t <session>` for steps 1–8 so they operate in the
current session (resolved from `$TMUX`). This matches how every other tmux command in the project
works when launched from inside a tmux session.

---

### `internal/tmux/tmux.go` — new methods

Eight thin wrappers added to `Client`, following the same pattern as existing methods:

| Method | tmux command |
|--------|-------------|
| `KillWindow(name string) error` | `kill-window -t <name>` |
| `NewWindow(name string) error` | `new-window -d -n <name>` |
| `SplitWindow(target string) error` | `split-window -t <target>` |
| `SendKeys(target, cmd string) error` | `send-keys -t <target> <cmd> Enter` |
| `SelectLayout(target, layout string) error` | `select-layout -t <target> <layout>` |
| `SetPaneBorderStatus(target, val string) error` | `set-option -t <target> pane-border-status <val>` |
| `SelectPane(target, title string) error` | `select-pane -t <target> -T <title>` |
| `SelectWindow(name string) error` | `select-window -t <name>` |

All return an error from `c.Runner.Run(...)`. The `Runner` interface is already injectable so the
new methods are covered by the existing test harness pattern.

---

### `internal/app/app.go` — new method

```go
func (a *App) OpenMosaic() error {
    if os.Getenv("TMUX") == "" {
        return fmt.Errorf("mosaic requires running inside a tmux session")
    }
    var live []session.Session
    for _, s := range a.Store.All() {
        if ok, _ := a.Tmux.HasSession(s.TmuxSession); ok {
            live = append(live, s)
        }
    }
    if len(live) == 0 {
        return fmt.Errorf("no live sessions to tile")
    }
    mc := mosaic.Client{Tmux: a.Tmux}
    return mc.Open(live)
}
```

---

### TUI changes

**`internal/tui/keys.go`**
- Add `Mosaic key.Binding` to `KeyMap` struct.
- Default: key `"m"`, help `"m  mosaic"`.

**`internal/tui/model.go`**
- Add `OpenMosaic() error` to the `Backend` interface.

**`internal/tui/messages.go`**
- Add `type MosaicOpenedMsg struct{}`.

**`internal/tui/update.go`**
- In `updateList`: add a case for `m.keys.Mosaic` that calls `m.backend.OpenMosaic()` as a
  `tea.Cmd`, returning `MosaicOpenedMsg{}` on success or `ErrorMsg{}` on failure.
- In `Update`: handle `MosaicOpenedMsg` by flashing
  `"mosaic open — Ctrl+b p to return"`.

---

## User Experience

1. User presses `m` in the session list.
2. Flash message appears: `"mosaic open — Ctrl+b p to return"`.
3. tmux switches focus to the `moomux-mosaic` window.
4. Panes are tiled; each pane border shows the session name at the top.
5. Each pane is a fully interactive terminal attached to its agent session.
6. `Ctrl+b p` / `Ctrl+b [window number]` returns to the moomux TUI.
7. Pressing `m` again rebuilds the mosaic (useful after adding/removing sessions).

---

## Error cases

| Condition | Behavior |
|-----------|----------|
| `$TMUX` not set | Flash: `"mosaic requires running inside a tmux session"` |
| No live sessions | Flash: `"no live sessions to tile"` |
| tmux command fails mid-build | Flash: `"error: <tmux stderr>"` — mosaic window may be partial; user can press `m` to retry |

---

## Out of scope (future)

- Live status icon updates in pane titles (requires watcher state to flow through to mosaic).
- Configurable layouts per project or per session count.
- A scope-filtered mosaic (current project only vs. all projects).
- Companion-pane mode (TUI left, selected session right).

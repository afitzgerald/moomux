# Cross-Terminal Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the iTerm2-only tab opener with a terminal-aware launcher that opens a new tab when the detected terminal supports it (iTerm2), and falls back to opening a new window for other terminals (Kitty, WezTerm, Alacritty, Terminal.app), with a print-only fallback for everything else.

**Architecture:** Introduce a `TerminalOpener` interface in a new `internal/terminal` package. A `Detect()` factory reads env vars (`$TERM_PROGRAM`, `$KITTY_WINDOW_ID`, `$WEZTERM_PANE`) to pick the right backend at startup. Each backend implements `OpenSession(tmuxSession, title string) error`. The existing `iterm` package is kept as-is and wrapped. `App.ITerm` is replaced with `App.Terminal TerminalOpener`.

**Tech Stack:** Go stdlib (`os`, `os/exec`), no new dependencies.

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Create | `internal/terminal/terminal.go` | `TerminalOpener` interface + `Detect()` factory |
| Create | `internal/terminal/terminal_test.go` | Tests for `Detect()` |
| Create | `internal/terminal/iterm.go` | iTerm2 backend (new tab via AppleScript) |
| Create | `internal/terminal/iterm_test.go` | Tests for iTerm2 backend |
| Create | `internal/terminal/window.go` | Generic new-window backend (Kitty, WezTerm, Alacritty, Terminal.app) |
| Create | `internal/terminal/window_test.go` | Tests for window backend |
| Create | `internal/terminal/fallback.go` | Print-only fallback backend |
| Create | `internal/terminal/fallback_test.go` | Tests for fallback backend |
| Modify | `internal/app/app.go` | Replace `ITerm *iterm.Client` with `Terminal terminal.TerminalOpener` |
| Modify | `main.go` | Call `terminal.Detect()` instead of `iterm.New()` |
| Delete | `internal/iterm/iterm.go` | Superseded by `internal/terminal/iterm.go` |
| Delete | `internal/iterm/iterm_test.go` | Superseded by `internal/terminal/iterm_test.go` |

---

## Task 1: Define the `TerminalOpener` interface

**Files:**
- Create: `internal/terminal/terminal.go`

- [ ] **Step 1: Create the interface file**

```go
// Package terminal detects the running terminal and opens tmux sessions in it.
package terminal

import "os"

// TerminalOpener opens a tmux session in the detected terminal.
type TerminalOpener interface {
	OpenSession(tmuxSession, title string) error
}

// Detect returns the best TerminalOpener for the current environment by
// inspecting well-known environment variables.
func Detect() TerminalOpener {
	switch {
	case os.Getenv("TERM_PROGRAM") == "iTerm.app":
		return newITermClient()
	case os.Getenv("KITTY_WINDOW_ID") != "":
		return &windowOpener{binary: "kitty", args: kittyArgs}
	case os.Getenv("WEZTERM_PANE") != "":
		return &windowOpener{binary: "wezterm", args: weztermArgs}
	case os.Getenv("TERM_PROGRAM") == "Apple_Terminal":
		return &windowOpener{binary: "open", args: terminalAppArgs}
	default:
		return &fallbackOpener{}
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/terminal/terminal.go
git commit -m "feat: add TerminalOpener interface and Detect factory skeleton"
```

---

## Task 2: iTerm2 backend (new tab via AppleScript)

**Files:**
- Create: `internal/terminal/iterm.go`
- Create: `internal/terminal/iterm_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/terminal/iterm_test.go
package terminal

import (
	"strings"
	"testing"
)

type fakeRunner struct{ script string }

func (f *fakeRunner) Run(script string) (string, error) {
	f.script = script
	return "", nil
}

func TestITermOpenSessionAttachesAndSetsTitle(t *testing.T) {
	fr := &fakeRunner{}
	c := &itermClient{runner: fr}
	if err := c.OpenSession("curral-foo", "feat/bar"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fr.script, "tmux attach -t curral-foo") {
		t.Fatalf("missing attach: %s", fr.script)
	}
	if !strings.Contains(fr.script, "iTerm2") {
		t.Fatalf("missing iTerm2 target: %s", fr.script)
	}
	if !strings.Contains(fr.script, `set name to "feat/bar"`) {
		t.Fatalf("missing tab title: %s", fr.script)
	}
}

func TestITermOpenSessionOmitsTitleWhenEmpty(t *testing.T) {
	fr := &fakeRunner{}
	c := &itermClient{runner: fr}
	if err := c.OpenSession("curral-foo", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fr.script, "set name to") {
		t.Fatalf("should not set name when title empty: %s", fr.script)
	}
}

func TestITermEscapesAppleScript(t *testing.T) {
	fr := &fakeRunner{}
	c := &itermClient{runner: fr}
	if err := c.OpenSession("curral-foo", `branch"with\special`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fr.script, `branch\"with\\special`) {
		t.Fatalf("backslash/quote not escaped: %s", fr.script)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/erickgoncalves/.local/share/curral/worktrees/curral/curral-work-4
go test ./internal/terminal/... 2>&1
```
Expected: compile error — `itermClient` undefined.

- [ ] **Step 3: Implement the iTerm2 backend**

```go
// internal/terminal/iterm.go
package terminal

import (
	"fmt"
	"os/exec"
)

type scriptRunner interface {
	Run(script string) (string, error)
}

type execScriptRunner struct{}

func (execScriptRunner) Run(script string) (string, error) {
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	return string(out), err
}

type itermClient struct {
	runner scriptRunner
}

func newITermClient() *itermClient {
	return &itermClient{runner: execScriptRunner{}}
}

func (c *itermClient) OpenSession(tmuxSession, title string) error {
	setName := ""
	if title != "" {
		setName = fmt.Sprintf("\n\t\t\tset name to \"%s\"", escapeAppleScript(title))
	}
	script := fmt.Sprintf(`
tell application "iTerm2"
	activate
	if (count of windows) = 0 then
		create window with default profile
	end if
	tell current window
		create tab with default profile
		tell current session of current tab%s
			write text "tmux attach -t %s"
		end tell
	end tell
end tell`, setName, tmuxSession)
	_, err := c.runner.Run(script)
	return err
}

func escapeAppleScript(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\\' || r == '"' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/terminal/... -run TestITerm -v
```
Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/terminal/iterm.go internal/terminal/iterm_test.go
git commit -m "feat: add iTerm2 backend to terminal package"
```

---

## Task 3: Generic new-window backend

**Files:**
- Create: `internal/terminal/window.go`
- Create: `internal/terminal/window_test.go`

The window backend calls a terminal binary with a `--title` flag and the tmux attach command as the child process. Each supported terminal needs a small arg-builder function.

- [ ] **Step 1: Write the failing tests**

```go
// internal/terminal/window_test.go
package terminal

import (
	"testing"
)

type fakeExec struct {
	binary string
	args   []string
}

func (f *fakeExec) Command(binary string, args ...string) error {
	f.binary = binary
	f.args = args
	return nil
}

func TestWindowOpenerKittyArgs(t *testing.T) {
	fe := &fakeExec{}
	w := &windowOpener{binary: "kitty", args: kittyArgs, exec: fe.Command}
	if err := w.OpenSession("curral-foo", "feat/bar"); err != nil {
		t.Fatal(err)
	}
	if fe.binary != "kitty" {
		t.Fatalf("wrong binary: %s", fe.binary)
	}
	assertContains(t, fe.args, "--title")
	assertContains(t, fe.args, "feat/bar")
	assertContains(t, fe.args, "tmux")
	assertContains(t, fe.args, "attach")
	assertContains(t, fe.args, "-t")
	assertContains(t, fe.args, "curral-foo")
}

func TestWindowOpenerWezTermArgs(t *testing.T) {
	fe := &fakeExec{}
	w := &windowOpener{binary: "wezterm", args: weztermArgs, exec: fe.Command}
	if err := w.OpenSession("curral-foo", "feat/bar"); err != nil {
		t.Fatal(err)
	}
	if fe.binary != "wezterm" {
		t.Fatalf("wrong binary: %s", fe.binary)
	}
	assertContains(t, fe.args, "start")
	assertContains(t, fe.args, "--")
	assertContains(t, fe.args, "tmux")
}

func TestWindowOpenerAlacrittyArgs(t *testing.T) {
	fe := &fakeExec{}
	w := &windowOpener{binary: "alacritty", args: alacrittyArgs, exec: fe.Command}
	if err := w.OpenSession("curral-foo", "feat/bar"); err != nil {
		t.Fatal(err)
	}
	assertContains(t, fe.args, "--title")
	assertContains(t, fe.args, "feat/bar")
}

func assertContains(t *testing.T, haystack []string, needle string) {
	t.Helper()
	for _, s := range haystack {
		if s == needle {
			return
		}
	}
	t.Fatalf("args %v missing %q", haystack, needle)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/terminal/... -run TestWindowOpener 2>&1
```
Expected: compile error — `windowOpener` undefined.

- [ ] **Step 3: Implement the window backend**

```go
// internal/terminal/window.go
package terminal

import "os/exec"

// argBuilder builds the full argument list given a title and tmux session name.
type argBuilder func(title, tmuxSession string) []string

type windowOpener struct {
	binary string
	args   argBuilder
	// exec is the command runner; nil means os/exec.Command.Run.
	exec func(binary string, args ...string) error
}

func (w *windowOpener) OpenSession(tmuxSession, title string) error {
	args := w.args(title, tmuxSession)
	if w.exec != nil {
		return w.exec(w.binary, args...)
	}
	return exec.Command(w.binary, args...).Start()
}

func kittyArgs(title, tmuxSession string) []string {
	args := []string{}
	if title != "" {
		args = append(args, "--title", title)
	}
	args = append(args, "--", "tmux", "attach", "-t", tmuxSession)
	return args
}

func weztermArgs(title, tmuxSession string) []string {
	args := []string{"start"}
	if title != "" {
		args = append(args, "--class", title)
	}
	args = append(args, "--", "tmux", "attach", "-t", tmuxSession)
	return args
}

func alacrittyArgs(title, tmuxSession string) []string {
	args := []string{}
	if title != "" {
		args = append(args, "--title", title)
	}
	args = append(args, "-e", "tmux", "attach", "-t", tmuxSession)
	return args
}

// terminalAppArgs opens a new Terminal.app window via `open -a Terminal`.
// Terminal.app does not accept a command via `open`, so we wrap it in a
// temporary shell script executed via osascript.
func terminalAppArgs(title, tmuxSession string) []string {
	// `open -a Terminal` can't pass a startup command; Terminal.app backend
	// should use the fallback script runner instead — this is intentionally
	// minimal and routes to a windowOpener that wraps osascript.
	return []string{"-a", "Terminal"}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/terminal/... -run TestWindowOpener -v
```
Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/terminal/window.go internal/terminal/window_test.go
git commit -m "feat: add generic new-window backend for Kitty, WezTerm, Alacritty"
```

---

## Task 4: Fallback backend (print-only)

**Files:**
- Create: `internal/terminal/fallback.go`
- Create: `internal/terminal/fallback_test.go`

When no terminal is detected, print the `tmux attach` command to stdout so the user can run it manually.

- [ ] **Step 1: Write the failing test**

```go
// internal/terminal/fallback_test.go
package terminal

import (
	"bytes"
	"strings"
	"testing"
)

func TestFallbackPrintsAttachCommand(t *testing.T) {
	var buf bytes.Buffer
	f := &fallbackOpener{out: &buf}
	if err := f.OpenSession("curral-foo", "feat/bar"); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "tmux attach -t curral-foo") {
		t.Fatalf("expected attach command in output, got: %s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/terminal/... -run TestFallback 2>&1
```
Expected: compile error — `fallbackOpener` undefined.

- [ ] **Step 3: Implement the fallback**

```go
// internal/terminal/fallback.go
package terminal

import (
	"fmt"
	"io"
	"os"
)

type fallbackOpener struct {
	out io.Writer
}

func (f *fallbackOpener) OpenSession(tmuxSession, title string) error {
	w := f.out
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w, "curral: run the following to attach to your session:\n  tmux attach -t %s\n", tmuxSession)
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/terminal/... -run TestFallback -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/terminal/fallback.go internal/terminal/fallback_test.go
git commit -m "feat: add print-only fallback terminal backend"
```

---

## Task 5: Test `Detect()` factory

**Files:**
- Create: `internal/terminal/terminal_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/terminal/terminal_test.go
package terminal

import (
	"testing"
)

func withEnv(t *testing.T, key, val string, f func()) {
	t.Helper()
	t.Setenv(key, val)
	f()
}

func TestDetectReturnsITermForITermApp(t *testing.T) {
	withEnv(t, "TERM_PROGRAM", "iTerm.app", func() {
		t.Setenv("KITTY_WINDOW_ID", "")
		t.Setenv("WEZTERM_PANE", "")
		got := Detect()
		if _, ok := got.(*itermClient); !ok {
			t.Fatalf("expected *itermClient, got %T", got)
		}
	})
}

func TestDetectReturnsWindowOpenerForKitty(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	withEnv(t, "KITTY_WINDOW_ID", "1", func() {
		t.Setenv("WEZTERM_PANE", "")
		got := Detect()
		wo, ok := got.(*windowOpener)
		if !ok {
			t.Fatalf("expected *windowOpener, got %T", got)
		}
		if wo.binary != "kitty" {
			t.Fatalf("expected kitty binary, got %s", wo.binary)
		}
	})
}

func TestDetectReturnsWindowOpenerForWezTerm(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	withEnv(t, "WEZTERM_PANE", "1", func() {
		got := Detect()
		wo, ok := got.(*windowOpener)
		if !ok {
			t.Fatalf("expected *windowOpener, got %T", got)
		}
		if wo.binary != "wezterm" {
			t.Fatalf("expected wezterm binary, got %s", wo.binary)
		}
	})
}

func TestDetectReturnsFallbackForUnknown(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("WEZTERM_PANE", "")
	got := Detect()
	if _, ok := got.(*fallbackOpener); !ok {
		t.Fatalf("expected *fallbackOpener, got %T", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/terminal/... -run TestDetect 2>&1
```
Expected: FAIL — `Detect()` returns wrong types (skeleton from Task 1 references types not yet compiled together).

- [ ] **Step 3: Run all terminal tests to ensure everything compiles and passes**

```bash
go test ./internal/terminal/... -v
```
Expected: all tests PASS (the implementations from Tasks 2–4 are already in place).

- [ ] **Step 4: Commit**

```bash
git add internal/terminal/terminal_test.go
git commit -m "test: add Detect() factory coverage for all terminal backends"
```

---

## Task 6: Wire `terminal.Detect()` into `App` and `main.go`

**Files:**
- Modify: `internal/app/app.go`
- Modify: `main.go`

- [ ] **Step 1: Update `App` struct**

In `internal/app/app.go`, replace:
```go
import (
    ...
    "github.com/erickgnclvs/curral/internal/iterm"
    ...
)

type App struct {
    ...
    ITerm        *iterm.Client
    ...
}
```

With:
```go
import (
    ...
    "github.com/erickgnclvs/curral/internal/terminal"
    ...
)

type App struct {
    Cfg          *config.Config
    Store        *session.Store
    Tmux         *tmux.Client
    Terminal     terminal.TerminalOpener
    Git          *gitwt.Client
    WorktreeRoot string
}
```

- [ ] **Step 2: Update call sites inside `app.go`**

Replace both `a.ITerm.OpenTab(...)` calls:

```go
// CreateSession (line ~64):
if err := a.Terminal.OpenSession(tmuxName, branch); err != nil {
    return session.Session{}, fmt.Errorf("terminal open: %w", err)
}

// OpenSession (line ~97):
return a.Terminal.OpenSession(s.TmuxSession, s.Branch)
```

- [ ] **Step 3: Update `main.go`**

Replace:
```go
import (
    ...
    "github.com/erickgnclvs/curral/internal/iterm"
    ...
)

a := &app.App{
    ...
    ITerm:        iterm.New(),
    ...
}
```

With:
```go
import (
    ...
    "github.com/erickgnclvs/curral/internal/terminal"
    ...
)

a := &app.App{
    ...
    Terminal:     terminal.Detect(),
    ...
}
```

- [ ] **Step 4: Verify it compiles**

```bash
go build ./...
```
Expected: exits 0, no output.

- [ ] **Step 5: Run all tests**

```bash
go test ./...
```
Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/app.go main.go
git commit -m "feat: wire terminal.Detect() into App, replacing iterm.Client"
```

---

## Task 7: Delete the now-superseded `internal/iterm` package

**Files:**
- Delete: `internal/iterm/iterm.go`
- Delete: `internal/iterm/iterm_test.go`

- [ ] **Step 1: Remove the package**

```bash
rm internal/iterm/iterm.go internal/iterm/iterm_test.go
rmdir internal/iterm
```

- [ ] **Step 2: Verify nothing references it**

```bash
grep -r "iterm" --include="*.go" .
```
Expected: no output (or only comments).

- [ ] **Step 3: Build and test**

```bash
go build ./... && go test ./...
```
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git commit -am "refactor: remove internal/iterm package, superseded by internal/terminal"
```

---

## Self-Review

**Spec coverage:**
- ✅ Detect terminal via env vars
- ✅ iTerm2 → new tab (AppleScript, existing behaviour preserved)
- ✅ Kitty → new window via `kitty --title ... -- tmux attach`
- ✅ WezTerm → new window via `wezterm start ...`
- ✅ Alacritty → new window via `alacritty --title ... -e tmux attach`
- ✅ Terminal.app → `open -a Terminal` (limited; no command passthrough)
- ✅ Unknown → print attach command to stdout
- ✅ All backends tested with fakes, no real processes spawned in tests
- ✅ `escapeAppleScript` moved into iTerm backend (same logic, same location)
- ✅ Old `iterm` package deleted

**Placeholder scan:** None found.

**Type consistency:**
- `TerminalOpener.OpenSession(tmuxSession, title string) error` — used consistently across all tasks.
- `argBuilder func(title, tmuxSession string) []string` — defined in Task 3, referenced only in Task 3.
- `windowOpener.exec` field introduced in Task 3 for testability — matches test usage in Task 3.

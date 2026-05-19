// Package iterm opens iTerm2 tabs that attach to a tmux session.
package iterm

import (
	"fmt"
	"os/exec"
)

type Runner interface {
	Run(script string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(script string) (string, error) {
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	return string(out), err
}

func ExecRunner() Runner { return execRunner{} }

type Client struct {
	Runner Runner
}

func New() *Client { return &Client{Runner: ExecRunner()} }

// OpenTab opens a new iTerm2 tab in the current window and attaches to tmuxSession.
// If title is non-empty, the tab/session name is set to it so iTerm2 displays the
// branch (or other meaningful label) instead of the running process name.
func (c *Client) OpenTab(tmuxSession, title string) error {
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
	_, err := c.Runner.Run(script)
	return err
}

// escapeAppleScript escapes characters that would break a double-quoted AppleScript string.
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

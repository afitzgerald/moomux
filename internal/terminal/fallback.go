package terminal

import "fmt"

// fallbackOpener is used when no supported terminal is detected. It cannot
// open anything itself, so it hands the caller an attach instruction to
// display instead of writing to stdout directly — moomux runs its TUI on
// the alt screen, and writing straight to stdout there corrupts the display.
type fallbackOpener struct{}

func (f *fallbackOpener) OpenSession(tmuxSession, title string) (string, error) {
	return fmt.Sprintf("run: tmux attach -t %s", tmuxSession), nil
}

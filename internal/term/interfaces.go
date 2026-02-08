package term

import (
	"context"
	"time"

	"github.com/rivo/tview"
)

type Pane interface {
	Primitive() tview.Primitive
	Start(ctx context.Context, command []string, cwd string, env []string) error
	StartPlayback(ctx context.Context, frames []PlaybackFrame, loop bool) error
	Stop() error
	Resize(cols, rows int) error
	SendInput(data []byte) error
	BracketedPasteEnabled() bool
	ToggleScrollback()
	Scroll(delta int)
	InScrollback() bool
	Snapshot(width, height int) Snapshot
}

type PlaybackFrame struct {
	After time.Duration
	Data  []byte
}

type Snapshot struct {
	Lines      []string
	CursorX    int
	CursorY    int
	CursorShow bool
	Scrollback bool
}

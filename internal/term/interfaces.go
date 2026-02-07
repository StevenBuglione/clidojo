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
	ToggleScrollback()
	Scroll(delta int)
	InScrollback() bool
}

type PlaybackFrame struct {
	After time.Duration
	Data  []byte
}

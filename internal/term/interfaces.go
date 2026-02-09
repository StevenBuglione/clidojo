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

// MetricsProvider exposes lightweight terminal runtime metrics for dev/debug UIs.
type MetricsProvider interface {
	TotalOutputBytes() int64
}

// FrameSnapshotter provides cell-level terminal snapshots for richer renderers.
// Implementations should always return exactly W*H cells.
type FrameSnapshotter interface {
	SnapshotFrame(width, height int) Frame
}

type PlaybackFrame struct {
	After time.Duration
	Data  []byte
}

type Snapshot struct {
	Lines       []string
	StyledLines []string
	CursorX     int
	CursorY     int
	CursorShow  bool
	Scrollback  bool
}

type CellStyle struct {
	FG        int
	BG        int
	FGDefault bool
	BGDefault bool
	Bold      bool
	Underline bool
	Dim       bool
}

type FrameCell struct {
	Ch    rune
	Style CellStyle
}

type Frame struct {
	W, H       int
	Cells      []FrameCell
	CursorX    int
	CursorY    int
	CursorShow bool
	Scrollback bool
}

func (f Frame) Cell(x, y int) FrameCell {
	if x < 0 || y < 0 || x >= f.W || y >= f.H {
		return FrameCell{
			Ch: ' ',
			Style: CellStyle{
				FGDefault: true,
				BGDefault: true,
			},
		}
	}
	idx := y*f.W + x
	if idx < 0 || idx >= len(f.Cells) {
		return FrameCell{
			Ch: ' ',
			Style: CellStyle{
				FGDefault: true,
				BGDefault: true,
			},
		}
	}
	return f.Cells[idx]
}

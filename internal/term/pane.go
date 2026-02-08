package term

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
	"github.com/rivo/tview"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

type TerminalPane struct {
	*tview.Box

	mu sync.Mutex

	vt    vt10x.Terminal
	cmd   *exec.Cmd
	ptmx  *os.File
	cols  int
	rows  int
	dirty func()

	playingBack  bool
	playbackStop context.CancelFunc

	scrollback      []string
	scrollbackMax   int
	inScrollback    bool
	scrollbackIndex int
	lineTail        string
}

func NewTerminalPane(onDirty func()) *TerminalPane {
	return &TerminalPane{
		Box:           tview.NewBox().SetTitle(" Terminal ").SetBorder(true),
		dirty:         onDirty,
		scrollbackMax: 10000,
		cols:          80,
		rows:          24,
	}
}

func (p *TerminalPane) Primitive() tview.Primitive { return p }

// SetDirty updates the redraw callback used when terminal output changes.
func (p *TerminalPane) SetDirty(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dirty = fn
}

func (p *TerminalPane) Start(ctx context.Context, command []string, cwd string, env []string) error {
	if len(command) == 0 {
		return errors.New("terminal command is empty")
	}

	if err := p.Stop(); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.cmd = cmd
	p.ptmx = ptmx
	p.playingBack = false
	p.stopPlaybackLocked()
	p.vt = vt10x.New(vt10x.WithWriter(ptmx), vt10x.WithSize(max(1, p.cols), max(1, p.rows)))
	p.scrollback = nil
	p.inScrollback = false
	p.scrollbackIndex = 0
	p.lineTail = ""
	_ = vt10x.ResizePty(ptmx, max(1, p.cols), max(1, p.rows))
	p.mu.Unlock()

	go p.readLoop()
	go func() {
		_ = cmd.Wait()
		p.mu.Lock()
		_ = p.closePTYLocked()
		p.mu.Unlock()
		p.markDirty()
	}()

	p.markDirty()
	return nil
}

func (p *TerminalPane) StartPlayback(ctx context.Context, frames []PlaybackFrame, loop bool) error {
	if len(frames) == 0 {
		return errors.New("playback frames are empty")
	}
	if err := p.Stop(); err != nil {
		return err
	}

	playCtx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	p.cmd = nil
	p.ptmx = nil
	p.playingBack = true
	p.playbackStop = cancel
	p.vt = vt10x.New(vt10x.WithWriter(io.Discard), vt10x.WithSize(max(1, p.cols), max(1, p.rows)))
	p.scrollback = nil
	p.inScrollback = false
	p.scrollbackIndex = 0
	p.lineTail = ""
	p.mu.Unlock()

	go p.playbackLoop(playCtx, frames, loop)
	p.markDirty()
	return nil
}

func (p *TerminalPane) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopPlaybackLocked()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
	}
	p.cmd = nil
	return p.closePTYLocked()
}

func (p *TerminalPane) stopPlaybackLocked() {
	if p.playbackStop != nil {
		p.playbackStop()
		p.playbackStop = nil
	}
	p.playingBack = false
}

func (p *TerminalPane) closePTYLocked() error {
	if p.ptmx != nil {
		err := p.ptmx.Close()
		p.ptmx = nil
		return err
	}
	return nil
}

func (p *TerminalPane) readLoop() {
	buf := make([]byte, 8192)
	for {
		p.mu.Lock()
		ptmx := p.ptmx
		vt := p.vt
		p.mu.Unlock()

		if ptmx == nil || vt == nil {
			return
		}

		n, err := ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			p.mu.Lock()
			_, _ = vt.Write(chunk)
			p.appendScrollbackLocked(chunk)
			p.mu.Unlock()
			p.markDirty()
		}
		if err != nil {
			return
		}
	}
}

func (p *TerminalPane) appendScrollbackLocked(chunk []byte) {
	plain := ansiPattern.ReplaceAllString(string(chunk), "")
	plain = strings.ReplaceAll(plain, "\r", "")
	if plain == "" {
		return
	}
	plain = p.lineTail + plain
	parts := strings.Split(plain, "\n")
	if len(parts) == 1 {
		p.lineTail = parts[0]
		return
	}
	p.lineTail = parts[len(parts)-1]
	for _, line := range parts[:len(parts)-1] {
		p.scrollback = append(p.scrollback, line)
	}
	if len(p.scrollback) > p.scrollbackMax {
		over := len(p.scrollback) - p.scrollbackMax
		p.scrollback = p.scrollback[over:]
	}
	if p.inScrollback {
		p.scrollbackIndex = len(p.scrollback)
	}
}

func (p *TerminalPane) markDirty() {
	if p.dirty != nil {
		p.dirty()
	}
}

func (p *TerminalPane) Resize(cols, rows int) error {
	p.mu.Lock()
	p.cols = cols
	p.rows = rows
	if p.vt != nil {
		p.vt.Resize(max(1, cols), max(1, rows))
	}
	if p.ptmx != nil {
		if err := vt10x.ResizePty(p.ptmx, max(1, cols), max(1, rows)); err != nil {
			p.mu.Unlock()
			return err
		}
	}
	p.mu.Unlock()
	p.markDirty()
	return nil
}

func (p *TerminalPane) SendInput(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inScrollback {
		return nil
	}
	if p.playingBack {
		if p.vt != nil {
			_, _ = p.vt.Write(data)
			p.appendScrollbackLocked(data)
			p.markDirty()
		}
		return nil
	}
	if p.ptmx == nil {
		return nil
	}
	_, err := p.ptmx.Write(data)
	return err
}

func (p *TerminalPane) ToggleScrollback() {
	p.mu.Lock()
	p.inScrollback = !p.inScrollback
	if p.inScrollback {
		p.scrollbackIndex = len(p.scrollback)
	}
	p.mu.Unlock()
	p.markDirty()
}

func (p *TerminalPane) InScrollback() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inScrollback
}

func (p *TerminalPane) Scroll(delta int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.inScrollback {
		return
	}
	p.scrollbackIndex += delta
	if p.scrollbackIndex < 0 {
		p.scrollbackIndex = 0
	}
	if p.scrollbackIndex > len(p.scrollback) {
		p.scrollbackIndex = len(p.scrollback)
	}
	p.markDirty()
}

func (p *TerminalPane) Draw(screen tcell.Screen) {
	p.Box.DrawForSubclass(screen, p)
	x, y, width, height := p.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.inScrollback {
		p.drawScrollbackLocked(screen, x, y, width, height)
		return
	}

	if p.vt == nil {
		drawTextLine(screen, x, y, width, "No terminal session", tcell.StyleDefault.Foreground(tcell.ColorYellow))
		return
	}

	p.vt.Lock()
	defer p.vt.Unlock()

	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			g := p.vt.Cell(col, row)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			style := tcell.StyleDefault.Foreground(vtColorToCell(g.FG, true)).Background(vtColorToCell(g.BG, false))
			screen.SetContent(x+col, y+row, ch, nil, style)
		}
	}

	if p.vt.CursorVisible() {
		cur := p.vt.Cursor()
		if cur.X >= 0 && cur.X < width && cur.Y >= 0 && cur.Y < height {
			g := p.vt.Cell(cur.X, cur.Y)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			style := tcell.StyleDefault.Foreground(vtColorToCell(g.BG, false)).Background(vtColorToCell(g.FG, true))
			screen.SetContent(x+cur.X, y+cur.Y, ch, nil, style)
		}
	}
}

func (p *TerminalPane) drawScrollbackLocked(screen tcell.Screen, x, y, width, height int) {
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			screen.SetContent(x+col, y+row, ' ', nil, tcell.StyleDefault)
		}
	}
	start := p.scrollbackIndex - height
	if start < 0 {
		start = 0
	}
	lines := p.scrollback[start:p.scrollbackIndex]
	for row, line := range lines {
		drawTextLine(screen, x, y+row, width, line, tcell.StyleDefault)
	}
	indicator := "SCROLLBACK"
	drawTextLine(screen, x+max(0, width-len(indicator)-1), y, len(indicator), indicator, tcell.StyleDefault.Foreground(tcell.ColorYellow))
}

func (p *TerminalPane) Focus(delegate func(p tview.Primitive)) {
	_ = delegate
}

func (p *TerminalPane) Blur() {}

func (p *TerminalPane) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return p.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {})
}

func (p *TerminalPane) playbackLoop(ctx context.Context, frames []PlaybackFrame, loop bool) {
	for {
		for _, frame := range frames {
			if frame.After > 0 {
				timer := time.NewTimer(frame.After)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			} else {
				select {
				case <-ctx.Done():
					return
				default:
				}
			}

			p.mu.Lock()
			if p.vt != nil {
				_, _ = p.vt.Write(frame.Data)
				p.appendScrollbackLocked(frame.Data)
			}
			p.mu.Unlock()
			p.markDirty()
		}
		if !loop {
			return
		}
	}
}

func drawTextLine(screen tcell.Screen, x, y, width int, text string, style tcell.Style) {
	if width <= 0 {
		return
	}
	runes := []rune(text)
	for i := 0; i < width; i++ {
		ch := ' '
		if i < len(runes) {
			ch = runes[i]
		}
		screen.SetContent(x+i, y, ch, nil, style)
	}
}

func vtColorToCell(c vt10x.Color, fg bool) tcell.Color {
	if c == vt10x.DefaultFG || c == vt10x.DefaultBG || c == vt10x.DefaultCursor {
		if fg {
			return tcell.ColorWhite
		}
		return tcell.ColorBlack
	}
	return tcell.PaletteColor(int(c))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

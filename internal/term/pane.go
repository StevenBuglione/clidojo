package term

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
	"github.com/rivo/tview"
)

const (
	bracketedPasteOnSeq  = "\x1b[?2004h"
	bracketedPasteOffSeq = "\x1b[?2004l"
	modeTailMaxLen       = 64
)

type TerminalPane struct {
	*tview.Box

	mu   sync.Mutex
	ioMu sync.Mutex

	vt    vt10x.Terminal
	cmd   *exec.Cmd
	ptmx  *os.File
	cols  int
	rows  int
	dirty func()

	playingBack  bool
	playbackStop context.CancelFunc

	scrollback        []string
	scrollbackMax     int
	inScrollback      bool
	scrollbackIndex   int
	lineTail          string
	modeTail          string
	bracketedPaste    bool
	captureScrollback bool
	totalOutputBytes  atomic.Int64
}

func NewTerminalPane(onDirty func()) *TerminalPane {
	return &TerminalPane{
		Box:               tview.NewBox().SetTitle(" Terminal ").SetBorder(true),
		dirty:             onDirty,
		scrollbackMax:     10000,
		cols:              80,
		rows:              24,
		captureScrollback: false,
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
	p.modeTail = ""
	p.bracketedPaste = false
	p.captureScrollback = false
	p.totalOutputBytes.Store(0)
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
	p.modeTail = ""
	p.bracketedPaste = false
	p.captureScrollback = false
	p.totalOutputBytes.Store(0)
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
			p.totalOutputBytes.Add(int64(n))

			p.mu.Lock()
			captureScrollback := p.captureScrollback || p.inScrollback
			p.updateModesLocked(chunk)
			p.mu.Unlock()

			_, _ = vt.Write(chunk)

			if captureScrollback {
				plainChunk := stripForScrollback(chunk)
				p.mu.Lock()
				p.appendScrollbackPlainLocked(plainChunk)
				p.mu.Unlock()
			}
			p.markDirty()
		}
		if err != nil {
			return
		}
	}
}

// TotalOutputBytes returns a monotonic counter of PTY output bytes processed.
func (p *TerminalPane) TotalOutputBytes() int64 {
	return p.totalOutputBytes.Load()
}

func (p *TerminalPane) appendScrollbackPlainLocked(plain string) {
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
	vt := p.vt
	ptmx := p.ptmx
	p.mu.Unlock()

	if vt != nil {
		vt.Resize(max(1, cols), max(1, rows))
	}
	if ptmx != nil {
		if err := vt10x.ResizePty(ptmx, max(1, cols), max(1, rows)); err != nil {
			return err
		}
	}
	p.markDirty()
	return nil
}

func (p *TerminalPane) SendInput(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	p.mu.Lock()
	inScrollback := p.inScrollback
	playingBack := p.playingBack
	vt := p.vt
	ptmx := p.ptmx
	p.mu.Unlock()

	if inScrollback {
		return nil
	}
	if playingBack {
		if vt == nil {
			return nil
		}
		_, _ = vt.Write(data)
		plain := stripForScrollback(data)
		p.mu.Lock()
		if p.captureScrollback || p.inScrollback {
			p.appendScrollbackPlainLocked(plain)
		}
		p.mu.Unlock()
		p.markDirty()
		return nil
	}
	if ptmx == nil {
		return nil
	}
	p.ioMu.Lock()
	_, err := ptmx.Write(data)
	p.ioMu.Unlock()
	return err
}

func (p *TerminalPane) BracketedPasteEnabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.bracketedPaste
}

func (p *TerminalPane) ToggleScrollback() {
	p.mu.Lock()
	p.inScrollback = !p.inScrollback
	if p.inScrollback {
		p.captureScrollback = true
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
	if p.inScrollback {
		lines := p.scrollbackWindowLocked(height)
		p.mu.Unlock()
		p.drawScrollback(screen, x, y, width, height, lines)
		return
	}
	vt := p.vt
	p.mu.Unlock()

	if vt == nil {
		drawTextLine(screen, x, y, width, "No terminal session", tcell.StyleDefault.Foreground(tcell.ColorYellow))
		return
	}

	vt.Lock()
	defer vt.Unlock()

	vtCols, vtRows := vt.Size()
	drawW := min(width, max(0, vtCols))
	drawH := min(height, max(0, vtRows))

	// Clear the viewport first, then draw only the vt-visible area.
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			screen.SetContent(x+col, y+row, ' ', nil, tcell.StyleDefault)
		}
	}

	for row := 0; row < drawH; row++ {
		for col := 0; col < drawW; col++ {
			g, ok := p.safeCell(vt, col, row)
			if !ok {
				continue
			}
			ch := sanitizeGlyphRune(g.Char)
			style := tcell.StyleDefault.Foreground(vtColorToCell(g.FG, true)).Background(vtColorToCell(g.BG, false))
			screen.SetContent(x+col, y+row, ch, nil, style)
		}
	}

	if vt.CursorVisible() {
		cur := vt.Cursor()
		if cur.X >= 0 && cur.X < drawW && cur.Y >= 0 && cur.Y < drawH {
			g, ok := p.safeCell(vt, cur.X, cur.Y)
			if !ok {
				return
			}
			ch := sanitizeGlyphRune(g.Char)
			style := tcell.StyleDefault.Foreground(vtColorToCell(g.BG, false)).Background(vtColorToCell(g.FG, true))
			screen.SetContent(x+cur.X, y+cur.Y, ch, nil, style)
		}
	}
}

// SnapshotFrame returns a cell-level snapshot of the visible terminal.
// It is used by Bubble Tea renderers to preserve terminal colors and cursor.
func (p *TerminalPane) SnapshotFrame(width, height int) Frame {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	out := Frame{
		W:          width,
		H:          height,
		Cells:      make([]FrameCell, width*height),
		CursorX:    -1,
		CursorY:    -1,
		CursorShow: false,
	}
	def := CellStyle{FGDefault: true, BGDefault: true}
	for i := range out.Cells {
		out.Cells[i] = FrameCell{Ch: ' ', Style: def}
	}

	p.mu.Lock()
	if p.inScrollback {
		lines := p.scrollbackWindowLocked(height)
		p.mu.Unlock()
		out.Scrollback = true
		for row := 0; row < height && row < len(lines); row++ {
			col := 0
			for _, ch := range []rune(lines[row]) {
				if col >= width {
					break
				}
				out.Cells[row*width+col] = FrameCell{Ch: sanitizeGlyphRune(ch), Style: def}
				col++
			}
		}
		return out
	}
	vt := p.vt
	p.mu.Unlock()

	if vt == nil {
		msg := []rune("No terminal session")
		for i := 0; i < len(msg) && i < width; i++ {
			out.Cells[i] = FrameCell{Ch: msg[i], Style: def}
		}
		return out
	}

	vt.Lock()
	defer vt.Unlock()

	vtCols, vtRows := vt.Size()
	drawW := min(width, max(0, vtCols))
	drawH := min(height, max(0, vtRows))
	for row := 0; row < drawH; row++ {
		for col := 0; col < drawW; col++ {
			g, ok := p.safeCell(vt, col, row)
			if !ok {
				continue
			}
			out.Cells[row*width+col] = FrameCell{
				Ch:    sanitizeGlyphRune(g.Char),
				Style: glyphToCellStyle(g),
			}
		}
	}

	if vt.CursorVisible() {
		cur := vt.Cursor()
		if cur.X >= 0 && cur.X < width && cur.Y >= 0 && cur.Y < height {
			out.CursorX = cur.X
			out.CursorY = cur.Y
			out.CursorShow = true
		}
	}

	return out
}

func (p *TerminalPane) scrollbackWindowLocked(height int) []string {
	if height <= 0 {
		return nil
	}
	start := p.scrollbackIndex - height
	if start < 0 {
		start = 0
	}
	if p.scrollbackIndex > len(p.scrollback) {
		p.scrollbackIndex = len(p.scrollback)
	}
	lines := append([]string(nil), p.scrollback[start:p.scrollbackIndex]...)
	return lines
}

func (p *TerminalPane) drawScrollback(screen tcell.Screen, x, y, width, height int, lines []string) {
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			screen.SetContent(x+col, y+row, ' ', nil, tcell.StyleDefault)
		}
	}
	for row, line := range lines {
		drawTextLine(screen, x, y+row, width, line, tcell.StyleDefault)
	}
	indicator := "SCROLLBACK"
	drawTextLine(screen, x+max(0, width-len(indicator)-1), y, len(indicator), indicator, tcell.StyleDefault.Foreground(tcell.ColorYellow))
}

// Snapshot returns a text snapshot of the current terminal view with optional
// cursor metadata. It is intended for renderer-agnostic UI layers.
func (p *TerminalPane) Snapshot(width, height int) Snapshot {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	out := Snapshot{
		Lines:       make([]string, height),
		StyledLines: make([]string, height),
		CursorX:     -1,
		CursorY:     -1,
		CursorShow:  false,
	}

	p.mu.Lock()
	if p.inScrollback {
		lines := p.scrollbackWindowLocked(height)
		p.mu.Unlock()
		out.Scrollback = true
		for row := 0; row < height; row++ {
			if row < len(lines) {
				out.Lines[row] = clipWidth(lines[row], width)
				out.StyledLines[row] = out.Lines[row]
			} else {
				out.Lines[row] = strings.Repeat(" ", width)
				out.StyledLines[row] = out.Lines[row]
			}
		}
		return out
	}
	vt := p.vt
	p.mu.Unlock()

	if vt == nil {
		out.Lines[0] = clipWidth("No terminal session", width)
		out.StyledLines[0] = out.Lines[0]
		for row := 1; row < height; row++ {
			out.Lines[row] = strings.Repeat(" ", width)
			out.StyledLines[row] = out.Lines[row]
		}
		return out
	}

	vt.Lock()
	defer vt.Unlock()

	vtCols, vtRows := vt.Size()
	drawW := min(width, max(0, vtCols))
	drawH := min(height, max(0, vtRows))

	for row := 0; row < height; row++ {
		buf := make([]rune, width)
		for i := range buf {
			buf[i] = ' '
		}
		var styled strings.Builder
		var prev vtRenderStyle
		hasStyle := false
		if row < drawH {
			for col := 0; col < drawW; col++ {
				g, ok := p.safeCell(vt, col, row)
				if !ok {
					continue
				}
				ch := sanitizeGlyphRune(g.Char)
				buf[col] = ch
				style := vtRenderStyleFromGlyph(g)
				if !hasStyle || !style.equal(prev) {
					styled.WriteString(style.sgr())
					prev = style
					hasStyle = true
				}
				styled.WriteRune(ch)
			}
			for col := drawW; col < width; col++ {
				spaceStyle := vtRenderStyleDefault()
				if !hasStyle || !spaceStyle.equal(prev) {
					styled.WriteString(spaceStyle.sgr())
					prev = spaceStyle
					hasStyle = true
				}
				styled.WriteRune(' ')
			}
		} else {
			styled.WriteString(strings.Repeat(" ", width))
		}
		if hasStyle {
			styled.WriteString("\x1b[0m")
		}
		out.Lines[row] = string(buf)
		out.StyledLines[row] = styled.String()
	}

	if vt.CursorVisible() {
		cur := vt.Cursor()
		if cur.X >= 0 && cur.X < width && cur.Y >= 0 && cur.Y < height {
			out.CursorX = cur.X
			out.CursorY = cur.Y
			out.CursorShow = true
		}
	}
	return out
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
				p.updateModesLocked(frame.Data)
				_, _ = p.vt.Write(frame.Data)
				if p.captureScrollback || p.inScrollback {
					p.appendScrollbackPlainLocked(stripForScrollback(frame.Data))
				}
			}
			p.mu.Unlock()
			p.markDirty()
		}
		if !loop {
			return
		}
	}
}

func (p *TerminalPane) updateModesLocked(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	state := p.modeTail + string(chunk)
	lastOn := strings.LastIndex(state, bracketedPasteOnSeq)
	lastOff := strings.LastIndex(state, bracketedPasteOffSeq)
	if lastOn >= 0 || lastOff >= 0 {
		p.bracketedPaste = lastOn > lastOff
	}
	if len(state) > modeTailMaxLen {
		state = state[len(state)-modeTailMaxLen:]
	}
	p.modeTail = state
}

func stripForScrollback(chunk []byte) string {
	plain := xansi.Strip(string(chunk))
	plain = strings.ReplaceAll(plain, "\r", "")
	return plain
}

const (
	vtAttrReverse   int16 = 1 << 0
	vtAttrUnderline int16 = 1 << 1
	vtAttrBold      int16 = 1 << 2
)

type vtRenderStyle struct {
	FG        vt10x.Color
	BG        vt10x.Color
	Bold      bool
	Underline bool
}

func vtRenderStyleDefault() vtRenderStyle {
	return vtRenderStyle{FG: vt10x.DefaultFG, BG: vt10x.DefaultBG}
}

func vtRenderStyleFromGlyph(g vt10x.Glyph) vtRenderStyle {
	style := vtRenderStyle{
		FG:        g.FG,
		BG:        g.BG,
		Bold:      g.Mode&vtAttrBold != 0,
		Underline: g.Mode&vtAttrUnderline != 0,
	}
	if g.Mode&vtAttrReverse != 0 {
		style.FG, style.BG = style.BG, style.FG
	}
	return style
}

func (s vtRenderStyle) equal(other vtRenderStyle) bool {
	return s.FG == other.FG &&
		s.BG == other.BG &&
		s.Bold == other.Bold &&
		s.Underline == other.Underline
}

func (s vtRenderStyle) sgr() string {
	codes := []string{"0"}
	if s.Bold {
		codes = append(codes, "1")
	}
	if s.Underline {
		codes = append(codes, "4")
	}
	codes = append(codes, vtColorToSGR(s.FG, true))
	codes = append(codes, vtColorToSGR(s.BG, false))
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

func vtColorToSGR(c vt10x.Color, foreground bool) string {
	if c == vt10x.DefaultFG || c == vt10x.DefaultBG || c == vt10x.DefaultCursor {
		if foreground {
			return "39"
		}
		return "49"
	}
	n := int(c)
	if n >= 0 && n < 8 {
		if foreground {
			return strconv.Itoa(30 + n)
		}
		return strconv.Itoa(40 + n)
	}
	if n >= 8 && n < 16 {
		if foreground {
			return strconv.Itoa(90 + (n - 8))
		}
		return strconv.Itoa(100 + (n - 8))
	}
	if foreground {
		return "38;5;" + strconv.Itoa(n)
	}
	return "48;5;" + strconv.Itoa(n)
}

func clipWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if s == "" {
		return strings.Repeat(" ", w)
	}
	r := []rune(s)
	if len(r) > w {
		r = r[:w]
	}
	if len(r) < w {
		r = append(r, []rune(strings.Repeat(" ", w-len(r)))...)
	}
	return string(r)
}

func sanitizeGlyphRune(ch rune) rune {
	if ch == 0 || ch == utf8.RuneError || !utf8.ValidRune(ch) {
		return ' '
	}
	switch ch {
	case '\u25a1', '\u25a0', '\u25af', '\u2423', '\u2400':
		return ' '
	}
	// Drop private-use glyphs frequently emitted as tofu boxes by browser terminal fonts.
	if (ch >= 0xE000 && ch <= 0xF8FF) ||
		(ch >= 0xF0000 && ch <= 0xFFFFD) ||
		(ch >= 0x100000 && ch <= 0x10FFFD) {
		return ' '
	}
	if ch < 0x20 || ch == 0x7f {
		return ' '
	}
	if unicode.IsControl(ch) {
		return ' '
	}
	return ch
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

func (p *TerminalPane) safeCell(vt vt10x.Terminal, col, row int) (g vt10x.Glyph, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	if vt == nil {
		return g, false
	}
	return vt.Cell(col, row), true
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

func glyphToCellStyle(g vt10x.Glyph) CellStyle {
	style := CellStyle{
		Bold:      g.Mode&vtAttrBold != 0,
		Underline: g.Mode&vtAttrUnderline != 0,
	}
	if g.FG == vt10x.DefaultFG || g.FG == vt10x.DefaultBG || g.FG == vt10x.DefaultCursor {
		style.FGDefault = true
	} else {
		style.FG = int(g.FG)
	}
	if g.BG == vt10x.DefaultFG || g.BG == vt10x.DefaultBG || g.BG == vt10x.DefaultCursor {
		style.BGDefault = true
	} else {
		style.BG = int(g.BG)
	}
	if g.Mode&vtAttrReverse != 0 {
		style.FG, style.BG = style.BG, style.FG
		style.FGDefault, style.BGDefault = style.BGDefault, style.FGDefault
	}
	return style
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

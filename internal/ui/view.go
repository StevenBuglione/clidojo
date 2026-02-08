package ui

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"clidojo/internal/term"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/harmonica"
	clog "github.com/charmbracelet/log"
	"github.com/charmbracelet/x/ansi"
)

type applyMsg struct {
	fn func(*Root)
}

type drawMsg struct{}
type clockMsg time.Time
type animateMsg time.Time

type gameKeyMap struct {
	Hints      key.Binding
	Goal       key.Binding
	Journal    key.Binding
	Check      key.Binding
	Reset      key.Binding
	Scrollback key.Binding
	Menu       key.Binding
}

func (k gameKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Hints, k.Goal, k.Journal, k.Check, k.Reset, k.Scrollback, k.Menu}
}

func (k gameKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Hints, k.Goal, k.Journal, k.Check}, {k.Reset, k.Scrollback, k.Menu}}
}

type Root struct {
	theme        Theme
	ascii        bool
	debug        bool
	term         term.Pane
	ctrl         Controller
	styleVariant string
	motionLevel  string
	mouseScope   string

	mu      sync.Mutex
	program *tea.Program
	running bool

	screen Screen
	layout LayoutMode
	cols   int
	rows   int

	forceTooSmall bool
	tooSmallCols  int
	tooSmallRows  int

	state         PlayingState
	mainMenu      MainMenuState
	catalog       []PackSummary
	selectedPack  string
	selectedLevel string
	result        ResultState
	setupMsg      string
	setupDetails  string
	statusFlash   string
	checking      bool

	journalEntries []JournalEntry
	referenceText  string
	diffText       string
	infoTitle      string
	infoText       string

	menuOpen      bool
	hintsOpen     bool
	goalOpen      bool
	journalOpen   bool
	resetOpen     bool
	infoOpen      bool
	referenceOpen bool
	diffOpen      bool

	mainMenuIndex int
	packIndex     int
	levelIndex    int
	catalogFocus  int
	menuIndex     int
	resetIndex    int
	resultIndex   int
	journalIndex  int

	help       help.Model
	keymap     gameKeyMap
	mastery    progress.Model
	checkSpin  spinner.Model
	markdown   *glamour.TermRenderer
	logger     *clog.Logger
	overlayPos float64
	overlayVel float64
	spring     harmonica.Spring

	drawPending atomic.Bool

	termCursorX    int
	termCursorY    int
	termCursorShow bool

	lastInputEvent string
}

type Options struct {
	ASCIIOnly    bool
	Debug        bool
	TermPane     term.Pane
	StyleVariant string
	MotionLevel  string
	MouseScope   string
}

func New(opts Options) *Root {
	logger := clog.NewWithOptions(os.Stderr, clog.Options{Prefix: "clidojo-ui", Level: clog.WarnLevel})
	if opts.Debug {
		logger.SetLevel(clog.DebugLevel)
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(78),
	)
	if err != nil {
		renderer = nil
	}

	h := help.New()
	h.Styles = help.DefaultDarkStyles()
	motionLevel := normalizeMotionLevel(opts.MotionLevel)
	mouseScope := normalizeMouseScope(opts.MouseScope)
	styleVariant := normalizeStyleVariant(opts.StyleVariant)
	theme := ThemeForVariant(styleVariant)
	spring := harmonica.NewSpring(harmonica.FPS(60), 10.0, 0.8)
	switch motionLevel {
	case "reduced":
		spring = harmonica.NewSpring(harmonica.FPS(30), 9.0, 0.92)
	case "off":
		spring = harmonica.NewSpring(harmonica.FPS(60), 1000.0, 1.0)
	}
	mastery := progress.New(
		progress.WithWidth(20),
		progress.WithColors(lipgloss.Color("#5EC2FF"), lipgloss.Color("#79E6A6"), lipgloss.Color("#F2D16B")),
		progress.WithScaled(true),
	)
	if motionLevel == "off" {
		mastery.SetSpringOptions(1000.0, 1.0)
	}
	checkSpin := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(theme.Accent),
	)

	r := &Root{
		theme:        theme,
		ascii:        opts.ASCIIOnly,
		debug:        opts.Debug,
		term:         opts.TermPane,
		styleVariant: styleVariant,
		motionLevel:  motionLevel,
		mouseScope:   mouseScope,
		screen:       ScreenMainMenu,
		layout:       LayoutWide,
		cols:         120,
		rows:         30,
		help:         h,
		mastery:      mastery,
		checkSpin:    checkSpin,
		markdown:     renderer,
		logger:       logger,
		spring:       spring,
		state: PlayingState{
			ModeLabel: "Free Play",
			StartedAt: time.Now(),
			HudWidth:  42,
		},
	}
	r.keymap = gameKeyMap{
		Hints:      key.NewBinding(key.WithKeys("f1"), key.WithHelp("F1", "Hints")),
		Goal:       key.NewBinding(key.WithKeys("f2"), key.WithHelp("F2", "Goal")),
		Journal:    key.NewBinding(key.WithKeys("f4"), key.WithHelp("F4", "Journal")),
		Check:      key.NewBinding(key.WithKeys("f5"), key.WithHelp("F5", "Check")),
		Reset:      key.NewBinding(key.WithKeys("f6"), key.WithHelp("F6", "Reset")),
		Scrollback: key.NewBinding(key.WithKeys("f9"), key.WithHelp("F9", "Scrollback")),
		Menu:       key.NewBinding(key.WithKeys("f10"), key.WithHelp("F10", "Menu")),
	}
	return r
}

func (r *Root) Init() tea.Cmd {
	return tea.Batch(clockTickCmd(), animateTickCmd(), spinnerTickCmd(r.checkSpin))
}

func (r *Root) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	defer func() {
		if rec := recover(); rec != nil {
			r.onModelPanic("update", rec, msg)
			model = r
			cmd = nil
		}
	}()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.cols = msg.Width
		r.rows = msg.Height
		r.layout = DetermineLayoutMode(r.cols, r.rows)
		if r.layout != LayoutTooSmall {
			r.forceTooSmall = false
		}
		if r.screen == ScreenPlaying {
			r.dispatchController(func(c Controller) { c.OnResize(msg.Width, msg.Height) })
		}
		return r, nil
	case applyMsg:
		if msg.fn != nil {
			msg.fn(r)
		}
		return r, r.animateIfNeeded()
	case drawMsg:
		r.drawPending.Store(false)
		return r, nil
	case clockMsg:
		return r, clockTickCmd()
	case animateMsg:
		target := 0.0
		if r.goalOpen {
			target = 1.0
		}
		r.overlayPos, r.overlayVel = r.spring.Update(r.overlayPos, r.overlayVel, target)
		if r.shouldAnimate(target) {
			return r, animateTickCmd()
		}
		if target == 0 {
			r.overlayPos = 0
			r.overlayVel = 0
		} else {
			r.overlayPos = 1
			r.overlayVel = 0
		}
		return r, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		r.checkSpin, cmd = r.checkSpin.Update(msg)
		return r, cmd
	case tea.PasteMsg:
		return r.handlePaste(msg)
	case tea.ClipboardMsg:
		return r.handlePaste(tea.PasteMsg{Content: msg.Content})
	case tea.MouseClickMsg:
		return r.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return r.handleMouseWheel(msg)
	case tea.KeyPressMsg:
		return r.handleKey(msg)
	}
	return r, nil
}

func (r *Root) View() (view tea.View) {
	defer func() {
		if rec := recover(); rec != nil {
			r.onModelPanic("view", rec, nil)
			width := max(1, r.cols)
			msg := "UI recovered from a rendering panic. Check logs."
			if r.statusFlash == "" {
				r.statusFlash = "Recovered UI panic"
			}
			view = tea.NewView(r.theme.Fail.Width(width).Render(trimForWidth(msg, max(1, width-1))))
		}
	}()

	if r.cols < 1 {
		r.cols = 120
	}
	if r.rows < 1 {
		r.rows = 30
	}
	r.termCursorShow = false

	var base string
	switch r.screen {
	case ScreenMainMenu:
		base = r.renderMainMenu()
	case ScreenLevelSelect:
		base = r.renderLevelSelect()
	default:
		base = r.renderPlaying()
	}

	if overlay := r.renderOverlay(); overlay != "" {
		base = composeOverlay(base, overlay, r.cols, r.rows)
	}
	v := tea.NewView(base)
	v.AltScreen = true
	v.MouseMode = r.currentMouseMode()
	if r.termCursorShow && !r.overlayActive() && r.screen == ScreenPlaying {
		v.Cursor = tea.NewCursor(r.termCursorX, r.termCursorY)
	}
	v.DisableBracketedPasteMode = false
	return v
}

func (r *Root) Run() error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return nil
	}
	p := tea.NewProgram(r)
	r.program = p
	r.running = true
	r.mu.Unlock()

	_, err := p.Run()

	r.mu.Lock()
	r.program = nil
	r.running = false
	r.mu.Unlock()
	return err
}

func (r *Root) Stop() {
	r.mu.Lock()
	p := r.program
	r.mu.Unlock()
	if p != nil {
		p.Quit()
	}
}

func (r *Root) SetController(c Controller) {
	r.ctrl = c
}

func (r *Root) SetScreen(screen Screen) {
	r.apply(func(m *Root) {
		m.screen = screen
		if screen == ScreenPlaying {
			if m.state.StartedAt.IsZero() {
				m.state.StartedAt = time.Now()
			}
			cols, rows := m.cols, m.rows
			m.dispatchController(func(c Controller) { c.OnResize(cols, rows) })
		}
	})
}

func (r *Root) SetMainMenuState(state MainMenuState) {
	r.apply(func(m *Root) {
		m.mainMenu = state
	})
}

func (r *Root) SetCatalog(packs []PackSummary) {
	r.apply(func(m *Root) {
		m.catalog = append([]PackSummary(nil), packs...)
		m.syncCatalogSelection()
	})
}

func (r *Root) SetLevelSelection(packID, levelID string) {
	r.apply(func(m *Root) {
		m.selectedPack = packID
		m.selectedLevel = levelID
		m.syncCatalogSelection()
	})
}

func (r *Root) SetPlayingState(s PlayingState) {
	r.apply(func(m *Root) {
		if s.HudWidth <= 0 {
			s.HudWidth = 42
		}
		if s.StartedAt.IsZero() {
			s.StartedAt = time.Now()
		}
		m.state = s
	})
}

func (r *Root) SetTooSmall(cols, rows int) {
	r.apply(func(m *Root) {
		m.forceTooSmall = true
		m.tooSmallCols = cols
		m.tooSmallRows = rows
	})
}

func (r *Root) SetSetupError(msg, details string) {
	r.apply(func(m *Root) {
		m.setupMsg = msg
		m.setupDetails = details
		m.screen = ScreenMainMenu
	})
}

func (r *Root) SetMenuOpen(open bool) {
	r.apply(func(m *Root) {
		m.menuOpen = open
		if !open {
			m.menuIndex = 0
		}
	})
}

func (r *Root) SetHintsOpen(open bool) {
	r.apply(func(m *Root) {
		m.hintsOpen = open
	})
}

func (r *Root) SetGoalOpen(open bool) {
	r.apply(func(m *Root) {
		m.goalOpen = open
		if m.motionLevel == "off" {
			if open {
				m.overlayPos = 1
			} else {
				m.overlayPos = 0
			}
			m.overlayVel = 0
		}
	})
}

func (r *Root) SetJournalOpen(open bool) {
	r.apply(func(m *Root) {
		m.journalOpen = open
		if !open {
			m.journalIndex = 0
		}
	})
}

func (r *Root) SetResetConfirmOpen(open bool) {
	r.apply(func(m *Root) {
		m.resetOpen = open
		if !open {
			m.resetIndex = 0
		}
	})
}

func (r *Root) SetResult(state ResultState) {
	r.apply(func(m *Root) {
		m.result = state
		if !state.Visible {
			m.resultIndex = 0
		}
	})
}

func (r *Root) SetJournalEntries(entries []JournalEntry) {
	r.apply(func(m *Root) {
		m.journalEntries = append([]JournalEntry(nil), entries...)
		if m.journalIndex >= len(m.journalEntries) {
			m.journalIndex = max(0, len(m.journalEntries)-1)
		}
	})
}

func (r *Root) SetReferenceText(text string, open bool) {
	r.apply(func(m *Root) {
		m.referenceText = text
		m.referenceOpen = open
	})
}

func (r *Root) SetDiffText(text string, open bool) {
	r.apply(func(m *Root) {
		m.diffText = text
		m.diffOpen = open
	})
}

func (r *Root) SetInfo(title, text string, open bool) {
	r.apply(func(m *Root) {
		m.infoTitle = title
		m.infoText = text
		m.infoOpen = open
	})
}

func (r *Root) SetChecking(checking bool) {
	r.apply(func(m *Root) {
		m.checking = checking
	})
}

func (r *Root) FlashStatus(msg string) {
	r.apply(func(m *Root) {
		m.statusFlash = msg
	})
}

func (r *Root) RequestDraw() {
	r.mu.Lock()
	p := r.program
	running := r.running
	r.mu.Unlock()
	if !running || p == nil {
		return
	}
	if !r.drawPending.CompareAndSwap(false, true) {
		return
	}
	time.AfterFunc(16*time.Millisecond, func() {
		r.mu.Lock()
		p := r.program
		running := r.running
		r.mu.Unlock()
		if !running || p == nil {
			r.drawPending.Store(false)
			return
		}
		p.Send(drawMsg{})
	})
}

func (r *Root) apply(fn func(*Root)) {
	if fn == nil {
		return
	}
	r.mu.Lock()
	p := r.program
	running := r.running
	if !running || p == nil {
		fn(r)
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
	p.Send(applyMsg{fn: fn})
}

func (r *Root) dispatchController(fn func(Controller)) {
	if fn == nil || r.ctrl == nil {
		return
	}
	ctrl := r.ctrl
	go fn(ctrl)
}

func (r *Root) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r.recordInputEvent(fmt.Sprintf("key:%v mod:%v text:%q", msg.Code, msg.Mod, msg.Text))

	if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+q"))) {
		r.dispatchController(func(c Controller) { c.OnQuit() })
		return r, nil
	}

	if r.overlayActive() {
		return r.handleOverlayKey(msg)
	}

	switch r.screen {
	case ScreenMainMenu:
		return r.handleMainMenuKey(msg)
	case ScreenLevelSelect:
		return r.handleLevelSelectKey(msg)
	default:
		return r.handlePlayingKey(msg)
	}
}

func (r *Root) handlePaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	r.recordInputEvent(fmt.Sprintf("paste:%d", len(msg.Content)))

	if r.screen != ScreenPlaying || r.overlayActive() {
		return r, nil
	}
	if r.term != nil && r.term.InScrollback() {
		r.term.ToggleScrollback()
	}
	if msg.Content == "" {
		return r, nil
	}
	bracketed := false
	if r.term != nil {
		bracketed = r.term.BracketedPasteEnabled()
	}
	content := term.EncodePasteToBytes(msg.Content, bracketed)
	if len(content) == 0 {
		return r, nil
	}
	r.dispatchController(func(c Controller) { c.OnTerminalInput(content) })
	return r, nil
}

func (r *Root) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	r.recordInputEvent(fmt.Sprintf("mouse_click:%d,%d button:%v", mouse.X, mouse.Y, mouse.Button))

	if r.mouseScope == "off" {
		return r, nil
	}
	m := mouse
	if m.Button != tea.MouseLeft {
		return r, nil
	}

	if r.overlayActive() {
		return r.handleOverlayMouseClick(m.X, m.Y)
	}
	if r.mouseScope == "scoped" && r.screen == ScreenPlaying {
		return r, nil
	}
	switch r.screen {
	case ScreenMainMenu:
		return r.handleMainMenuMouseClick(m.X, m.Y)
	case ScreenLevelSelect:
		return r.handleLevelSelectMouseClick(m.X, m.Y)
	}
	return r, nil
}

func (r *Root) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	r.recordInputEvent(fmt.Sprintf("mouse_wheel:%d,%d button:%v", mouse.X, mouse.Y, mouse.Button))

	if r.mouseScope == "off" {
		return r, nil
	}
	m := mouse
	delta := 0
	if m.Button == tea.MouseWheelUp {
		delta = -1
	} else if m.Button == tea.MouseWheelDown {
		delta = 1
	}
	if delta == 0 {
		return r, nil
	}

	if r.overlayActive() && r.topOverlay() == "journal" && len(r.journalEntries) > 0 {
		r.journalIndex += delta
		if r.journalIndex < 0 {
			r.journalIndex = 0
		}
		if r.journalIndex > len(r.journalEntries)-1 {
			r.journalIndex = len(r.journalEntries) - 1
		}
		return r, nil
	}
	if r.term != nil && r.screen == ScreenPlaying && (r.mouseScope == "full" || r.term.InScrollback()) {
		if !r.term.InScrollback() {
			r.term.ToggleScrollback()
		}
		r.term.Scroll(delta * 3)
	}
	return r, nil
}

func (r *Root) handleMainMenuMouseClick(x, y int) (tea.Model, tea.Cmd) {
	items := r.mainMenuItems()
	if len(items) == 0 {
		return r, nil
	}
	leftW := min(36, max(24, r.cols/3))
	if x < 1 || x >= leftW-1 {
		return r, nil
	}
	idx := y - 2
	if idx < 0 || idx >= len(items) {
		return r, nil
	}
	r.mainMenuIndex = idx
	r.activateMainMenuSelection()
	return r, nil
}

func (r *Root) handleLevelSelectMouseClick(x, y int) (tea.Model, tea.Cmd) {
	if y < 2 {
		return r, nil
	}
	leftW := min(34, max(24, r.cols/4))
	middleW := min(46, max(28, r.cols/3))
	idx := y - 2

	if x >= 1 && x < leftW-1 {
		if len(r.catalog) == 0 {
			return r, nil
		}
		r.catalogFocus = 0
		r.packIndex = wrapIndex(idx, len(r.catalog))
		r.syncSelectionFromIndices()
		return r, nil
	}
	if x >= leftW+1 && x < leftW+middleW-1 {
		levels := r.selectedPackLevels()
		if len(levels) == 0 {
			return r, nil
		}
		r.catalogFocus = 1
		r.levelIndex = wrapIndex(idx, len(levels))
		r.syncSelectionFromIndices()
		r.startSelectedLevel()
		return r, nil
	}
	return r, nil
}

func (r *Root) handleOverlayMouseClick(x, y int) (tea.Model, tea.Cmd) {
	top := r.topOverlay()
	spec, ok := r.overlaySpec(top)
	if !ok {
		return r, nil
	}
	if x < spec.startCol+1 || x >= spec.startCol+spec.width-1 || y < spec.startRow+1 || y >= spec.startRow+spec.height-1 {
		return r, nil
	}
	contentRow := y - (spec.startRow + 1)
	switch top {
	case "menu":
		items := r.menuItems()
		if contentRow >= 0 && contentRow < len(items) {
			r.menuIndex = contentRow
			r.activateMenuItem(items[contentRow])
		}
	case "result":
		buttons := r.resultButtons()
		baseRows := len(strings.Split(strings.TrimSuffix(r.resultText(), "\n"), "\n"))
		actionRowStart := baseRows + 2
		row := contentRow - actionRowStart
		if row >= 0 && row < len(buttons) {
			r.resultIndex = row
			r.activateResultButton(buttons[row])
		}
	case "reset":
		// Reset actions are rendered as two selectable rows after prompt text.
		row := contentRow - 2
		if row >= 0 && row <= 1 {
			r.resetIndex = row
			if row == 1 {
				r.resetOpen = false
				r.dispatchController(func(c Controller) { c.OnReset() })
			} else {
				r.resetOpen = false
			}
		}
	case "hints":
		// Click anywhere in hints overlay to reveal next available hint.
		r.dispatchController(func(c Controller) { c.OnRevealHint() })
	case "journal":
		// Click anywhere in journal overlay to trigger explain action.
		r.dispatchController(func(c Controller) { c.OnJournalExplainAI() })
	default:
		_ = x
	}
	return r, nil
}

func (r *Root) handleOverlayKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.Code == tea.KeyF10 {
		if r.topOverlay() == "menu" {
			r.menuOpen = false
			r.dispatchController(func(c Controller) { c.OnMenu() })
			return r, r.animateIfNeeded()
		}
		r.dismissAllOverlays()
		r.menuOpen = true
		r.dispatchController(func(c Controller) { c.OnMenu() })
		return r, r.animateIfNeeded()
	}

	if (msg.Code == 'c' || msg.Code == 'C') && msg.Mod&tea.ModCtrl != 0 {
		text := r.overlayCopyText(true)
		if strings.TrimSpace(text) == "" {
			return r, nil
		}
		r.statusFlash = "Copied overlay text"
		return r, tea.SetClipboard(text)
	}
	if msg.Mod == 0 && (msg.Code == 'y' || msg.Code == 'Y') {
		full := msg.Code == 'Y'
		text := r.overlayCopyText(full)
		if strings.TrimSpace(text) == "" {
			return r, nil
		}
		if full {
			r.statusFlash = "Copied overlay text"
		} else {
			r.statusFlash = "Copied selection"
		}
		return r, tea.SetClipboard(text)
	}

	if msg.Code == tea.KeyEsc || msg.Code == tea.KeyEscape ||
		(msg.Mod == 0 && (msg.Code == 'q' || msg.Code == 'Q')) {
		r.dismissTopOverlay()
		return r, r.animateIfNeeded()
	}

	switch r.topOverlay() {
	case "menu":
		items := r.menuItems()
		switch msg.Code {
		case tea.KeyUp:
			r.menuIndex = wrapIndex(r.menuIndex-1, len(items))
		case tea.KeyDown, tea.KeyTab:
			r.menuIndex = wrapIndex(r.menuIndex+1, len(items))
		case tea.KeyEnter:
			r.activateMenuItem(items[r.menuIndex])
		}
	case "reset":
		switch msg.Code {
		case tea.KeyLeft, tea.KeyUp:
			r.resetIndex = 0
		case tea.KeyRight, tea.KeyDown, tea.KeyTab:
			r.resetIndex = 1
		case tea.KeyEnter:
			if r.resetIndex == 1 {
				r.resetOpen = false
				r.dispatchController(func(c Controller) { c.OnReset() })
			} else {
				r.resetOpen = false
			}
		}
	case "result":
		buttons := r.resultButtons()
		if len(buttons) == 0 {
			r.result = ResultState{}
			return r, r.animateIfNeeded()
		}
		switch msg.Code {
		case tea.KeyUp:
			r.resultIndex = wrapIndex(r.resultIndex-1, len(buttons))
		case tea.KeyDown, tea.KeyTab:
			r.resultIndex = wrapIndex(r.resultIndex+1, len(buttons))
		case tea.KeyEnter:
			r.activateResultButton(buttons[r.resultIndex])
		}
	case "hints":
		switch msg.Code {
		case tea.KeyEnter:
			r.dispatchController(func(c Controller) { c.OnRevealHint() })
		}
	case "journal":
		switch msg.Code {
		case tea.KeyEnter:
			r.dispatchController(func(c Controller) { c.OnJournalExplainAI() })
		}
	}
	return r, nil
}

func (r *Root) dismissTopOverlay() {
	switch r.topOverlay() {
	case "menu":
		r.menuOpen = false
		r.dispatchController(func(c Controller) { c.OnMenu() })
	case "hints":
		r.hintsOpen = false
		r.dispatchController(func(c Controller) { c.OnHints() })
	case "journal":
		r.journalOpen = false
		r.dispatchController(func(c Controller) { c.OnJournal() })
	case "result":
		r.result = ResultState{}
		r.dispatchController(func(c Controller) { c.OnTryAgain() })
	default:
		r.closeTopOverlay()
	}
}

func (r *Root) dismissAllOverlays() {
	for i := 0; i < 8 && r.overlayActive(); i++ {
		r.dismissTopOverlay()
	}
}

func (r *Root) handleMainMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	items := r.mainMenuItems()
	switch msg.Code {
	case tea.KeyUp:
		r.mainMenuIndex = wrapIndex(r.mainMenuIndex-1, len(items))
	case tea.KeyDown, tea.KeyTab:
		r.mainMenuIndex = wrapIndex(r.mainMenuIndex+1, len(items))
	case tea.KeyEnter:
		r.activateMainMenuSelection()
	case tea.KeyEsc:
		r.dispatchController(func(c Controller) { c.OnQuit() })
	}
	return r, nil
}

func (r *Root) handleLevelSelectKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.Code == tea.KeyEsc {
		r.dispatchController(func(c Controller) { c.OnBackToMainMenu() })
		return r, nil
	}
	if msg.Code == tea.KeyTab && msg.Mod&tea.ModShift != 0 {
		r.catalogFocus = 0
		return r, nil
	}

	switch msg.Code {
	case tea.KeyLeft:
		r.catalogFocus = 0
	case tea.KeyRight, tea.KeyTab:
		r.catalogFocus = 1
	case tea.KeyUp:
		if r.catalogFocus == 0 {
			r.packIndex = wrapIndex(r.packIndex-1, len(r.catalog))
			r.syncSelectionFromIndices()
		} else {
			levels := r.selectedPackLevels()
			r.levelIndex = wrapIndex(r.levelIndex-1, len(levels))
			r.syncSelectionFromIndices()
		}
	case tea.KeyDown:
		if r.catalogFocus == 0 {
			r.packIndex = wrapIndex(r.packIndex+1, len(r.catalog))
			r.syncSelectionFromIndices()
		} else {
			levels := r.selectedPackLevels()
			r.levelIndex = wrapIndex(r.levelIndex+1, len(levels))
			r.syncSelectionFromIndices()
		}
	case tea.KeyEnter:
		if r.catalogFocus == 0 {
			r.catalogFocus = 1
			return r, nil
		}
		r.startSelectedLevel()
	}
	return r, nil
}

func (r *Root) handlePlayingKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if (msg.Code == tea.KeyInsert && msg.Mod&tea.ModShift != 0) ||
		((msg.Code == 'v' || msg.Code == 'V') && msg.Mod&tea.ModCtrl != 0 && msg.Mod&tea.ModShift != 0) {
		return r, func() tea.Msg { return tea.ReadClipboard() }
	}

	switch msg.Code {
	case tea.KeyF1:
		r.dispatchController(func(c Controller) { c.OnHints() })
		return r, nil
	case tea.KeyF2:
		r.dispatchController(func(c Controller) { c.OnGoal() })
		return r, nil
	case tea.KeyF4:
		r.dispatchController(func(c Controller) { c.OnJournal() })
		return r, nil
	case tea.KeyF5:
		r.dispatchController(func(c Controller) { c.OnCheck() })
		return r, nil
	case tea.KeyF6:
		r.resetOpen = true
		return r, r.animateIfNeeded()
	case tea.KeyF9:
		if r.term != nil {
			r.term.ToggleScrollback()
		}
		return r, nil
	case tea.KeyF10:
		r.dispatchController(func(c Controller) { c.OnMenu() })
		return r, nil
	case tea.KeyEsc:
		if r.goalOpen {
			r.dispatchController(func(c Controller) { c.OnGoal() })
			return r, nil
		}
		if r.term != nil && r.term.InScrollback() {
			r.term.ToggleScrollback()
			return r, nil
		}
	}

	if r.term != nil {
		if msg.Code == tea.KeyPgUp && msg.Mod&tea.ModShift != 0 {
			if !r.term.InScrollback() {
				r.term.ToggleScrollback()
			}
			r.term.Scroll(-10)
			return r, nil
		}
		if msg.Code == tea.KeyPgDown && msg.Mod&tea.ModShift != 0 {
			if !r.term.InScrollback() {
				r.term.ToggleScrollback()
			}
			r.term.Scroll(10)
			return r, nil
		}
		if r.term.InScrollback() {
			switch msg.Code {
			case tea.KeyUp:
				r.term.Scroll(-1)
			case tea.KeyDown:
				r.term.Scroll(1)
			case tea.KeyPgUp:
				r.term.Scroll(-10)
			case tea.KeyPgDown:
				r.term.Scroll(10)
			}
			return r, nil
		}
	}

	if data := term.EncodeKeyPressToBytes(msg); len(data) > 0 {
		r.dispatchController(func(c Controller) { c.OnTerminalInput(data) })
	}
	return r, nil
}

func (r *Root) renderMainMenu() string {
	w, h := r.cols, r.rows
	header := r.theme.Header.Width(max(1, w)).Render("CLI Dojo")

	items := r.mainMenuItems()
	menuLines := make([]string, len(items))
	for i, item := range items {
		prefix := "  "
		if i == r.mainMenuIndex {
			prefix = "> "
		}
		menuLines[i] = prefix + item.Label
	}
	left := r.drawPanel("Main Menu", menuLines, min(36, max(24, w/3)), max(8, h-2))
	rightText := r.mainMenuInfoText(items)
	right := r.drawPanel("Overview", strings.Split(strings.TrimSuffix(rightText, "\n"), "\n"), max(20, w-lipgloss.Width(left)), max(8, h-2))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	if r.setupMsg != "" {
		setup := r.drawPanel("Setup", strings.Split(strings.TrimSpace(r.setupMsg+"\n\n"+r.setupDetails), "\n"), min(100, w), 10)
		body = body + "\n" + setup
	}
	return header + "\n" + body
}

func (r *Root) renderLevelSelect() string {
	w, h := r.cols, r.rows
	header := r.theme.Header.Width(max(1, w)).Render("CLI Dojo - Level Select")

	packs := make([]string, len(r.catalog))
	for i, p := range r.catalog {
		prefix := "  "
		if r.catalogFocus == 0 && i == r.packIndex {
			prefix = "> "
		}
		packs[i] = fmt.Sprintf("%s%s (%d)", prefix, p.Name, len(p.Levels))
	}
	if len(packs) == 0 {
		packs = []string{"No packs loaded."}
	}
	left := r.drawPanel("Packs", packs, min(34, max(24, w/4)), max(8, h-2))

	levels := r.selectedPackLevels()
	levelLines := make([]string, len(levels))
	for i, lv := range levels {
		prefix := "  "
		if r.catalogFocus == 1 && i == r.levelIndex {
			prefix = "> "
		}
		levelLines[i] = fmt.Sprintf("%s%s [d:%d ~%dm]", prefix, lv.Title, lv.Difficulty, lv.EstimatedMinutes)
	}
	if len(levelLines) == 0 {
		levelLines = []string{"No levels in this pack."}
	}
	middleW := min(46, max(28, w/3))
	middle := r.drawPanel("Levels", levelLines, middleW, max(8, h-2))

	detail := r.levelDetailText()
	right := r.drawPanel("Details", strings.Split(strings.TrimSuffix(detail, "\n"), "\n"), max(22, w-lipgloss.Width(left)-lipgloss.Width(middle)), max(8, h-2))

	return header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, middle, right)
}

func (r *Root) renderPlaying() string {
	w, h := r.cols, r.rows
	mode := DetermineLayoutMode(w, h)
	if r.forceTooSmall {
		mode = LayoutTooSmall
	}
	r.layout = mode

	if mode == LayoutTooSmall {
		cols := w
		rows := h
		if r.forceTooSmall {
			cols = r.tooSmallCols
			rows = r.tooSmallRows
		}
		msg := []string{
			"Terminal too small",
			fmt.Sprintf("Current: %dx%d", cols, rows),
			"Minimum: 90x24",
			"Resize the terminal to continue.",
		}
		panel := r.drawPanel("Resize Required", msg, min(60, w), min(12, h))
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, panel)
	}

	header := r.headerText()
	status := r.statusText()
	bodyH := max(3, h-2)
	bodyY := 1

	var body string
	if mode == LayoutWide {
		hudW := r.state.HudWidth
		if hudW <= 0 {
			hudW = 42
		}
		hudW = min(max(30, hudW), max(30, w-20))
		termW := max(20, w-hudW)
		hudPanel := r.drawPanel("HUD", strings.Split(strings.TrimSuffix(r.hudText(), "\n"), "\n"), hudW, bodyH)
		termPanel := r.renderTerminalPanel(termW, bodyH, hudW, bodyY)
		body = lipgloss.JoinHorizontal(lipgloss.Top, hudPanel, termPanel)
	} else {
		body = r.renderTerminalPanel(w, bodyH, 0, bodyY)
	}

	base := header + "\n" + body + "\n" + status
	if mode == LayoutMedium {
		drawer := r.renderGoalDrawer(bodyH)
		if drawer != "" {
			base = composeOverlayAt(base, drawer, w, h, bodyY, 0)
		}
	}
	return base
}

func (r *Root) renderTerminalPanel(width, height, originX, originY int) string {
	innerW := max(1, width-2)
	innerH := max(1, height-2)
	lines := make([]string, innerH)
	if r.term != nil {
		snap := r.term.Snapshot(innerW, innerH)
		copy(lines, snap.Lines)
		if snap.Scrollback && len(lines) > 0 {
			indicator := "[SCROLLBACK] "
			base := []rune(lines[0])
			for i, ch := range []rune(indicator) {
				if i >= len(base) {
					break
				}
				base[i] = ch
			}
			lines[0] = string(base)
		}
		if snap.CursorShow && !snap.Scrollback {
			if snap.CursorY >= 0 && snap.CursorY < len(lines) {
				row := []rune(lines[snap.CursorY])
				if snap.CursorX >= 0 && snap.CursorX < len(row) {
					cursorRune := '▌'
					if r.ascii {
						cursorRune = '|'
					}
					row[snap.CursorX] = cursorRune
					lines[snap.CursorY] = string(row)
					x := originX + 1 + snap.CursorX
					y := originY + 1 + snap.CursorY
					if x >= 0 && x < r.cols && y >= 0 && y < r.rows {
						r.termCursorX = x
						r.termCursorY = y
						r.termCursorShow = true
					}
				}
			}
		}
	} else {
		lines[0] = "No terminal session"
	}
	for i := range lines {
		if lines[i] == "" {
			lines[i] = strings.Repeat(" ", innerW)
		} else {
			lines[i] = padRune(lines[i], innerW)
		}
	}
	return r.drawPanel("Terminal", lines, width, height)
}

func (r *Root) renderGoalDrawer(bodyHeight int) string {
	progress := r.overlayPos
	if r.goalOpen && progress < 0.2 {
		progress = 0.2
	}
	if !r.goalOpen && progress < 0.05 {
		return ""
	}
	hudW := r.state.HudWidth
	if hudW <= 0 {
		hudW = 38
	}
	hudW = min(max(32, hudW), max(32, r.cols-18))
	drawW := int(float64(hudW) * maxFloat(progress, 0))
	if drawW < 18 {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(r.hudText(), "\n"), "\n")
	lines = append(lines, "", "Esc closes drawer")
	return r.drawPanel("HUD Drawer", lines, drawW, bodyHeight)
}

func (r *Root) renderOverlay() string {
	spec, ok := r.overlaySpec(r.topOverlay())
	if !ok {
		return ""
	}
	return r.drawPanel(spec.title, spec.lines, spec.width, spec.height)
}

type overlaySpec struct {
	title    string
	lines    []string
	width    int
	height   int
	startRow int
	startCol int
}

func (r *Root) overlaySpec(top string) (overlaySpec, bool) {
	if top == "" {
		return overlaySpec{}, false
	}
	w := min(max(56, r.cols-12), r.cols)
	h := min(max(10, r.rows/2), max(8, r.rows-4))

	var title string
	var lines []string
	switch top {
	case "menu":
		title = "Menu"
		items := r.menuItems()
		for i, item := range items {
			prefix := "  "
			if i == r.menuIndex {
				prefix = "> "
			}
			lines = append(lines, prefix+item.Label)
		}
	case "hints":
		title = "Hints"
		lines = strings.Split(strings.TrimSuffix(r.hintsText(), "\n"), "\n")
		lines = append(lines, "", "Enter: Reveal hint", "y: Copy hints", "Esc: Close")
	case "journal":
		title = "Journal"
		lines = strings.Split(strings.TrimSuffix(r.journalText(), "\n"), "\n")
		lines = append(lines, "", "Enter: AI Explain", "y: Copy current  Y/Ctrl+C: Copy all", "Esc: Close")
	case "result":
		title = "Results"
		lines = strings.Split(strings.TrimSuffix(r.resultText(), "\n"), "\n")
		buttons := r.resultButtons()
		if len(buttons) > 0 {
			lines = append(lines, "", "Actions:")
			for i, b := range buttons {
				prefix := "  "
				if i == r.resultIndex {
					prefix = "> "
				}
				lines = append(lines, prefix+b)
			}
		}
		lines = append(lines, "", "Ctrl+C: Copy results")
	case "reset":
		title = "Confirm Reset"
		lines = []string{"Reset will destroy current /work state. Continue?", ""}
		labels := []string{"Cancel", "Reset"}
		for i, label := range labels {
			prefix := "  "
			if i == r.resetIndex {
				prefix = "> "
			}
			lines = append(lines, prefix+label)
		}
	case "reference":
		title = "Reference Solutions"
		lines = strings.Split(strings.TrimSuffix(r.referenceText, "\n"), "\n")
		lines = append(lines, "", "Ctrl+C: Copy text", "Esc/q: Close")
	case "diff":
		title = "Artifact Diff"
		lines = strings.Split(strings.TrimSuffix(r.diffText, "\n"), "\n")
		lines = append(lines, "", "Ctrl+C: Copy text", "Esc/q: Close")
	case "info":
		title = firstNonEmptyStr(r.infoTitle, "Info")
		lines = strings.Split(strings.TrimSuffix(r.infoText, "\n"), "\n")
		lines = append(lines, "", "Ctrl+C: Copy text", "Esc/q: Close")
	default:
		return overlaySpec{}, false
	}
	if len(lines) == 0 {
		lines = []string{"(empty)"}
	}
	needH := len(lines) + 2
	maxH := max(8, r.rows-4)
	if needH > h {
		h = min(needH, maxH)
	}
	return overlaySpec{
		title:    title,
		lines:    lines,
		width:    w,
		height:   h,
		startRow: (r.rows - h) / 2,
		startCol: (r.cols - w) / 2,
	}, true
}

func (r *Root) headerText() string {
	elapsed := r.state.ElapsedLabel
	if strings.TrimSpace(elapsed) == "" {
		d := time.Since(r.state.StartedAt).Truncate(time.Second)
		if r.state.StartedAt.IsZero() {
			d = 0
		}
		elapsed = d.String()
	}
	width := max(1, r.cols-1)
	engine := "Engine: " + firstNonEmptyStr(r.state.Engine, "unknown")
	mode := firstNonEmptyStr(r.state.ModeLabel, "Free Play")
	packLevel := strings.Trim(strings.TrimSpace(r.state.PackID)+"/"+strings.TrimSpace(r.state.LevelID), "/")
	parts := []string{"CLI Dojo", mode}
	if packLevel != "" {
		parts = append(parts, packLevel)
	}
	parts = append(parts, elapsed, engine)
	txt := strings.Join(parts, " | ")
	if len([]rune(txt)) > width {
		parts = []string{"CLI Dojo"}
		if packLevel != "" {
			parts = append(parts, packLevel)
		}
		parts = append(parts, elapsed, engine)
		txt = strings.Join(parts, " | ")
	}
	if len([]rune(txt)) > width && packLevel != "" {
		shortLevel := trimForWidth(packLevel, max(8, width/3))
		txt = strings.Join([]string{"CLI Dojo", shortLevel, elapsed, engine}, " | ")
	}
	txt = trimForWidth(txt, width)
	if r.debug {
		txt = fmt.Sprintf("%s | %dx%d %v", txt, r.cols, r.rows, r.layout)
		txt = trimForWidth(txt, width)
	}
	return r.theme.Header.Width(max(1, r.cols)).Render(txt)
}

func (r *Root) statusText() string {
	keys := r.help.View(r.keymap)
	if keys == "" {
		keys = "F1 Hints  F2 Goal  F4 Journal  F5 Check  F6 Reset  F9 Scrollback  F10 Menu"
	}
	if r.checking {
		keys += " | " + r.theme.Accent.Render(strings.TrimSpace(r.checkSpin.View())+" Checking...")
	}
	if r.statusFlash != "" {
		keys += " | " + r.statusFlash
	}
	keys = trimForWidth(keys, max(1, r.cols-1))
	return r.theme.Status.Width(max(1, r.cols)).Render(keys)
}

func (r *Root) hudText() string {
	var b strings.Builder
	b.WriteString("Objective\n")
	for _, obj := range r.state.Objective {
		b.WriteString("- " + obj + "\n")
	}
	if len(r.state.SessionGoals) > 0 {
		b.WriteString("\nSession Goals\n")
		for _, goal := range r.state.SessionGoals {
			b.WriteString("- " + goal + "\n")
		}
	}
	b.WriteString("\nChecks\n")
	for _, c := range r.state.Checks {
		icon := "o"
		if c.Status == "pass" {
			icon = "v"
		}
		if c.Status == "fail" {
			icon = "x"
		}
		if !r.ascii {
			if c.Status == "pass" {
				icon = "✓"
			} else if c.Status == "fail" {
				icon = "✗"
			} else {
				icon = "•"
			}
		}
		b.WriteString(fmt.Sprintf("%s %s\n", icon, c.Description))
	}
	b.WriteString("\nHints\n")
	for i, h := range r.state.Hints {
		text := h.Text
		status := "available"
		if h.Locked {
			text = "(hidden)"
			status = "locked"
			if h.LockReason != "" {
				status += " (" + h.LockReason + ")"
			}
		}
		if h.Revealed {
			status = "revealed"
		}
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, status, text))
	}
	b.WriteString("\nScore\n")
	b.WriteString(fmt.Sprintf("Current: %d\nHints: %d  Resets: %d\nStreak: %d\n", r.state.Score, r.state.HintsUsed, r.state.Resets, r.state.Streak))
	b.WriteString("\nMastery\n")
	b.WriteString(r.masteryBar(24) + "\n")
	if len(r.state.Badges) > 0 {
		b.WriteString("\nBadges\n")
		for _, badge := range r.state.Badges {
			b.WriteString("- " + badge + "\n")
		}
	}
	return b.String()
}

func (r *Root) hintsText() string {
	var b strings.Builder
	for i, h := range r.state.Hints {
		status := "available"
		text := h.Text
		if h.Locked {
			status = "locked"
			text = "(hidden)"
			if h.LockReason != "" {
				status += " (" + h.LockReason + ")"
			}
		}
		if h.Revealed {
			status = "revealed"
		}
		b.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, status, text))
	}
	if b.Len() == 0 {
		return "No hints configured."
	}
	return b.String()
}

func (r *Root) journalText() string {
	if len(r.journalEntries) == 0 {
		return "No commands logged yet."
	}
	start := r.journalIndex
	if start < 0 {
		start = 0
	}
	if start > len(r.journalEntries)-1 {
		start = len(r.journalEntries) - 1
	}
	var b strings.Builder
	for _, e := range r.journalEntries[start:] {
		tags := ""
		if len(e.Tags) > 0 {
			tags = " [" + strings.Join(e.Tags, ",") + "]"
		}
		b.WriteString(fmt.Sprintf("%s  %s%s\n", e.Timestamp, e.Command, tags))
	}
	return b.String()
}

func (r *Root) resultText() string {
	if !r.result.Visible {
		return ""
	}
	banner := "FAIL"
	if r.result.Passed {
		banner = "PASS"
	}
	var b strings.Builder
	b.WriteString(banner + "\n\n")
	b.WriteString(r.result.Summary + "\n\n")
	for _, c := range r.result.Checks {
		mark := "x"
		if c.Passed {
			mark = "v"
			if !r.ascii {
				mark = "✓"
			}
		} else if !r.ascii {
			mark = "✗"
		}
		b.WriteString(fmt.Sprintf("%s %s: %s\n", mark, c.ID, c.Message))
	}
	if len(r.result.Breakdown) > 0 {
		b.WriteString("\nScoring\n")
		for _, row := range r.result.Breakdown {
			b.WriteString(fmt.Sprintf("- %s: %s\n", row.Label, row.Value))
		}
	}
	b.WriteString(fmt.Sprintf("\nFinal Score: %d\n", r.result.Score))
	return b.String()
}

func (r *Root) mainMenuItems() []menuItem {
	return []menuItem{
		{Label: "Continue", Action: "continue"},
		{Label: "Daily Drill", Action: "daily"},
		{Label: "Level Select", Action: "select"},
		{Label: "Settings", Action: "settings"},
		{Label: "Stats", Action: "stats"},
		{Label: "Quit", Action: "quit"},
	}
}

func (r *Root) mainMenuInfoText(items []menuItem) string {
	idx := wrapIndex(r.mainMenuIndex, len(items))
	action := "Use Enter to select an option."
	if len(items) > 0 {
		switch items[idx].Action {
		case "continue":
			action = "Continue your most recent level run."
		case "daily":
			action = "Daily drill uses deterministic state in dev mode."
		case "select":
			action = "Browse packs and choose a level."
		case "settings":
			action = "Inspect runtime configuration."
		case "stats":
			action = "Review local progress summary."
		case "quit":
			action = "Exit CLI Dojo."
		}
	}
	var b strings.Builder
	b.WriteString("CLI Dojo\n\n")
	b.WriteString(fmt.Sprintf("Engine: %s\n", firstNonEmptyStr(r.mainMenu.EngineName, "unknown")))
	b.WriteString(fmt.Sprintf("Packs: %d  Levels: %d\n", r.mainMenu.PackCount, r.mainMenu.LevelCount))
	b.WriteString(fmt.Sprintf("Due Reviews: %d\n", r.mainMenu.DueReviews))
	if r.mainMenu.LastPackID != "" && r.mainMenu.LastLevelID != "" {
		b.WriteString(fmt.Sprintf("Last Played: %s / %s\n", r.mainMenu.LastPackID, r.mainMenu.LastLevelID))
	}
	b.WriteString(fmt.Sprintf("Runs: %d  Passes: %d  Attempts: %d  Resets: %d\n", r.mainMenu.LevelRuns, r.mainMenu.Passes, r.mainMenu.Attempts, r.mainMenu.Resets))
	b.WriteString(fmt.Sprintf("Streak: %d\n", r.mainMenu.Streak))
	if strings.TrimSpace(r.mainMenu.Tip) != "" {
		b.WriteString("\nTip:\n")
		b.WriteString(r.mainMenu.Tip)
		b.WriteString("\n")
	}
	b.WriteString("\nAction:\n" + action + "\n")
	return b.String()
}

func (r *Root) levelDetailText() string {
	pack := r.selectedPackSummary()
	if pack == nil || len(pack.Levels) == 0 {
		return "No levels available in this pack."
	}
	idx := wrapIndex(r.levelIndex, len(pack.Levels))
	lv := pack.Levels[idx]
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s\n", lv.Title))
	b.WriteString(fmt.Sprintf("ID: %s\nDifficulty: %d\nEstimated: %d min\n", lv.LevelID, lv.Difficulty, lv.EstimatedMinutes))
	if lv.Tier > 0 {
		b.WriteString(fmt.Sprintf("Tier: %d\n", lv.Tier))
	}
	if len(lv.ToolFocus) > 0 {
		b.WriteString("Tools: " + strings.Join(lv.ToolFocus, ", ") + "\n")
	}
	if len(lv.Concepts) > 0 {
		b.WriteString("Concepts: " + strings.Join(lv.Concepts, ", ") + "\n")
	}
	if strings.TrimSpace(lv.SummaryMD) != "" {
		summary := strings.TrimSpace(lv.SummaryMD)
		if r.markdown != nil {
			if rendered, err := r.markdown.Render(summary); err == nil {
				summary = strings.TrimSpace(rendered)
			}
		}
		b.WriteString("\n" + summary + "\n")
	}
	if len(lv.ObjectiveBullets) > 0 {
		b.WriteString("\nObjectives:\n")
		for _, obj := range lv.ObjectiveBullets {
			b.WriteString("- " + obj + "\n")
		}
	}
	b.WriteString("\nEnter: Start level    Esc: Back to main menu")
	return b.String()
}

type menuItem struct {
	Label  string
	Action string
}

func (r *Root) menuItems() []menuItem {
	return []menuItem{
		{Label: "Continue", Action: "continue"},
		{Label: "Restart level", Action: "restart"},
		{Label: "Level select", Action: "level_select"},
		{Label: "Main menu", Action: "main_menu"},
		{Label: "Settings", Action: "settings"},
		{Label: "Stats", Action: "stats"},
		{Label: "Quit", Action: "quit"},
	}
}

func (r *Root) activateMainMenuSelection() {
	items := r.mainMenuItems()
	if len(items) == 0 {
		return
	}
	item := items[wrapIndex(r.mainMenuIndex, len(items))]
	switch item.Action {
	case "continue", "daily":
		r.dispatchController(func(c Controller) { c.OnContinue() })
	case "select":
		r.dispatchController(func(c Controller) { c.OnOpenLevelSelect() })
	case "settings":
		r.dispatchController(func(c Controller) { c.OnOpenSettings() })
	case "stats":
		r.dispatchController(func(c Controller) { c.OnOpenStats() })
	case "quit":
		r.dispatchController(func(c Controller) { c.OnQuit() })
	}
}

func (r *Root) activateMenuItem(item menuItem) {
	r.menuOpen = false
	switch item.Action {
	case "continue":
		r.dispatchController(func(c Controller) { c.OnMenu() })
	case "restart":
		r.dispatchController(func(c Controller) { c.OnMenu() })
		r.resetOpen = true
	case "level_select":
		r.dispatchController(func(c Controller) { c.OnMenu() })
		r.dispatchController(func(c Controller) { c.OnOpenLevelSelect() })
	case "main_menu":
		r.dispatchController(func(c Controller) { c.OnMenu() })
		r.dispatchController(func(c Controller) { c.OnOpenMainMenu() })
	case "settings":
		r.dispatchController(func(c Controller) { c.OnMenu() })
		r.dispatchController(func(c Controller) { c.OnOpenSettings() })
	case "stats":
		r.dispatchController(func(c Controller) { c.OnMenu() })
		r.dispatchController(func(c Controller) { c.OnOpenStats() })
	case "quit":
		r.dispatchController(func(c Controller) { c.OnMenu() })
		r.dispatchController(func(c Controller) { c.OnQuit() })
	}
}

func (r *Root) startSelectedLevel() {
	pack := r.selectedPackSummary()
	if pack == nil || len(pack.Levels) == 0 {
		return
	}
	idx := wrapIndex(r.levelIndex, len(pack.Levels))
	lv := pack.Levels[idx]
	r.selectedLevel = lv.LevelID
	r.dispatchController(func(c Controller) { c.OnStartLevel(pack.PackID, lv.LevelID) })
}

func (r *Root) syncCatalogSelection() {
	if len(r.catalog) == 0 {
		r.packIndex = 0
		r.levelIndex = 0
		return
	}
	pidx := 0
	if r.selectedPack != "" {
		for i, p := range r.catalog {
			if p.PackID == r.selectedPack {
				pidx = i
				break
			}
		}
	}
	r.packIndex = pidx
	pack := r.catalog[pidx]
	if len(pack.Levels) == 0 {
		r.levelIndex = 0
		return
	}
	lidx := 0
	if r.selectedLevel != "" {
		for i, lv := range pack.Levels {
			if lv.LevelID == r.selectedLevel {
				lidx = i
				break
			}
		}
	}
	r.levelIndex = lidx
	r.selectedPack = pack.PackID
	r.selectedLevel = pack.Levels[lidx].LevelID
}

func (r *Root) syncSelectionFromIndices() {
	if len(r.catalog) == 0 {
		return
	}
	r.packIndex = wrapIndex(r.packIndex, len(r.catalog))
	pack := r.catalog[r.packIndex]
	r.selectedPack = pack.PackID
	if len(pack.Levels) == 0 {
		r.levelIndex = 0
		r.selectedLevel = ""
		return
	}
	r.levelIndex = wrapIndex(r.levelIndex, len(pack.Levels))
	r.selectedLevel = pack.Levels[r.levelIndex].LevelID
}

func (r *Root) selectedPackSummary() *PackSummary {
	if len(r.catalog) == 0 {
		return nil
	}
	if r.packIndex < 0 || r.packIndex >= len(r.catalog) {
		r.packIndex = 0
	}
	return &r.catalog[r.packIndex]
}

func (r *Root) selectedPackLevels() []LevelSummary {
	pack := r.selectedPackSummary()
	if pack == nil {
		return nil
	}
	return pack.Levels
}

func (r *Root) topOverlay() string {
	switch {
	case r.diffOpen:
		return "diff"
	case r.referenceOpen:
		return "reference"
	case r.infoOpen:
		return "info"
	case r.resetOpen:
		return "reset"
	case r.result.Visible:
		return "result"
	case r.journalOpen:
		return "journal"
	case r.hintsOpen:
		return "hints"
	case r.menuOpen:
		return "menu"
	}
	return ""
}

func (r *Root) overlayActive() bool {
	return r.topOverlay() != ""
}

func (r *Root) closeTopOverlay() {
	switch r.topOverlay() {
	case "diff":
		r.diffOpen = false
		r.diffText = ""
	case "reference":
		r.referenceOpen = false
		r.referenceText = ""
	case "info":
		r.infoOpen = false
		r.infoText = ""
		r.infoTitle = ""
	case "reset":
		r.resetOpen = false
	case "result":
		r.result = ResultState{}
	case "journal":
		r.journalOpen = false
	case "hints":
		r.hintsOpen = false
	case "menu":
		r.menuOpen = false
	}
}

func (r *Root) resultButtons() []string {
	if !r.result.Visible {
		return nil
	}
	buttons := make([]string, 0, 4)
	if r.result.CanShowReference {
		buttons = append(buttons, "Show reference solutions")
	}
	if r.result.CanOpenDiff {
		buttons = append(buttons, "Open diff")
	}
	primary := r.result.PrimaryAction
	if primary == "" {
		if r.result.Passed {
			primary = "Continue"
		} else {
			primary = "Try again"
		}
	}
	buttons = append(buttons, primary, "Close")
	return buttons
}

func (r *Root) activateResultButton(label string) {
	primary := r.result.PrimaryAction
	if primary == "" {
		if r.result.Passed {
			primary = "Continue"
		} else {
			primary = "Try again"
		}
	}
	switch label {
	case "Show reference solutions":
		r.dispatchController(func(c Controller) { c.OnShowReferenceSolutions() })
	case "Open diff":
		r.dispatchController(func(c Controller) { c.OnOpenDiff() })
	case primary:
		passed := r.result.Passed
		r.result = ResultState{}
		if passed {
			r.dispatchController(func(c Controller) { c.OnNextLevel() })
		} else {
			r.dispatchController(func(c Controller) { c.OnTryAgain() })
		}
	default:
		r.result = ResultState{}
		r.dispatchController(func(c Controller) { c.OnTryAgain() })
	}
}

func (r *Root) overlayCopyText(full bool) string {
	switch r.topOverlay() {
	case "journal":
		if len(r.journalEntries) == 0 {
			return ""
		}
		if full {
			return strings.TrimSpace(r.journalText())
		}
		idx := r.journalIndex
		if idx < 0 {
			idx = 0
		}
		if idx >= len(r.journalEntries) {
			idx = len(r.journalEntries) - 1
		}
		e := r.journalEntries[idx]
		line := fmt.Sprintf("%s\t%s", e.Timestamp, e.Command)
		if len(e.Tags) > 0 {
			line += "\t[" + strings.Join(e.Tags, ",") + "]"
		}
		return line
	case "result":
		return strings.TrimSpace(r.resultText())
	case "reference":
		return strings.TrimSpace(r.referenceText)
	case "diff":
		return strings.TrimSpace(r.diffText)
	case "info":
		title := strings.TrimSpace(r.infoTitle)
		text := strings.TrimSpace(r.infoText)
		if title == "" {
			return text
		}
		if text == "" {
			return title
		}
		return title + "\n\n" + text
	case "hints":
		return strings.TrimSpace(r.hintsText())
	}
	return ""
}

func (r *Root) drawPanel(title string, lines []string, width, height int) string {
	width = max(4, width)
	height = max(3, height)
	innerW := width - 2
	innerH := height - 2

	h := "─"
	v := "│"
	tl := "┌"
	tr := "┐"
	bl := "└"
	br := "┘"
	if r.ascii {
		h = "-"
		v = "|"
		tl, tr, bl, br = "+", "+", "+", "+"
	}

	top := tl + strings.Repeat(h, innerW) + tr
	if title != "" && innerW > 2 {
		t := " " + title + " "
		runes := []rune(top)
		start := 1
		for i, ch := range []rune(t) {
			pos := start + i
			if pos >= len(runes)-1 {
				break
			}
			runes[pos] = ch
		}
		top = string(runes)
	}

	out := make([]string, 0, height)
	out = append(out, r.theme.PanelBorder.Render(top))
	for row := 0; row < innerH; row++ {
		line := ""
		if row < len(lines) {
			line = lines[row]
		}
		line = padRune(line, innerW)
		out = append(out, r.theme.PanelBorder.Render(v)+r.theme.PanelBody.Render(line)+r.theme.PanelBorder.Render(v))
	}
	out = append(out, r.theme.PanelBorder.Render(bl+strings.Repeat(h, innerW)+br))
	return strings.Join(out, "\n")
}

func (r *Root) animateIfNeeded() tea.Cmd {
	target := 0.0
	if r.goalOpen {
		target = 1.0
	}
	if r.shouldAnimate(target) {
		return animateTickCmd()
	}
	return nil
}

func (r *Root) masteryBar(width int) string {
	m := r.mastery
	m.SetWidth(max(8, width))
	return m.ViewAs(r.masteryPercent())
}

func (r *Root) masteryPercent() float64 {
	total := len(r.state.Checks)
	if total == 0 {
		return 0
	}
	passed := 0
	failed := 0
	for _, check := range r.state.Checks {
		switch strings.ToLower(strings.TrimSpace(check.Status)) {
		case "pass":
			passed++
		case "fail":
			failed++
		}
	}
	checkRatio := float64(passed) / float64(total)
	scoreRatio := float64(r.state.Score) / 1000.0
	if scoreRatio < 0 {
		scoreRatio = 0
	}
	if scoreRatio > 1 {
		scoreRatio = 1
	}
	penalty := float64(r.state.HintsUsed)*0.08 + float64(r.state.Resets)*0.12 + float64(failed)*0.05
	v := 0.75*checkRatio + 0.25*scoreRatio - penalty
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	if passed == total && failed == 0 {
		v = maxFloat(v, 0.98)
	}
	if r.result.Visible && r.result.Passed {
		v = 1
	}
	return v
}

func (r *Root) shouldAnimate(target float64) bool {
	if r.motionLevel == "off" {
		return false
	}
	if target > 0 {
		return r.overlayPos < 0.999 || abs(r.overlayVel) > 0.001
	}
	return r.overlayPos > 0.001 || abs(r.overlayVel) > 0.001
}

func clockTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return clockMsg(t) })
}

func animateTickCmd() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg { return animateMsg(t) })
}

func spinnerTickCmd(model spinner.Model) tea.Cmd {
	return func() tea.Msg {
		return model.Tick()
	}
}

func firstNonEmptyStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func wrapIndex(i, n int) int {
	if n <= 0 {
		return 0
	}
	if i < 0 {
		i = n - 1
	}
	if i >= n {
		i = 0
	}
	return i
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

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func padRune(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(strings.ReplaceAll(s, "\t", "    "))
	if len(r) > width {
		r = r[:width]
	}
	if len(r) < width {
		r = append(r, []rune(strings.Repeat(" ", width-len(r)))...)
	}
	return string(r)
}

func composeOverlay(base, overlay string, cols, rows int) string {
	if cols <= 0 || rows <= 0 {
		return base
	}
	base = ansi.Strip(base)
	overlay = ansi.Strip(overlay)
	baseLines := strings.Split(base, "\n")
	if len(baseLines) < rows {
		pad := make([]string, rows-len(baseLines))
		baseLines = append(baseLines, pad...)
	}
	for i := 0; i < rows; i++ {
		baseLines[i] = padRune(baseLines[i], cols)
	}

	overlayLines := strings.Split(strings.TrimRight(overlay, "\n"), "\n")
	if len(overlayLines) == 0 {
		return strings.Join(baseLines[:rows], "\n")
	}
	ow := 1
	for _, line := range overlayLines {
		lw := len([]rune(line))
		if lw > ow {
			ow = lw
		}
	}
	if ow > cols {
		ow = cols
	}
	oh := len(overlayLines)
	if oh > rows {
		oh = rows
	}
	startRow := (rows - oh) / 2
	startCol := (cols - ow) / 2
	if startCol < 0 {
		startCol = 0
	}

	for i := 0; i < oh; i++ {
		row := startRow + i
		if row < 0 || row >= rows {
			continue
		}
		dst := []rune(baseLines[row])
		src := []rune(overlayLines[i])
		if len(src) > ow {
			src = src[:ow]
		}
		for j := 0; j < ow && startCol+j < len(dst); j++ {
			dst[startCol+j] = ' '
		}
		for j := 0; j < len(src) && startCol+j < len(dst); j++ {
			dst[startCol+j] = src[j]
		}
		baseLines[row] = string(dst)
	}
	return strings.Join(baseLines[:rows], "\n")
}

func composeOverlayAt(base, overlay string, cols, rows, startRow, startCol int) string {
	if cols <= 0 || rows <= 0 {
		return base
	}
	base = ansi.Strip(base)
	overlay = ansi.Strip(overlay)
	baseLines := strings.Split(base, "\n")
	if len(baseLines) < rows {
		pad := make([]string, rows-len(baseLines))
		baseLines = append(baseLines, pad...)
	}
	for i := 0; i < rows; i++ {
		baseLines[i] = padRune(baseLines[i], cols)
	}

	overlayLines := strings.Split(strings.TrimRight(overlay, "\n"), "\n")
	if len(overlayLines) == 0 {
		return strings.Join(baseLines[:rows], "\n")
	}
	ow := 1
	for _, line := range overlayLines {
		lw := len([]rune(line))
		if lw > ow {
			ow = lw
		}
	}
	if ow > cols {
		ow = cols
	}
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	for i, line := range overlayLines {
		row := startRow + i
		if row < 0 || row >= rows {
			continue
		}
		dst := []rune(baseLines[row])
		src := []rune(line)
		if len(src) > ow {
			src = src[:ow]
		}
		for j := 0; j < ow && startCol+j < len(dst); j++ {
			dst[startCol+j] = ' '
		}
		for j := 0; j < len(src) && startCol+j < len(dst); j++ {
			dst[startCol+j] = src[j]
		}
		baseLines[row] = string(dst)
	}
	return strings.Join(baseLines[:rows], "\n")
}

func trimForWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(strings.ReplaceAll(ansi.Strip(s), "\n", " "))
	if len(r) <= width {
		return string(r)
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (r *Root) currentMouseMode() tea.MouseMode {
	switch r.mouseScope {
	case "off":
		return tea.MouseModeNone
	case "full":
		return tea.MouseModeCellMotion
	default:
		if r.screen == ScreenPlaying && !r.overlayActive() && !r.goalOpen {
			return tea.MouseModeNone
		}
		return tea.MouseModeCellMotion
	}
}

func normalizeStyleVariant(v string) string {
	switch strings.TrimSpace(v) {
	case "cozy_clean", "retro_terminal", "modern_arcade":
		return strings.TrimSpace(v)
	default:
		return "modern_arcade"
	}
}

func normalizeMotionLevel(v string) string {
	switch strings.TrimSpace(v) {
	case "off", "reduced", "full":
		return strings.TrimSpace(v)
	default:
		return "full"
	}
}

func normalizeMouseScope(v string) string {
	switch strings.TrimSpace(v) {
	case "off", "scoped", "full":
		return strings.TrimSpace(v)
	default:
		return "scoped"
	}
}

func (r *Root) recordInputEvent(event string) {
	r.lastInputEvent = trimForWidth(strings.TrimSpace(event), 160)
}

func (r *Root) onModelPanic(where string, recovered any, msg tea.Msg) {
	if r.statusFlash == "" {
		r.statusFlash = "Recovered UI panic"
	}

	message := fmt.Sprintf("%v", recovered)
	msgType := ""
	if msg != nil {
		msgType = fmt.Sprintf("%T", msg)
	}
	r.logger.Error("ui.panic_recovered", map[string]any{
		"where":       where,
		"panic":       message,
		"messageType": msgType,
		"screen":      r.screen,
		"layout":      r.layout,
		"cols":        r.cols,
		"rows":        r.rows,
		"overlay":     r.topOverlay(),
		"last_input":  r.lastInputEvent,
		"stack":       string(debug.Stack()),
	})
}

var _ tea.Model = (*Root)(nil)
var _ View = (*Root)(nil)

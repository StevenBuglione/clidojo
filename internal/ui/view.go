package ui

import (
	"fmt"
	"hash/fnv"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"clidojo/internal/term"

	"github.com/charmbracelet/bubbles/v2/help"
	"github.com/charmbracelet/bubbles/v2/key"
	"github.com/charmbracelet/bubbles/v2/list"
	"github.com/charmbracelet/bubbles/v2/progress"
	"github.com/charmbracelet/bubbles/v2/spinner"
	"github.com/charmbracelet/bubbles/v2/viewport"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss/v2"
	clog "github.com/charmbracelet/log"
	"github.com/charmbracelet/x/ansi"
)

type applyMsg struct {
	fn func(*Root)
}

type drawMsg struct{}
type clockMsg time.Time
type animateMsg time.Time
type spinnerStartMsg struct{}
type escFlushMsg struct {
	seq uint64
}
type csiFlushMsg struct {
	seq uint64
}

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

type uiListItem struct {
	title       string
	description string
	filterValue string
}

func (i uiListItem) Title() string       { return i.title }
func (i uiListItem) Description() string { return i.description }
func (i uiListItem) FilterValue() string {
	if strings.TrimSpace(i.filterValue) != "" {
		return i.filterValue
	}
	return strings.TrimSpace(i.title + " " + i.description)
}

type Root struct {
	theme        Theme
	ascii        bool
	debug        bool
	devShortcuts bool
	term         term.Pane
	ctrl         Controller
	ctrlQueue    chan func()
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
	settingsOpen  bool
	briefingOpen  bool
	infoOpen      bool
	referenceOpen bool
	diffOpen      bool

	mainMenuIndex int
	packIndex     int
	levelIndex    int
	catalogFocus  int
	levelSearch   string
	levelDiffBand int
	menuIndex     int
	resetIndex    int
	resultIndex   int
	journalIndex  int
	settingsIndex int

	settings SettingsState

	mainList  list.Model
	packList  list.Model
	levelList list.Model
	detailVP  viewport.Model
	detailMD  string

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

	perfWindowStart time.Time
	perfFrameCount  int
	perfFPS         int
	perfLastRender  time.Duration
	perfLastSample  time.Time
	perfLastBytes   int64
	perfBytesPerSec int64

	lastInputEvent string
	pendingEsc     bool
	pendingEscSeq  uint64
	escFragment    bool
	pendingCSI     byte
	pendingCSISeq  uint64
}

type Options struct {
	ASCIIOnly    bool
	Debug        bool
	DevMode      bool
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
	spring := springForMotion(motionLevel)
	mastery := progress.New(
		progress.WithWidth(20),
		progress.WithScaledGradient("#5EC2FF", "#79E6A6"),
	)
	if motionLevel == "off" {
		mastery.SetSpringOptions(1000.0, 1.0)
	}
	checkSpin := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(theme.Accent),
	)
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(1)
	delegate.SetSpacing(0)
	delegate.ShowDescription = false
	newList := func() list.Model {
		m := list.New([]list.Item{}, delegate, 0, 0)
		m.SetShowTitle(false)
		m.SetShowHelp(false)
		m.SetShowPagination(false)
		m.SetShowStatusBar(false)
		m.SetShowFilter(false)
		m.SetFilteringEnabled(false)
		m.DisableQuitKeybindings()
		return m
	}

	r := &Root{
		theme:        theme,
		ascii:        opts.ASCIIOnly,
		debug:        opts.Debug,
		devShortcuts: opts.DevMode,
		term:         opts.TermPane,
		ctrlQueue:    make(chan func(), 128),
		styleVariant: styleVariant,
		motionLevel:  motionLevel,
		mouseScope:   mouseScope,
		screen:       ScreenMainMenu,
		layout:       LayoutWide,
		cols:         120,
		rows:         30,
		help:         h,
		mainList:     newList(),
		packList:     newList(),
		levelList:    newList(),
		detailVP:     viewport.New(viewport.WithWidth(1), viewport.WithHeight(1)),
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
		settings: SettingsState{
			AutoCheckMode:       "off",
			AutoCheckDebounceMS: 800,
			StyleVariant:        styleVariant,
			MotionLevel:         motionLevel,
			MouseScope:          mouseScope,
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
	r.refreshMainMenuList()
	r.refreshLevelSelectLists()
	go r.controllerLoop()
	return r
}

func (r *Root) controllerLoop() {
	for task := range r.ctrlQueue {
		if task != nil {
			task()
		}
	}
}

func (r *Root) Init() tea.Cmd {
	return tea.Batch(clockTickCmd(), animateTickCmd())
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
		r.samplePerfMetrics()
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
	case spinnerStartMsg:
		if !r.checking {
			return r, nil
		}
		return r, spinnerTickCmd(r.checkSpin)
	case spinner.TickMsg:
		var cmd tea.Cmd
		r.checkSpin, cmd = r.checkSpin.Update(msg)
		if !r.checking {
			return r, nil
		}
		return r, tea.Batch(cmd, spinnerTickCmd(r.checkSpin))
	case tea.PasteMsg:
		return r.handlePaste(msg)
	case tea.ClipboardMsg:
		return r.handlePaste(tea.PasteMsg(msg))
	case tea.MouseClickMsg:
		return r.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return r.handleMouseWheel(msg)
	case tea.KeyPressMsg:
		normalized, escFragment := normalizeKeyPressMsgWithMeta(msg)
		r.escFragment = escFragment
		model, cmd := r.handleKey(normalized)
		r.escFragment = false
		return model, cmd
	case escFlushMsg:
		if msg.seq != r.pendingEscSeq || !r.pendingEsc {
			return r, nil
		}
		if r.screen != ScreenPlaying || r.overlayActive() {
			r.pendingEsc = false
			return r, nil
		}
		r.pendingEsc = false
		r.sendTerminalInput([]byte{0x1b})
		return r, nil
	case csiFlushMsg:
		if msg.seq != r.pendingCSISeq || r.pendingCSI == 0 {
			return r, nil
		}
		prefix := r.pendingCSI
		r.pendingCSI = 0
		if r.screen != ScreenPlaying || r.overlayActive() {
			return r, nil
		}
		r.sendTerminalInput([]byte{0x1b, prefix})
		return r, nil
	}
	return r, nil
}

func (r *Root) View() (view string) {
	start := time.Now()
	defer func() {
		r.recordRenderFrame(time.Since(start))
	}()

	defer func() {
		if rec := recover(); rec != nil {
			r.onModelPanic("view", rec, nil)
			width := max(1, r.cols)
			msg := "UI recovered from a rendering panic. Check logs."
			if r.statusFlash == "" {
				r.statusFlash = "Recovered UI panic"
			}
			view = r.theme.Fail.Width(width).Render(trimForWidth(msg, max(1, width-1)))
		}
	}()

	if r.cols < 1 {
		r.cols = 120
	}
	if r.rows < 1 {
		r.rows = 30
	}

	var base string
	switch r.screen {
	case ScreenMainMenu:
		base = r.renderMainMenu()
	case ScreenLevelSelect:
		base = r.renderLevelSelect()
	default:
		base = r.renderPlaying()
	}

	overlay := r.renderOverlay()
	if overlay != "" {
		base = dimScreen(base, r.cols, r.rows)
	}
	if r.confettiActive() {
		base = r.applyConfetti(base)
	}
	if overlay != "" {
		base = composeOverlay(base, overlay, r.cols, r.rows)
	}
	base = normalizeScreen(base, r.rows, r.cols)
	return base
}

func (r *Root) Run() error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return nil
	}
	opts := []tea.ProgramOption{
		tea.WithColorProfile(colorprofile.ANSI256),
		tea.WithAltScreen(),
	}
	if r.mouseScope != "off" {
		opts = append(opts, tea.WithMouseCellMotion())
	}

	p := tea.NewProgram(r, opts...)
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
		if screen != ScreenLevelSelect {
			m.briefingOpen = false
		}
		if screen == ScreenPlaying {
			if m.state.StartedAt.IsZero() {
				m.state.StartedAt = time.Now()
			}
			cols, rows := m.cols, m.rows
			if cols > 0 && rows > 0 {
				m.dispatchController(func(c Controller) { c.OnResize(cols, rows) })
			}
		}
	})
}

func (r *Root) SetMainMenuState(state MainMenuState) {
	r.apply(func(m *Root) {
		m.mainMenu = state
		m.refreshMainMenuList()
	})
}

func (r *Root) SetCatalog(packs []PackSummary) {
	r.apply(func(m *Root) {
		m.catalog = append([]PackSummary(nil), packs...)
		m.syncCatalogSelection()
		m.refreshLevelSelectLists()
	})
}

func (r *Root) SetLevelSelection(packID, levelID string) {
	r.apply(func(m *Root) {
		m.selectedPack = packID
		m.selectedLevel = levelID
		m.syncCatalogSelection()
		m.refreshLevelSelectLists()
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

func (r *Root) SetSettings(state SettingsState, open bool) {
	r.apply(func(m *Root) {
		state.AutoCheckMode = normalizeAutoCheckMode(state.AutoCheckMode)
		if state.AutoCheckDebounceMS <= 0 {
			state.AutoCheckDebounceMS = 800
		}
		state.StyleVariant = normalizeStyleVariant(state.StyleVariant)
		state.MotionLevel = normalizeMotionLevel(state.MotionLevel)
		state.MouseScope = normalizeMouseScope(state.MouseScope)

		m.settings = state
		m.settingsOpen = open
		if !open {
			m.settingsIndex = 0
			return
		}
		m.infoOpen = false
		m.referenceOpen = false
		m.diffOpen = false
		m.settingsIndex = wrapIndex(m.settingsIndex, len(m.settingsMenuItems()))
	})
}

func (r *Root) SetChecking(checking bool) {
	r.apply(func(m *Root) {
		m.checking = checking
	})
	if checking {
		r.scheduleSpinner()
	}
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
	task := func() { fn(ctrl) }
	if r.ctrlQueue == nil {
		go task()
		return
	}
	select {
	case r.ctrlQueue <- task:
	default:
		// Keep input/render loop responsive even when controller queue is saturated.
		go task()
	}
}

func (r *Root) sendTerminalInput(data []byte) {
	if len(data) == 0 {
		return
	}
	if r.term != nil {
		_ = r.term.SendInput(data)
		return
	}
	r.dispatchController(func(c Controller) { c.OnTerminalInput(data) })
}

func (r *Root) scheduleSpinner() {
	r.mu.Lock()
	p := r.program
	running := r.running
	r.mu.Unlock()
	if !running || p == nil {
		return
	}
	p.Send(spinnerStartMsg{})
}

func (r *Root) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r.recordInputEvent(fmt.Sprintf("key:%v mod:%v text:%q", msg.Code, msg.Mod, msg.Text))

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
	contentText := string(msg)
	r.recordInputEvent(fmt.Sprintf("paste:%d", len(contentText)))

	if r.screen != ScreenPlaying || r.overlayActive() {
		return r, nil
	}
	if r.term != nil && r.term.InScrollback() {
		r.term.ToggleScrollback()
	}
	if contentText == "" {
		return r, nil
	}
	bracketed := false
	if r.term != nil {
		bracketed = r.term.BracketedPasteEnabled()
	}
	content := term.EncodePasteToBytes(contentText, bracketed)
	if len(content) == 0 {
		return r, nil
	}
	r.sendTerminalInput(content)
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
		level := levels[r.levelIndex]
		if level.Locked {
			reason := strings.TrimSpace(level.LockReason)
			if reason == "" {
				reason = "Level is locked."
			}
			r.statusFlash = reason
			return r, nil
		}
		r.briefingOpen = true
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
	case "settings":
		row := contentRow
		items := r.settingsMenuItems()
		if row >= 0 && row < len(items) {
			r.settingsIndex = row
			action := items[row].Action
			if action == "apply" {
				r.settingsOpen = false
				r.dispatchController(func(c Controller) { c.OnApplySettings(r.settings) })
			} else if action == "cancel" {
				r.settingsOpen = false
				r.settingsIndex = 0
			} else {
				r.stepSetting(action, true)
			}
		}
	case "briefing":
		r.briefingOpen = false
		r.startSelectedLevel()
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
	case "settings":
		items := r.settingsMenuItems()
		if len(items) == 0 {
			r.settingsOpen = false
			return r, nil
		}
		switch msg.Code {
		case tea.KeyUp:
			r.settingsIndex = wrapIndex(r.settingsIndex-1, len(items))
		case tea.KeyDown, tea.KeyTab:
			r.settingsIndex = wrapIndex(r.settingsIndex+1, len(items))
		case tea.KeyLeft:
			r.stepSetting(items[r.settingsIndex].Action, false)
		case tea.KeyRight:
			r.stepSetting(items[r.settingsIndex].Action, true)
		case tea.KeyEnter:
			action := items[r.settingsIndex].Action
			if action == "apply" {
				r.settingsOpen = false
				r.dispatchController(func(c Controller) { c.OnApplySettings(r.settings) })
			} else if action == "cancel" {
				r.settingsOpen = false
				r.settingsIndex = 0
			} else {
				r.stepSetting(action, true)
			}
		}
	case "info":
		if strings.EqualFold(strings.TrimSpace(r.infoTitle), "stats") &&
			(msg.Code == 'r' || msg.Code == 'R') &&
			msg.Mod&tea.ModCtrl == 0 && msg.Mod&tea.ModAlt == 0 {
			r.dispatchController(func(c Controller) { c.OnOpenStats() })
		}
	case "briefing":
		switch msg.Code {
		case tea.KeyEnter:
			r.briefingOpen = false
			r.startSelectedLevel()
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
		// In medium layout, opening hints also opens the HUD drawer.
		// Esc dismissal should close both to match expected UX.
		if r.layout == LayoutCompact && r.goalOpen {
			r.goalOpen = false
			r.dispatchController(func(c Controller) { c.OnGoal() })
		}
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
	if len(items) == 0 {
		return r, nil
	}
	r.refreshMainMenuList()
	switch msg.Code {
	case tea.KeyEsc:
		r.statusFlash = "Select Quit from the menu to exit."
		return r, nil
	case tea.KeyEnter:
		r.mainMenuIndex = wrapIndex(r.mainList.Index(), len(items))
		r.activateMainMenuSelection()
		return r, nil
	case tea.KeyTab:
		r.mainMenuIndex = wrapIndex(r.mainMenuIndex+1, len(items))
		r.mainList.Select(r.mainMenuIndex)
		return r, nil
	}
	var cmd tea.Cmd
	r.mainList, cmd = r.mainList.Update(msg)
	r.mainMenuIndex = wrapIndex(r.mainList.Index(), len(items))
	return r, cmd
}

func (r *Root) handleLevelSelectKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r.refreshLevelSelectLists()

	if msg.Mod&tea.ModCtrl != 0 {
		switch msg.Code {
		case 'u', 'U':
			r.levelSearch = ""
			r.syncSelectionFromIndices()
			r.refreshLevelSelectLists()
			return r, nil
		}
	}
	if msg.Mod&tea.ModAlt != 0 {
		switch msg.Code {
		case 'f', 'F':
			r.levelDiffBand = wrapIndex(r.levelDiffBand+1, 4)
			r.syncSelectionFromIndices()
			r.refreshLevelSelectLists()
			return r, nil
		}
	}
	if msg.Code == tea.KeyEsc {
		if r.levelSearch != "" {
			r.levelSearch = ""
			r.syncSelectionFromIndices()
			r.refreshLevelSelectLists()
			return r, nil
		}
		r.dispatchController(func(c Controller) { c.OnBackToMainMenu() })
		return r, nil
	}
	if msg.Code == tea.KeyBackspace {
		rs := []rune(r.levelSearch)
		if len(rs) > 0 {
			r.levelSearch = string(rs[:len(rs)-1])
			r.syncSelectionFromIndices()
			r.refreshLevelSelectLists()
		}
		return r, nil
	}
	if msg.Code == tea.KeyTab && msg.Mod&tea.ModShift != 0 {
		r.catalogFocus = 0
		return r, nil
	}
	if msg.Mod == 0 && msg.Text != "" && msg.Code >= 32 && msg.Code != tea.KeyEnter {
		r.levelSearch += msg.Text
		r.syncSelectionFromIndices()
		r.refreshLevelSelectLists()
		return r, nil
	}

	switch msg.Code {
	case tea.KeyLeft:
		r.catalogFocus = 0
	case tea.KeyRight, tea.KeyTab:
		r.catalogFocus = 1
	case tea.KeyUp:
		fallthrough
	case tea.KeyDown:
		fallthrough
	case tea.KeyHome:
		fallthrough
	case tea.KeyEnd:
		fallthrough
	case tea.KeyPgUp:
		fallthrough
	case tea.KeyPgDown:
		if r.catalogFocus == 0 {
			var cmd tea.Cmd
			r.packList, cmd = r.packList.Update(msg)
			r.packIndex = wrapIndex(r.packList.Index(), max(1, len(r.packList.Items())))
			r.syncSelectionFromIndices()
			r.refreshLevelSelectLists()
			return r, cmd
		} else {
			var cmd tea.Cmd
			r.levelList, cmd = r.levelList.Update(msg)
			r.levelIndex = wrapIndex(r.levelList.Index(), max(1, len(r.levelList.Items())))
			r.syncSelectionFromIndices()
			r.refreshLevelSelectLists()
			return r, cmd
		}
	case tea.KeyEnter:
		if r.catalogFocus == 0 {
			r.catalogFocus = 1
			return r, nil
		}
		levels := r.selectedPackLevels()
		if len(levels) == 0 {
			return r, nil
		}
		level := levels[wrapIndex(r.levelIndex, len(levels))]
		if level.Locked {
			reason := strings.TrimSpace(level.LockReason)
			if reason == "" {
				reason = "Level is locked."
			}
			r.statusFlash = reason
			return r, nil
		}
		r.briefingOpen = true
	}
	return r, nil
}

func (r *Root) handlePlayingKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if r.pendingEsc && msg.Code != tea.KeyEsc && msg.Code != tea.KeyEscape {
		if r.escFragment {
			// Browser/websocket paths can split CSI keys into ESC + fragment.
			// The fragment will be encoded with its own ESC prefix, so drop the
			// pending standalone ESC to avoid sending duplicate escapes.
			r.pendingEsc = false
		} else if msg.Text == "[" || msg.Text == "O" || msg.Code == '[' || msg.Code == 'O' {
			// Some browser/webterm paths split CSI into ESC + "[" + "B" events.
			// Buffer ESC+prefix briefly so the trailing byte can be coalesced.
			r.pendingEsc = false
			if msg.Text == "O" || msg.Code == 'O' {
				r.pendingCSI = 'O'
			} else {
				r.pendingCSI = '['
			}
			r.pendingCSISeq++
			return r, csiFlushCmd(r.pendingCSISeq)
		} else {
			r.pendingEsc = false
			r.sendTerminalInput([]byte{0x1b})
		}
	}
	if r.pendingCSI != 0 {
		prefix := r.pendingCSI
		r.pendingCSI = 0
		if msg.Text != "" {
			// If websocket fragmentation delivers only the CSI prefix again, keep
			// waiting briefly for the final byte.
			if msg.Text == string(prefix) {
				r.pendingCSI = prefix
				r.pendingCSISeq++
				return r, csiFlushCmd(r.pendingCSISeq)
			}
			// Some paths may deliver the entire CSI fragment as text (e.g. "[B").
			// In that case we should not prepend the buffered prefix again.
			if strings.HasPrefix(msg.Text, string(prefix)) || looksLikeEscFragmentText(msg.Text) {
				r.sendTerminalInput(append([]byte{0x1b}, []byte(msg.Text)...))
				return r, nil
			}
			payload := []byte{0x1b, prefix}
			payload = append(payload, []byte(msg.Text)...)
			r.sendTerminalInput(payload)
			return r, nil
		}

		// If Bubble Tea surfaced the trailing key as a key code (e.g. KeyDown),
		// use its terminal encoding directly to avoid emitting a bare ESC+[ prefix.
		if encoded := term.EncodeKeyPressToBytes(msg); len(encoded) > 0 {
			if encoded[0] == 0x1b {
				r.sendTerminalInput(encoded)
				return r, nil
			}
			payload := []byte{0x1b, prefix}
			payload = append(payload, encoded...)
			r.sendTerminalInput(payload)
			return r, nil
		}

		if msg.Code >= 32 && msg.Code <= 126 {
			r.sendTerminalInput([]byte{0x1b, prefix, byte(msg.Code)})
			return r, nil
		}

		r.sendTerminalInput([]byte{0x1b, prefix})
		return r, nil
	}

	if (msg.Code == tea.KeyInsert && msg.Mod&tea.ModShift != 0) ||
		((msg.Code == 'v' || msg.Code == 'V') && msg.Mod&tea.ModCtrl != 0 && msg.Mod&tea.ModShift != 0) {
		return r, func() tea.Msg { return tea.ReadClipboard() }
	}
	if msg.Mod&tea.ModCtrl != 0 {
		switch msg.Code {
		case 'v', 'V':
			return r, func() tea.Msg { return tea.ReadClipboard() }
		case 'h', 'H':
			if r.devShortcuts {
				r.dispatchController(func(c Controller) { c.OnHints() })
				return r, nil
			}
		case 'g', 'G':
			if r.devShortcuts {
				r.dispatchController(func(c Controller) { c.OnGoal() })
				return r, nil
			}
		case 'j', 'J':
			if r.devShortcuts {
				r.dispatchController(func(c Controller) { c.OnJournal() })
				return r, nil
			}
		case 'r', 'R':
			if r.devShortcuts {
				r.resetOpen = true
				return r, r.animateIfNeeded()
			}
		case 'm', 'M':
			if r.devShortcuts {
				r.dispatchController(func(c Controller) { c.OnMenu() })
				return r, nil
			}
		case 'y', 'Y':
			if r.devShortcuts && r.term != nil {
				r.term.ToggleScrollback()
				return r, nil
			}
		case tea.KeyEnter:
			r.dispatchController(func(c Controller) { c.OnCheck() })
			return r, nil
		}
	}
	if msg.Mod&tea.ModAlt != 0 {
		switch msg.Code {
		case 'h', 'H':
			r.dispatchController(func(c Controller) { c.OnHints() })
			return r, nil
		case 'g', 'G':
			r.dispatchController(func(c Controller) { c.OnGoal() })
			return r, nil
		case 'j', 'J':
			r.dispatchController(func(c Controller) { c.OnJournal() })
			return r, nil
		case 'r', 'R':
			r.resetOpen = true
			return r, r.animateIfNeeded()
		case 'm', 'M':
			r.dispatchController(func(c Controller) { c.OnMenu() })
			return r, nil
		case 'y', 'Y':
			if r.term != nil {
				r.term.ToggleScrollback()
			}
			return r, nil
		}
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
		r.pendingCSI = 0
		if r.goalOpen {
			r.dispatchController(func(c Controller) { c.OnGoal() })
			return r, r.animateIfNeeded()
		}
		if r.term != nil && r.term.InScrollback() {
			r.term.ToggleScrollback()
			return r, nil
		}
		if !r.running || r.program == nil {
			r.sendTerminalInput([]byte{0x1b})
			return r, nil
		}
		r.pendingEsc = true
		r.pendingEscSeq++
		seq := r.pendingEscSeq
		return r, escFlushCmd(seq)
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
		r.sendTerminalInput(data)
	}
	return r, nil
}

func (r *Root) renderMainMenu() string {
	w, h := r.cols, r.rows
	header := r.theme.Header.Width(max(1, w)).Render("CLI Dojo")
	r.refreshMainMenuList()

	items := r.mainMenuItems()
	leftW := min(36, max(24, w/3))
	bodyH := max(8, h-2)
	if len(items) > 0 {
		r.mainList.SetWidth(max(6, leftW-4))
		r.mainList.SetHeight(max(3, bodyH-4))
		r.mainList.Select(wrapIndex(r.mainMenuIndex, len(items)))
		r.mainMenuIndex = wrapIndex(r.mainList.Index(), len(items))
	}
	menuView := strings.TrimRight(r.mainList.View(), "\n")
	menuLines := []string{"(empty)"}
	if strings.TrimSpace(menuView) != "" {
		menuLines = strings.Split(menuView, "\n")
	}
	left := r.drawPanel("Main Menu", menuLines, leftW, bodyH)
	rightText := r.mainMenuInfoText(items)
	right := r.drawPanel("Overview", strings.Split(strings.TrimSuffix(rightText, "\n"), "\n"), max(20, w-lipgloss.Width(left)), bodyH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	if r.setupMsg != "" {
		setup := r.drawPanel("Setup", strings.Split(strings.TrimSpace(r.setupMsg+"\n\n"+r.setupDetails), "\n"), min(100, w), 10)
		body = body + "\n" + setup
	}
	return header + "\n" + body
}

func (r *Root) renderLevelSelect() string {
	w, h := r.cols, r.rows
	r.refreshLevelSelectLists()
	search := strings.TrimSpace(r.levelSearch)
	filter := r.levelDiffBandLabel()
	headerTxt := "CLI Dojo - Level Select"
	if search != "" || filter != "all" {
		headerTxt = fmt.Sprintf("%s | Search: %q | Filter: %s", headerTxt, search, filter)
	} else {
		headerTxt = fmt.Sprintf("%s | / type to search  Alt+F difficulty filter", headerTxt)
	}
	header := r.theme.Header.Width(max(1, w)).Render(trimForWidth(headerTxt, max(1, w-1)))

	leftW := min(34, max(24, w/4))
	bodyH := max(8, h-2)
	if len(r.packList.Items()) > 0 {
		r.packList.SetWidth(max(8, leftW-4))
		r.packList.SetHeight(max(3, bodyH-4))
		r.packList.Select(wrapIndex(r.packIndex, len(r.packList.Items())))
	}
	leftView := strings.TrimRight(r.packList.View(), "\n")
	leftLines := []string{"No packs loaded."}
	if strings.TrimSpace(leftView) != "" {
		leftLines = strings.Split(leftView, "\n")
	}
	left := r.drawPanel("Packs", leftLines, leftW, bodyH)

	middleW := min(46, max(28, w/3))
	if len(r.levelList.Items()) > 0 {
		r.levelList.SetWidth(max(8, middleW-4))
		r.levelList.SetHeight(max(3, bodyH-4))
		r.levelList.Select(wrapIndex(r.levelIndex, len(r.levelList.Items())))
	}
	middleView := strings.TrimRight(r.levelList.View(), "\n")
	levelLines := []string{"No levels match current search/filter."}
	if strings.TrimSpace(middleView) != "" {
		levelLines = strings.Split(middleView, "\n")
	}
	middle := r.drawPanel("Levels", levelLines, middleW, bodyH)

	rightW := max(22, w-lipgloss.Width(left)-lipgloss.Width(middle))
	r.updateDetailViewport(max(8, rightW-4), max(3, bodyH-4))
	detailView := strings.TrimRight(r.detailVP.View(), "\n")
	detailLines := []string{"No details available."}
	if strings.TrimSpace(detailView) != "" {
		detailLines = strings.Split(detailView, "\n")
	}
	right := r.drawPanel("Details", detailLines, rightW, bodyH)

	return header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, middle, right)
}

func (r *Root) renderPlaying() string {
	w, h := r.cols, r.rows
	mode := DetermineLayoutMode(w, h)
	// Recover from stale forced-too-small state when dimensions are now valid.
	// This can happen if a transient zero-size resize was observed while loading.
	if mode != LayoutTooSmall {
		r.forceTooSmall = false
	}
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
			"Minimum: 80x24",
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
		hudPanel := r.renderHUDColumn(hudW, bodyH)
		termPanel := r.renderTerminalPanel(termW, bodyH, hudW, bodyY)
		body = lipgloss.JoinHorizontal(lipgloss.Top, hudPanel, termPanel)
	} else {
		body = r.renderTerminalPanel(w, bodyH, 0, bodyY)
	}

	base := header + "\n" + body + "\n" + status
	if mode == LayoutCompact {
		drawer := r.renderGoalDrawer(bodyH)
		if drawer != "" {
			base = composeOverlayAt(base, drawer, w, h, bodyY, 0)
		}
	}
	return base
}

func (r *Root) renderTerminalPanel(width, height int, originX, originY int) string {
	_ = originX
	_ = originY
	innerW := max(1, width-2)
	innerH := max(1, height-2)
	lines := make([]string, innerH)
	if r.term != nil {
		if concrete, ok := r.term.(*term.TerminalPane); ok {
			frame := concrete.SnapshotFrame(innerW, innerH)
			copy(lines, renderTermFrameRows(frame, innerW, innerH, r.ascii))
			if frame.Scrollback && len(lines) > 0 {
				indicatorText := "[SCROLLBACK] "
				indicatorText = ansi.Truncate(indicatorText, innerW, "")
				indicatorWidth := ansi.StringWidth(indicatorText)
				base := lines[0]
				lines[0] = r.theme.Pending.Render(indicatorText) + ansi.Cut(base, indicatorWidth, innerW)
			}
		} else {
			snap := r.term.Snapshot(innerW, innerH)
			if len(snap.StyledLines) >= innerH {
				copy(lines, snap.StyledLines[:innerH])
			} else {
				copy(lines, snap.Lines)
			}
			if snap.Scrollback && len(lines) > 0 {
				indicatorText := "[SCROLLBACK] "
				indicatorText = ansi.Truncate(indicatorText, innerW, "")
				indicatorWidth := ansi.StringWidth(indicatorText)
				base := lines[0]
				if base == "" {
					if len(snap.StyledLines) > 0 {
						base = snap.StyledLines[0]
					} else if len(snap.Lines) > 0 {
						base = snap.Lines[0]
					}
				}
				lines[0] = r.theme.Pending.Render(indicatorText) + ansi.Cut(base, indicatorWidth, innerW)
			}
			if snap.CursorShow && !snap.Scrollback &&
				snap.CursorX >= 0 && snap.CursorX < innerW &&
				snap.CursorY >= 0 && snap.CursorY < innerH {
				lines[snap.CursorY] = overlayCursor(lines[snap.CursorY], snap.CursorX, innerW, r.ascii)
			}
		}
	} else {
		lines[0] = "No terminal session"
	}
	for i := range lines {
		if lines[i] == "" {
			lines[i] = strings.Repeat(" ", innerW)
		} else {
			lines[i] = padANSI(lines[i], innerW)
		}
	}
	return r.drawTerminalPanel("Terminal", lines, width, height)
}

func renderTermFrameRows(frame term.Frame, width, height int, ascii bool) []string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	rows := make([]string, height)
	var curStyle term.CellStyle
	styleActive := false
	for y := 0; y < height; y++ {
		var b strings.Builder
		styleActive = false
		for x := 0; x < width; x++ {
			cell := frame.Cell(x, y)
			style := cell.Style
			if frame.CursorShow && x == frame.CursorX && y == frame.CursorY {
				if ascii {
					cell.Ch = '_'
				} else {
					style = reverseCellStyle(style)
					// Reverse-video on a default-style blank cell can still be
					// visually invisible in some terminals. Force a concrete
					// fg/bg pair so the cursor remains obvious while typing.
					if cellStyleIsDefault(style) {
						style = term.CellStyle{
							FG:        0,
							BG:        7,
							FGDefault: false,
							BGDefault: false,
						}
					}
				}
			}
			if cell.Ch == 0 {
				cell.Ch = ' '
			}
			if cellStyleIsDefault(style) {
				if styleActive {
					b.WriteString("\x1b[0m")
					styleActive = false
				}
				b.WriteRune(cell.Ch)
				continue
			}
			if !styleActive || !cellStyleEqual(style, curStyle) {
				b.WriteString(cellStyleSGR(style))
				curStyle = style
				styleActive = true
			}
			b.WriteRune(cell.Ch)
		}
		if styleActive {
			b.WriteString("\x1b[0m")
		}
		rows[y] = b.String()
	}
	return rows
}

func cellStyleIsDefault(s term.CellStyle) bool {
	return s.FGDefault && s.BGDefault && !s.Bold && !s.Underline && !s.Dim
}

func reverseCellStyle(s term.CellStyle) term.CellStyle {
	s.FG, s.BG = s.BG, s.FG
	s.FGDefault, s.BGDefault = s.BGDefault, s.FGDefault
	return s
}

func cellStyleEqual(a, b term.CellStyle) bool {
	return a.FG == b.FG &&
		a.BG == b.BG &&
		a.FGDefault == b.FGDefault &&
		a.BGDefault == b.BGDefault &&
		a.Bold == b.Bold &&
		a.Underline == b.Underline &&
		a.Dim == b.Dim
}

func cellStyleSGR(s term.CellStyle) string {
	codes := []string{"0"}
	if s.Bold {
		codes = append(codes, "1")
	}
	if s.Dim {
		codes = append(codes, "2")
	}
	if s.Underline {
		codes = append(codes, "4")
	}
	codes = append(codes, colorIndexToSGR(s.FG, s.FGDefault, true))
	codes = append(codes, colorIndexToSGR(s.BG, s.BGDefault, false))
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

func colorIndexToSGR(index int, isDefault, fg bool) string {
	if isDefault {
		if fg {
			return "39"
		}
		return "49"
	}
	if index >= 0 && index < 8 {
		if fg {
			return fmt.Sprintf("%d", 30+index)
		}
		return fmt.Sprintf("%d", 40+index)
	}
	if index >= 8 && index < 16 {
		if fg {
			return fmt.Sprintf("%d", 90+(index-8))
		}
		return fmt.Sprintf("%d", 100+(index-8))
	}
	if fg {
		return fmt.Sprintf("38;5;%d", index)
	}
	return fmt.Sprintf("48;5;%d", index)
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

type confettiParticle struct {
	row   int
	col   int
	glyph string
}

func (r *Root) overlaySpec(top string) (overlaySpec, bool) {
	if top == "" {
		return overlaySpec{}, false
	}
	maxModalW := max(28, r.cols-6)
	maxModalH := max(8, r.rows-4)
	minW := 48
	maxWCap := min(maxModalW, 96)
	minH := 10
	contentWidth := func(lines []string) int {
		width := 0
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > width {
				width = w
			}
		}
		return width
	}

	var title string
	var lines []string
	switch top {
	case "menu":
		title = "Menu"
		minW = 28
		maxWCap = min(maxModalW, 44)
		minH = 8
		items := r.menuItems()
		for i, item := range items {
			if i == r.menuIndex {
				lines = append(lines, r.theme.Accent.Render("> "+item.Label))
				continue
			}
			lines = append(lines, "  "+item.Label)
		}
	case "hints":
		title = "Hints"
		minW = 56
		maxWCap = min(maxModalW, 90)
		minH = 12
		lines = strings.Split(strings.TrimSuffix(r.hintsText(), "\n"), "\n")
		lines = append(lines, "", "Enter: Reveal hint", "y: Copy hints", "Esc: Close")
	case "journal":
		title = "Journal"
		minW = 58
		maxWCap = min(maxModalW, 92)
		minH = 12
		lines = strings.Split(strings.TrimSuffix(r.journalText(), "\n"), "\n")
		lines = append(lines, "", "Enter: AI Explain", "y: Copy current  Y/Ctrl+C: Copy all", "Esc: Close")
	case "result":
		title = "Results"
		minW = 60
		maxWCap = min(maxModalW, 110)
		minH = 12
		lines = strings.Split(strings.TrimSuffix(r.resultText(), "\n"), "\n")
		buttons := r.resultButtons()
		if len(buttons) > 0 {
			lines = append(lines, "", "Actions:")
			for i, b := range buttons {
				if i == r.resultIndex {
					lines = append(lines, r.theme.Accent.Render("> "+b))
					continue
				}
				lines = append(lines, "  "+b)
			}
		}
		lines = append(lines, "", "Ctrl+C: Copy results")
	case "reset":
		title = "Confirm Reset"
		minW = 44
		maxWCap = min(maxModalW, 64)
		minH = 8
		lines = []string{"Reset will destroy current /work state. Continue?", ""}
		labels := []string{"Cancel", "Reset"}
		for i, label := range labels {
			if i == r.resetIndex {
				lines = append(lines, r.theme.Accent.Render("> "+label))
				continue
			}
			lines = append(lines, "  "+label)
		}
	case "settings":
		title = "Settings"
		minW = 56
		maxWCap = min(maxModalW, 84)
		minH = 12
		lines = r.renderSettingsLines()
	case "briefing":
		title = "Level Briefing"
		minW = 66
		maxWCap = min(maxModalW, 112)
		minH = 14
		lines = strings.Split(strings.TrimSuffix(r.briefingText(), "\n"), "\n")
		lines = append(lines, "", "Enter: Start level", "Esc: Back")
	case "reference":
		title = "Reference Solutions"
		minW = 64
		maxWCap = maxModalW
		minH = 12
		lines = strings.Split(strings.TrimSuffix(r.referenceText, "\n"), "\n")
		lines = append(lines, "", "Ctrl+C: Copy text", "Esc/q: Close")
	case "diff":
		title = "Artifact Diff"
		minW = 64
		maxWCap = maxModalW
		minH = 12
		lines = strings.Split(strings.TrimSuffix(r.diffText, "\n"), "\n")
		lines = append(lines, "", "Ctrl+C: Copy text", "Esc/q: Close")
	case "info":
		title = firstNonEmptyStr(r.infoTitle, "Info")
		minW = 50
		maxWCap = min(maxModalW, 100)
		minH = 10
		lines = strings.Split(strings.TrimSuffix(r.infoText, "\n"), "\n")
		lines = append(lines, "", "Ctrl+C: Copy text", "Esc/q: Close")
	default:
		return overlaySpec{}, false
	}
	if len(lines) == 0 {
		lines = []string{"(empty)"}
	}
	needW := contentWidth(lines) + 4
	w := min(max(needW, minW), maxWCap)
	needH := len(lines) + 2
	h := min(max(needH, minH), maxModalH)

	return overlaySpec{
		title:    title,
		lines:    lines,
		width:    w,
		height:   h,
		startRow: (r.rows - h) / 2,
		startCol: (r.cols - w) / 2,
	}, true
}

func (r *Root) confettiActive() bool {
	if !r.result.Visible || !r.result.Passed {
		return false
	}
	return normalizeMotionLevel(r.motionLevel) != "off"
}

func (r *Root) applyConfetti(base string) string {
	rows := max(1, r.rows)
	cols := max(1, r.cols)
	out := normalizeScreen(base, rows, cols)
	for _, p := range r.confettiParticles(cols, rows) {
		if p.row < 0 || p.row >= rows || p.col < 0 || p.col >= cols {
			continue
		}
		out = composeOverlayAt(out, p.glyph, cols, rows, p.row, p.col)
	}
	return out
}

func (r *Root) confettiParticles(cols, rows int) []confettiParticle {
	if cols < 8 || rows < 6 {
		return nil
	}

	count := 22
	if normalizeMotionLevel(r.motionLevel) == "reduced" {
		count = 12
	}

	glyphs := []string{"✦", "✶", "✷", "❖"}
	if r.ascii {
		glyphs = []string{"*", "+", "."}
	}
	colorize := []func(...string) string{
		r.theme.Pass.Render,
		r.theme.Accent.Render,
		r.theme.Pending.Render,
		r.theme.Info.Render,
	}

	seed := r.confettiSeed()
	next := func() uint64 {
		seed = seed*1664525 + 1013904223
		return seed
	}

	minRow := 1
	maxRowExclusive := rows - 1 // keep status row clear
	if maxRowExclusive <= minRow {
		return nil
	}

	spec, hasModal := r.overlaySpec(r.topOverlay())
	particles := make([]confettiParticle, 0, count)
	seen := make(map[int]struct{}, count)
	maxAttempts := count * 12
	for i := 0; i < maxAttempts && len(particles) < count; i++ {
		row := minRow + int(next()%uint64(max(1, maxRowExclusive-minRow)))
		col := int(next() % uint64(cols))
		if hasModal &&
			row >= spec.startRow-1 && row <= spec.startRow+spec.height &&
			col >= spec.startCol-1 && col <= spec.startCol+spec.width {
			continue
		}
		key := row*cols + col
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		glyph := glyphs[int(next()%uint64(len(glyphs)))]
		style := colorize[int(next()%uint64(len(colorize)))]
		particles = append(particles, confettiParticle{
			row:   row,
			col:   col,
			glyph: style(glyph),
		})
	}
	return particles
}

func (r *Root) confettiSeed() uint64 {
	h := fnv.New64a()
	payload := fmt.Sprintf(
		"%s|%s|%d|%s|%d",
		r.state.PackID,
		r.state.LevelID,
		r.result.Score,
		r.result.Summary,
		len(r.result.Checks),
	)
	_, _ = h.Write([]byte(payload))
	return h.Sum64()
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
		txt = fmt.Sprintf(
			"%s | %dx%d %v | %dfps %.1fms %dB/s",
			txt,
			r.cols,
			r.rows,
			r.layout,
			r.perfFPS,
			float64(r.perfLastRender.Microseconds())/1000.0,
			r.perfBytesPerSec,
		)
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

func (r *Root) renderHUDColumn(width, height int) string {
	width = max(4, width)
	height = max(3, height)

	type cardSpec struct {
		title   string
		lines   []string
		desired int
	}

	cards := []cardSpec{
		{title: "Objective", lines: r.objectiveCardLines(), desired: max(5, min(10, len(r.state.Objective)+3))},
		{title: "Checks", lines: r.checkCardLines(), desired: max(5, min(12, len(r.state.Checks)+3))},
		{title: "Hints", lines: r.hintCardLines(), desired: max(5, min(10, len(r.state.Hints)+3))},
		{title: "Score", lines: r.scoreCardLines(), desired: 6},
		{title: "Mastery", lines: r.masteryCardLines(), desired: 5},
	}
	if len(r.state.Badges) > 0 {
		cards = append(cards, cardSpec{
			title:   "Badges",
			lines:   r.badgesCardLines(),
			desired: max(4, min(8, len(r.state.Badges)+3)),
		})
	}

	remaining := height
	rendered := make([]string, 0, len(cards))
	for _, card := range cards {
		if remaining < 3 {
			break
		}
		cardH := min(card.desired, remaining)
		if cardH < 3 {
			break
		}
		rendered = append(rendered, r.drawPanel(card.title, card.lines, width, cardH))
		remaining -= cardH
	}
	if len(rendered) == 0 {
		return r.drawPanel("HUD", []string{"No HUD data"}, width, height)
	}
	out := strings.Join(rendered, "\n")
	lines := normalizeScreenLines(out, height, width)
	return strings.Join(lines, "\n")
}

func (r *Root) objectiveCardLines() []string {
	lines := make([]string, 0, len(r.state.Objective)+len(r.state.SessionGoals)+2)
	for _, obj := range r.state.Objective {
		lines = append(lines, "• "+obj)
	}
	if len(lines) == 0 {
		lines = append(lines, "No objective loaded.")
	}
	if len(r.state.SessionGoals) > 0 {
		lines = append(lines, "", "Session Goals")
		for _, goal := range r.state.SessionGoals {
			lines = append(lines, "◦ "+goal)
		}
	}
	return lines
}

func (r *Root) checkCardLines() []string {
	lines := make([]string, 0, len(r.state.Checks))
	for _, c := range r.state.Checks {
		icon := r.theme.Pending.Render("•")
		switch strings.ToLower(strings.TrimSpace(c.Status)) {
		case "pass":
			if r.ascii {
				icon = r.theme.Pass.Render("v")
			} else {
				icon = r.theme.Pass.Render("✓")
			}
		case "fail":
			if r.ascii {
				icon = r.theme.Fail.Render("x")
			} else {
				icon = r.theme.Fail.Render("✗")
			}
		}
		lines = append(lines, icon+" "+c.Description)
	}
	if len(lines) == 0 {
		lines = append(lines, "No checks loaded.")
	}
	return lines
}

func (r *Root) hintCardLines() []string {
	lines := make([]string, 0, len(r.state.Hints))
	for i, h := range r.state.Hints {
		status := r.theme.Info.Render("available")
		text := h.Text
		if h.Locked && !h.Revealed {
			status = r.theme.Muted.Render("locked")
			text = "(hidden)"
			if h.LockReason != "" {
				status = r.theme.Muted.Render("locked: " + h.LockReason)
			}
		} else if h.Revealed {
			status = r.theme.Pass.Render("revealed")
		}
		lines = append(lines, fmt.Sprintf("%d. %s %s", i+1, status, text))
	}
	if len(lines) == 0 {
		lines = append(lines, "No hints configured.")
	}
	return lines
}

func (r *Root) scoreCardLines() []string {
	return []string{
		fmt.Sprintf("Current: %d", r.state.Score),
		fmt.Sprintf("Hints: %d", r.state.HintsUsed),
		fmt.Sprintf("Resets: %d", r.state.Resets),
		fmt.Sprintf("Streak: %d", r.state.Streak),
	}
}

func (r *Root) masteryCardLines() []string {
	return []string{
		r.masteryBar(24),
		fmt.Sprintf("Progress: %d%%", int(r.masteryPercent()*100)),
	}
}

func (r *Root) badgesCardLines() []string {
	lines := make([]string, 0, len(r.state.Badges))
	for _, b := range r.state.Badges {
		lines = append(lines, "★ "+b)
	}
	return lines
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
		{Label: "Campaign", Action: "campaign"},
		{Label: "Practice", Action: "practice"},
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
			action = "Start today's deterministic daily challenge."
		case "campaign":
			action = "Play structured progression with prerequisites."
		case "practice":
			action = "Browse packs and choose any level."
		case "select":
			action = "Open level browser directly."
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
	if strings.TrimSpace(r.mainMenu.ModeLabel) != "" {
		b.WriteString(fmt.Sprintf("Mode: %s\n", r.mainMenu.ModeLabel))
	}
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

func (r *Root) mainMenuDescription(action string) string {
	switch action {
	case "continue":
		return "Resume your latest run"
	case "daily":
		return "Deterministic daily set"
	case "campaign":
		return "Progressive guided track"
	case "practice":
		return "Free play practice mode"
	case "select":
		return "Browse all levels"
	case "settings":
		return "Configure UI and checks"
	case "stats":
		return "Review performance"
	default:
		return "Exit CLI Dojo"
	}
}

func (r *Root) refreshMainMenuList() {
	items := r.mainMenuItems()
	listItems := make([]list.Item, 0, len(items))
	for _, item := range items {
		listItems = append(listItems, uiListItem{
			title:       item.Label,
			description: r.mainMenuDescription(item.Action),
			filterValue: strings.ToLower(item.Label + " " + item.Action),
		})
	}
	r.mainList.SetItems(listItems)
	if len(listItems) == 0 {
		r.mainMenuIndex = 0
		return
	}
	r.mainMenuIndex = wrapIndex(r.mainMenuIndex, len(listItems))
	r.mainList.Select(r.mainMenuIndex)
}

func (r *Root) refreshLevelSelectLists() {
	packItems := make([]list.Item, 0, len(r.catalog))
	for _, p := range r.catalog {
		packItems = append(packItems, uiListItem{
			title:       p.Name,
			description: fmt.Sprintf("%d levels", len(p.Levels)),
			filterValue: strings.ToLower(p.PackID + " " + p.Name),
		})
	}
	r.packList.SetItems(packItems)
	if len(packItems) > 0 {
		r.packIndex = wrapIndex(r.packIndex, len(packItems))
		r.packList.Select(r.packIndex)
	} else {
		r.packIndex = 0
	}

	levels := r.selectedPackLevels()
	levelItems := make([]list.Item, 0, len(levels))
	for _, lv := range levels {
		state := "new"
		if lv.Locked {
			state = "locked"
		} else if lv.PassedCount > 0 {
			state = "done"
		}
		title := fmt.Sprintf("%s [d:%d ~%dm]", lv.Title, lv.Difficulty, lv.EstimatedMinutes)
		levelItems = append(levelItems, uiListItem{
			title:       title,
			description: state,
			filterValue: strings.ToLower(lv.LevelID + " " + lv.Title + " " + strings.Join(lv.ToolFocus, " ")),
		})
	}
	r.levelList.SetItems(levelItems)
	if len(levelItems) > 0 {
		r.levelIndex = wrapIndex(r.levelIndex, len(levelItems))
		r.levelList.Select(r.levelIndex)
	} else {
		r.levelIndex = 0
	}
}

func (r *Root) updateDetailViewport(width, height int) {
	innerW := max(1, width)
	innerH := max(1, height)
	r.detailVP.SetWidth(innerW)
	r.detailVP.SetHeight(innerH)
	content := r.levelDetailText()
	if content != r.detailMD {
		r.detailMD = content
		r.detailVP.SetContent(content)
		r.detailVP.GotoTop()
	}
}

func (r *Root) settingsMenuItems() []menuItem {
	return []menuItem{
		{Label: "Auto-check mode", Action: "auto_check_mode"},
		{Label: "Auto-check debounce", Action: "auto_check_debounce"},
		{Label: "Style", Action: "style"},
		{Label: "Motion", Action: "motion"},
		{Label: "Mouse scope", Action: "mouse"},
		{Label: "Apply", Action: "apply"},
		{Label: "Cancel", Action: "cancel"},
	}
}

func (r *Root) renderSettingsLines() []string {
	items := r.settingsMenuItems()
	lines := make([]string, 0, len(items)+4)
	for i, item := range items {
		label := item.Label
		switch item.Action {
		case "auto_check_mode":
			label = fmt.Sprintf("%s: %s", label, normalizeAutoCheckMode(r.settings.AutoCheckMode))
		case "auto_check_debounce":
			label = fmt.Sprintf("%s: %dms", label, max(100, r.settings.AutoCheckDebounceMS))
		case "style":
			label = fmt.Sprintf("%s: %s", label, normalizeStyleVariant(r.settings.StyleVariant))
		case "motion":
			label = fmt.Sprintf("%s: %s", label, normalizeMotionLevel(r.settings.MotionLevel))
		case "mouse":
			label = fmt.Sprintf("%s: %s", label, normalizeMouseScope(r.settings.MouseScope))
		}
		if i == r.settingsIndex {
			lines = append(lines, r.theme.Accent.Render("> "+label))
			continue
		}
		lines = append(lines, "  "+label)
	}
	lines = append(lines, "", "Left/Right/Enter: change value  Up/Down: move", "Esc: close")
	return lines
}

func (r *Root) stepSetting(action string, forward bool) {
	switch action {
	case "auto_check_mode":
		opts := []string{"off", "manual", "command_debounce", "command_and_fs_debounce"}
		r.settings.AutoCheckMode = cycleString(opts, normalizeAutoCheckMode(r.settings.AutoCheckMode), forward)
	case "auto_check_debounce":
		opts := []int{300, 500, 800, 1200, 2000}
		current := r.settings.AutoCheckDebounceMS
		if current <= 0 {
			current = 800
		}
		r.settings.AutoCheckDebounceMS = cycleInt(opts, current, forward)
	case "style":
		opts := []string{"modern_arcade", "cozy_clean", "retro_terminal"}
		next := cycleString(opts, normalizeStyleVariant(r.settings.StyleVariant), forward)
		r.settings.StyleVariant = next
		r.theme = ThemeForVariant(next)
		r.styleVariant = next
	case "motion":
		opts := []string{"full", "reduced", "off"}
		next := cycleString(opts, normalizeMotionLevel(r.settings.MotionLevel), forward)
		r.settings.MotionLevel = next
		r.motionLevel = next
		r.spring = springForMotion(next)
		if next == "off" {
			r.overlayVel = 0
			if r.goalOpen {
				r.overlayPos = 1
			} else {
				r.overlayPos = 0
			}
		}
	case "mouse":
		opts := []string{"scoped", "full", "off"}
		next := cycleString(opts, normalizeMouseScope(r.settings.MouseScope), forward)
		r.settings.MouseScope = next
		r.mouseScope = next
	}
}

func (r *Root) levelDetailText() string {
	pack := r.selectedPackSummary()
	if pack == nil {
		return "No levels available in this pack."
	}
	levels := r.filteredLevels(pack.Levels)
	if len(levels) == 0 {
		return "No levels match current search/filter.\n\nType to search, Backspace to edit, Ctrl+U to clear.\nUse Alt+F to cycle difficulty filters."
	}
	idx := wrapIndex(r.levelIndex, len(levels))
	lv := levels[idx]
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s\n", lv.Title))
	b.WriteString(fmt.Sprintf("ID: %s\nDifficulty: %d\nEstimated: %d min\n", lv.LevelID, lv.Difficulty, lv.EstimatedMinutes))
	if lv.Tier > 0 {
		b.WriteString(fmt.Sprintf("Tier: %d\n", lv.Tier))
	}
	if lv.PassedCount > 0 {
		b.WriteString(fmt.Sprintf("Completed: %d run(s)", lv.PassedCount))
		if lv.BestScore > 0 {
			b.WriteString(fmt.Sprintf("  Best score: %d", lv.BestScore))
		}
		b.WriteString("\n")
	}
	if lv.Locked {
		lockReason := strings.TrimSpace(lv.LockReason)
		if lockReason == "" {
			lockReason = "This level is locked."
		}
		b.WriteString("Status: LOCKED\n")
		b.WriteString(lockReason + "\n")
	}
	if len(lv.Prerequisites) > 0 {
		b.WriteString("Prerequisites: " + strings.Join(lv.Prerequisites, ", ") + "\n")
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
	if lv.Locked {
		b.WriteString("\nEnter: Locked    Esc: Back to main menu")
	} else {
		b.WriteString("\nEnter: Start level    Esc: Back to main menu")
	}
	return b.String()
}

func (r *Root) briefingText() string {
	levels := r.selectedPackLevels()
	if len(levels) == 0 {
		return "No selectable level."
	}
	lv := levels[wrapIndex(r.levelIndex, len(levels))]
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s\n", lv.Title))
	b.WriteString(fmt.Sprintf("Level: %s\nDifficulty: %d\nEstimated: %d min\n", lv.LevelID, lv.Difficulty, lv.EstimatedMinutes))
	if len(lv.ToolFocus) > 0 {
		b.WriteString("Tools: " + strings.Join(lv.ToolFocus, ", ") + "\n")
	}
	if len(lv.Concepts) > 0 {
		b.WriteString("Concepts: " + strings.Join(lv.Concepts, ", ") + "\n")
	}
	if len(lv.ObjectiveBullets) > 0 {
		b.WriteString("\nObjectives:\n")
		for _, obj := range lv.ObjectiveBullets {
			b.WriteString("- " + obj + "\n")
		}
	}
	if strings.TrimSpace(lv.SummaryMD) != "" {
		b.WriteString("\nSummary:\n")
		b.WriteString(strings.TrimSpace(lv.SummaryMD))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
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
	case "continue":
		r.dispatchController(func(c Controller) { c.OnContinue() })
	case "daily":
		r.dispatchController(func(c Controller) { c.OnStartDailyDrill() })
	case "campaign":
		r.dispatchController(func(c Controller) { c.OnStartCampaign() })
	case "practice":
		r.dispatchController(func(c Controller) { c.OnStartPractice() })
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
	levels := r.selectedPackLevels()
	if pack == nil || len(levels) == 0 {
		return
	}
	idx := wrapIndex(r.levelIndex, len(levels))
	lv := levels[idx]
	if lv.Locked {
		reason := strings.TrimSpace(lv.LockReason)
		if reason == "" {
			reason = "Level is locked."
		}
		r.statusFlash = reason
		return
	}
	r.selectedLevel = lv.LevelID
	r.briefingOpen = false
	r.dispatchController(func(c Controller) { c.OnStartLevel(pack.PackID, lv.LevelID) })
}

func (r *Root) syncCatalogSelection() {
	if len(r.catalog) == 0 {
		r.packIndex = 0
		r.levelIndex = 0
		r.refreshLevelSelectLists()
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
	levels := r.filteredLevels(pack.Levels)
	if len(levels) == 0 {
		r.levelIndex = 0
		r.selectedLevel = ""
		return
	}
	lidx := 0
	if r.selectedLevel != "" {
		for i, lv := range levels {
			if lv.LevelID == r.selectedLevel {
				lidx = i
				break
			}
		}
	}
	r.levelIndex = lidx
	r.selectedPack = pack.PackID
	r.selectedLevel = levels[lidx].LevelID
	r.refreshLevelSelectLists()
}

func (r *Root) syncSelectionFromIndices() {
	if len(r.catalog) == 0 {
		r.refreshLevelSelectLists()
		return
	}
	r.packIndex = wrapIndex(r.packIndex, len(r.catalog))
	pack := r.catalog[r.packIndex]
	r.selectedPack = pack.PackID
	levels := r.filteredLevels(pack.Levels)
	if len(levels) == 0 {
		r.levelIndex = 0
		r.selectedLevel = ""
		r.refreshLevelSelectLists()
		return
	}
	if r.selectedLevel != "" {
		for i, lv := range levels {
			if lv.LevelID == r.selectedLevel {
				r.levelIndex = i
				r.selectedLevel = lv.LevelID
				r.refreshLevelSelectLists()
				return
			}
		}
	}
	r.levelIndex = wrapIndex(r.levelIndex, len(levels))
	r.selectedLevel = levels[r.levelIndex].LevelID
	r.refreshLevelSelectLists()
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
	return r.filteredLevels(pack.Levels)
}

func (r *Root) levelDiffBandLabel() string {
	switch r.levelDiffBand {
	case 1:
		return "easy(1-2)"
	case 2:
		return "mid(3)"
	case 3:
		return "hard(4-5)"
	default:
		return "all"
	}
}

func (r *Root) filteredLevels(levels []LevelSummary) []LevelSummary {
	if len(levels) == 0 {
		return nil
	}
	search := strings.ToLower(strings.TrimSpace(r.levelSearch))
	out := make([]LevelSummary, 0, len(levels))
	for _, lv := range levels {
		if !r.matchesDifficultyBand(lv.Difficulty) {
			continue
		}
		if search != "" && !r.levelMatchesSearch(lv, search) {
			continue
		}
		out = append(out, lv)
	}
	return out
}

func (r *Root) matchesDifficultyBand(diff int) bool {
	switch r.levelDiffBand {
	case 1:
		return diff <= 2
	case 2:
		return diff == 3
	case 3:
		return diff >= 4
	default:
		return true
	}
}

func (r *Root) levelMatchesSearch(lv LevelSummary, q string) bool {
	var b strings.Builder
	b.WriteString(strings.ToLower(lv.LevelID))
	b.WriteString("\n")
	b.WriteString(strings.ToLower(lv.Title))
	b.WriteString("\n")
	b.WriteString(strings.ToLower(lv.SummaryMD))
	for _, item := range lv.ToolFocus {
		b.WriteString("\n")
		b.WriteString(strings.ToLower(item))
	}
	for _, item := range lv.Concepts {
		b.WriteString("\n")
		b.WriteString(strings.ToLower(item))
	}
	for _, item := range lv.Prerequisites {
		b.WriteString("\n")
		b.WriteString(strings.ToLower(item))
	}
	b.WriteString("\n")
	b.WriteString(strings.ToLower(lv.LockReason))
	for _, item := range lv.ObjectiveBullets {
		b.WriteString("\n")
		b.WriteString(strings.ToLower(item))
	}
	return strings.Contains(b.String(), q)
}

func (r *Root) topOverlay() string {
	switch {
	case r.diffOpen:
		return "diff"
	case r.referenceOpen:
		return "reference"
	case r.briefingOpen:
		return "briefing"
	case r.settingsOpen:
		return "settings"
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
	case "briefing":
		r.briefingOpen = false
	case "settings":
		r.settingsOpen = false
		r.settingsIndex = 0
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
	case "briefing":
		return strings.TrimSpace(r.briefingText())
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

	out := make([]string, 0, height)
	if title != "" && innerW > 2 {
		label := " " + trimForWidth(title, innerW-2) + " "
		label = ansi.Truncate(label, innerW, "")
		labelW := ansi.StringWidth(label)
		fill := strings.Repeat(h, max(0, innerW-labelW))
		top := r.theme.PanelBorder.Render(tl) + r.theme.PanelTitle.Render(label) + r.theme.PanelBorder.Render(fill+tr)
		out = append(out, top)
	} else {
		top := tl + strings.Repeat(h, innerW) + tr
		out = append(out, r.theme.PanelBorder.Render(top))
	}
	for row := 0; row < innerH; row++ {
		line := ""
		if row < len(lines) {
			line = lines[row]
		}
		line = padANSI(strings.ReplaceAll(line, "\t", "    "), innerW)
		out = append(out, r.theme.PanelBorder.Render(v)+r.theme.PanelBody.Render(line)+r.theme.PanelBorder.Render(v))
	}
	out = append(out, r.theme.PanelBorder.Render(bl+strings.Repeat(h, innerW)+br))
	return strings.Join(out, "\n")
}

func (r *Root) drawTerminalPanel(title string, lines []string, width, height int) string {
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

	out := make([]string, 0, height)
	if title != "" && innerW > 2 {
		label := " " + trimForWidth(title, innerW-2) + " "
		label = ansi.Truncate(label, innerW, "")
		labelW := ansi.StringWidth(label)
		fill := strings.Repeat(h, max(0, innerW-labelW))
		top := r.theme.TerminalBorder.Render(tl) + r.theme.PanelTitle.Render(label) + r.theme.TerminalBorder.Render(fill+tr)
		out = append(out, top)
	} else {
		top := tl + strings.Repeat(h, innerW) + tr
		out = append(out, r.theme.TerminalBorder.Render(top))
	}
	for row := 0; row < innerH; row++ {
		line := strings.Repeat(" ", innerW)
		if row < len(lines) && lines[row] != "" {
			line = padANSI(lines[row], innerW)
		}
		out = append(out, r.theme.TerminalBorder.Render(v)+line+r.theme.TerminalBorder.Render(v))
	}
	out = append(out, r.theme.TerminalBorder.Render(bl+strings.Repeat(h, innerW)+br))
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

func escFlushCmd(seq uint64) tea.Cmd {
	// Keep this short to preserve normal Esc behavior in terminal apps while
	// still allowing websocket-fragmented CSI keys to coalesce.
	return tea.Tick(35*time.Millisecond, func(time.Time) tea.Msg {
		return escFlushMsg{seq: seq}
	})
}

func csiFlushCmd(seq uint64) tea.Cmd {
	// Some browser/websocket terminals deliver ESC + "[" + "B" as separate
	// events. Buffer briefly so we can coalesce the CSI prefix and final byte.
	return tea.Tick(35*time.Millisecond, func(time.Time) tea.Msg {
		return csiFlushMsg{seq: seq}
	})
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

func padANSI(s string, width int) string {
	if width <= 0 {
		return ""
	}
	visible := ansi.StringWidth(s)
	if visible > width {
		s = ansi.Truncate(s, width, "")
		visible = ansi.StringWidth(s)
	}
	if visible < width {
		s += strings.Repeat(" ", width-visible)
	}
	return s
}

func overlayCursor(line string, col, width int, ascii bool) string {
	if width <= 0 || col < 0 || col >= width {
		return padANSI(line, max(0, width))
	}
	line = padANSI(line, width)
	left := ansi.Cut(line, 0, col)
	cell := ansi.Cut(line, col, col+1)
	right := ansi.Cut(line, col+1, width)
	if cell == "" {
		cell = " "
	}
	if ascii {
		return left + "_" + right
	}
	return left + "\x1b[7m" + cell + "\x1b[0m" + right
}

func composeOverlay(base, overlay string, cols, rows int) string {
	if cols <= 0 || rows <= 0 {
		return base
	}
	overlayLines := strings.Split(strings.TrimRight(overlay, "\n"), "\n")
	if len(overlayLines) == 0 {
		return normalizeScreen(base, rows, cols)
	}

	ow := 1
	for _, line := range overlayLines {
		if w := ansi.StringWidth(line); w > ow {
			ow = w
		}
	}
	ow = min(ow, cols)
	oh := min(len(overlayLines), rows)
	startRow := max(0, (rows-oh)/2)
	startCol := max(0, (cols-ow)/2)

	return composeOverlayAt(normalizeScreen(base, rows, cols), strings.Join(overlayLines[:oh], "\n"), cols, rows, startRow, startCol)
}

func dimScreen(base string, cols, rows int) string {
	if cols <= 0 || rows <= 0 {
		return base
	}
	lines := normalizeScreenLines(base, rows, cols)
	for i := range lines {
		// Apply faint at line scope to avoid lossy ANSI/cell conversion artifacts
		// when composing overlays over already styled terminal content.
		lines[i] = "\x1b[2m" + lines[i] + "\x1b[22m"
	}
	return strings.Join(lines, "\n")
}

func composeOverlayAt(base, overlay string, cols, rows, startRow, startCol int) string {
	if cols <= 0 || rows <= 0 {
		return base
	}
	baseLines := normalizeScreenLines(base, rows, cols)
	overlayLines := strings.Split(strings.TrimRight(overlay, "\n"), "\n")
	if len(overlayLines) == 0 {
		return strings.Join(baseLines, "\n")
	}
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}
	if startRow >= rows || startCol >= cols {
		return strings.Join(baseLines, "\n")
	}

	ow := 1
	for _, line := range overlayLines {
		if w := ansi.StringWidth(line); w > ow {
			ow = w
		}
	}
	maxOW := max(1, cols-startCol)
	ow = min(ow, maxOW)

	maxOH := max(1, rows-startRow)
	prepared := make([]string, 0, min(len(overlayLines), maxOH))
	for _, line := range overlayLines {
		if len(prepared) >= maxOH {
			break
		}
		prepared = append(prepared, padANSI(ansi.Truncate(line, ow, ""), ow))
	}
	if len(prepared) == 0 {
		return strings.Join(baseLines, "\n")
	}
	for i, line := range prepared {
		row := startRow + i
		if row < 0 || row >= rows {
			continue
		}
		left := ansi.Cut(baseLines[row], 0, startCol)
		right := ansi.Cut(baseLines[row], startCol+ow, cols)
		baseLines[row] = left + line + right
	}
	return strings.Join(baseLines, "\n")
}

func normalizeScreen(screen string, rows, cols int) string {
	lines := normalizeScreenLines(screen, rows, cols)
	return strings.Join(lines[:rows], "\n")
}

func normalizeScreenLines(screen string, rows, cols int) []string {
	lines := strings.Split(screen, "\n")
	if len(lines) < rows {
		lines = append(lines, make([]string, rows-len(lines))...)
	}
	if len(lines) > rows {
		lines = lines[:rows]
	}
	for i := 0; i < rows; i++ {
		lines[i] = padANSI(lines[i], cols)
	}
	return lines
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

func normalizeAutoCheckMode(v string) string {
	switch strings.TrimSpace(v) {
	case "off", "manual", "command_debounce", "command_and_fs_debounce":
		return strings.TrimSpace(v)
	default:
		return "off"
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

func springForMotion(level string) harmonica.Spring {
	switch normalizeMotionLevel(level) {
	case "reduced":
		return harmonica.NewSpring(harmonica.FPS(30), 9.0, 0.92)
	case "off":
		return harmonica.NewSpring(harmonica.FPS(60), 1000.0, 1.0)
	default:
		return harmonica.NewSpring(harmonica.FPS(60), 10.0, 0.8)
	}
}

func cycleString(options []string, current string, forward bool) string {
	if len(options) == 0 {
		return current
	}
	idx := 0
	for i, opt := range options {
		if opt == current {
			idx = i
			break
		}
	}
	if forward {
		idx = (idx + 1) % len(options)
	} else {
		idx = (idx - 1 + len(options)) % len(options)
	}
	return options[idx]
}

func cycleInt(options []int, current int, forward bool) int {
	if len(options) == 0 {
		return current
	}
	idx := 0
	for i, opt := range options {
		if opt == current {
			idx = i
			break
		}
	}
	if forward {
		idx = (idx + 1) % len(options)
	} else {
		idx = (idx - 1 + len(options)) % len(options)
	}
	return options[idx]
}

func normalizeKeyPressMsg(msg tea.KeyPressMsg) tea.KeyPressMsg {
	normalized, _ := normalizeKeyPressMsgWithMeta(msg)
	return normalized
}

func normalizeKeyPressMsgWithMeta(msg tea.KeyPressMsg) (tea.KeyPressMsg, bool) {
	if msg.Text == "" {
		return msg, false
	}

	raw := msg.Text
	if strings.HasPrefix(raw, "\x1b") {
		fragment := strings.TrimPrefix(raw, "\x1b")
		if fragment == "" {
			msg.Code = tea.KeyEsc
			msg.Text = ""
			return msg, false
		}
		if normalized, ok := parseEscFragmentKey(fragment); ok {
			return normalized, true
		}
		if looksLikeEscFragmentText(fragment) {
			msg.Text = fragment
			return msg, true
		}
	}

	switch msg.Text {
	case "\r", "\n":
		msg.Code = tea.KeyEnter
		msg.Text = ""
		return msg, false
	case "\t":
		msg.Code = tea.KeyTab
		msg.Text = ""
		return msg, false
	case "\x1b":
		msg.Code = tea.KeyEsc
		msg.Text = ""
		return msg, false
	case "\x7f", "\b":
		msg.Code = tea.KeyBackspace
		msg.Text = ""
		return msg, false
	}

	if normalized, ok := parseEscFragmentKey(msg.Text); ok {
		return normalized, true
	}
	if looksLikeEscFragmentText(msg.Text) {
		return msg, true
	}

	return msg, false
}

func looksLikeEscFragmentText(s string) bool {
	if len(s) < 2 || len(s) > 16 {
		return false
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	if strings.HasPrefix(s, "[") {
		last := s[len(s)-1]
		if !((last >= 'A' && last <= 'Z') || last == '~') {
			return false
		}
		for i := 1; i < len(s)-1; i++ {
			ch := s[i]
			if (ch >= '0' && ch <= '9') || ch == ';' || ch == '?' {
				continue
			}
			return false
		}
		return true
	}
	if strings.HasPrefix(s, "O") && len(s) == 2 {
		switch s[1] {
		case 'P', 'Q', 'R', 'S', 'A', 'B', 'C', 'D', 'H', 'F', 'Z':
			return true
		}
	}
	return false
}

func parseEscFragmentKey(fragment string) (tea.KeyPressMsg, bool) {
	switch fragment {
	case "[A", "OA":
		return tea.KeyPressMsg{Code: tea.KeyUp}, true
	case "[B", "OB":
		return tea.KeyPressMsg{Code: tea.KeyDown}, true
	case "[C", "OC":
		return tea.KeyPressMsg{Code: tea.KeyRight}, true
	case "[D", "OD":
		return tea.KeyPressMsg{Code: tea.KeyLeft}, true
	case "[H", "OH":
		return tea.KeyPressMsg{Code: tea.KeyHome}, true
	case "[F", "OF":
		return tea.KeyPressMsg{Code: tea.KeyEnd}, true
	case "[Z":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, true
	case "OP":
		return tea.KeyPressMsg{Code: tea.KeyF1}, true
	case "OQ":
		return tea.KeyPressMsg{Code: tea.KeyF2}, true
	case "OR":
		return tea.KeyPressMsg{Code: tea.KeyF3}, true
	case "OS":
		return tea.KeyPressMsg{Code: tea.KeyF4}, true
	case "[2~":
		return tea.KeyPressMsg{Code: tea.KeyInsert}, true
	case "[3~":
		return tea.KeyPressMsg{Code: tea.KeyDelete}, true
	case "[5~":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}, true
	case "[6~":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}, true
	case "[15~":
		return tea.KeyPressMsg{Code: tea.KeyF5}, true
	case "[17~":
		return tea.KeyPressMsg{Code: tea.KeyF6}, true
	case "[18~":
		return tea.KeyPressMsg{Code: tea.KeyF7}, true
	case "[19~":
		return tea.KeyPressMsg{Code: tea.KeyF8}, true
	case "[20~":
		return tea.KeyPressMsg{Code: tea.KeyF9}, true
	case "[21~":
		return tea.KeyPressMsg{Code: tea.KeyF10}, true
	case "[23~":
		return tea.KeyPressMsg{Code: tea.KeyF11}, true
	case "[24~":
		return tea.KeyPressMsg{Code: tea.KeyF12}, true
	default:
		return tea.KeyPressMsg{}, false
	}
}

func (r *Root) recordRenderFrame(d time.Duration) {
	if d < 0 {
		d = 0
	}
	r.perfLastRender = d
	now := time.Now()
	if r.perfWindowStart.IsZero() {
		r.perfWindowStart = now
		r.perfFrameCount = 0
	}
	r.perfFrameCount++
	window := now.Sub(r.perfWindowStart)
	if window >= time.Second {
		r.perfFPS = int(float64(r.perfFrameCount) / window.Seconds())
		r.perfWindowStart = now
		r.perfFrameCount = 0
	}
}

func (r *Root) samplePerfMetrics() {
	provider, ok := r.term.(term.MetricsProvider)
	if !ok {
		return
	}
	now := time.Now()
	cur := provider.TotalOutputBytes()
	if r.perfLastSample.IsZero() {
		r.perfLastSample = now
		r.perfLastBytes = cur
		r.perfBytesPerSec = 0
		return
	}
	dt := now.Sub(r.perfLastSample).Seconds()
	if dt <= 0 {
		return
	}
	delta := cur - r.perfLastBytes
	if delta < 0 {
		delta = 0
	}
	r.perfBytesPerSec = int64(float64(delta) / dt)
	r.perfLastSample = now
	r.perfLastBytes = cur
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

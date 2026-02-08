package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"clidojo/internal/term"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/harmonica"
	clog "github.com/charmbracelet/log"
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
	theme Theme
	ascii bool
	debug bool
	term  term.Pane
	ctrl  Controller

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
	markdown   *glamour.TermRenderer
	logger     *clog.Logger
	overlayPos float64
	overlayVel float64
	spring     harmonica.Spring

	drawPending atomic.Bool
}

type Options struct {
	ASCIIOnly bool
	Debug     bool
	TermPane  term.Pane
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

	r := &Root{
		theme:   DefaultTheme(),
		ascii:   opts.ASCIIOnly,
		debug:   opts.Debug,
		term:    opts.TermPane,
		screen:  ScreenMainMenu,
		layout:  LayoutWide,
		cols:    120,
		rows:    30,
		help:    h,
		markdown: renderer,
		logger:  logger,
		spring:  harmonica.NewSpring(harmonica.FPS(60), 10.0, 0.8),
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
	return tea.Batch(clockTickCmd(), animateTickCmd())
}

func (r *Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if r.overlayActive() {
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
	case tea.KeyPressMsg:
		return r.handleKey(msg)
	}
	return r, nil
}

func (r *Root) View() tea.View {
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

	if overlay := r.renderOverlay(); overlay != "" {
		base = composeOverlay(base, overlay, r.cols, r.rows)
	}
	v := tea.NewView(base)
	v.AltScreen = true
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
			m.dispatchController(func(c Controller) { c.OnResize(m.cols, m.rows) })
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

func (r *Root) handleOverlayKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.Code == tea.KeyEsc || msg.Code == tea.KeyEscape {
		r.closeTopOverlay()
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
			r.goalOpen = false
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

	var body string
	if mode == LayoutWide {
		hudW := r.state.HudWidth
		if hudW <= 0 {
			hudW = 42
		}
		hudW = min(max(30, hudW), max(30, w-20))
		termW := max(20, w-hudW)
		hudPanel := r.drawPanel("HUD", strings.Split(strings.TrimSuffix(r.hudText(), "\n"), "\n"), hudW, bodyH)
		termPanel := r.renderTerminalPanel(termW, bodyH)
		body = lipgloss.JoinHorizontal(lipgloss.Top, hudPanel, termPanel)
	} else {
		if r.goalOpen {
			hudW := min(max(30, r.state.HudWidth), max(30, w-20))
			termW := max(20, w-hudW)
			hudPanel := r.drawPanel("HUD", strings.Split(strings.TrimSuffix(r.hudText(), "\n"), "\n"), hudW, bodyH)
			termPanel := r.renderTerminalPanel(termW, bodyH)
			body = lipgloss.JoinHorizontal(lipgloss.Top, hudPanel, termPanel)
		} else {
			body = r.renderTerminalPanel(w, bodyH)
		}
	}

	return header + "\n" + body + "\n" + status
}

func (r *Root) renderTerminalPanel(width, height int) string {
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

func (r *Root) renderOverlay() string {
	top := r.topOverlay()
	if top == "" {
		return ""
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
		lines = append(lines, "", "Enter: Reveal hint", "Esc: Close")
	case "journal":
		title = "Journal"
		lines = strings.Split(strings.TrimSuffix(r.journalText(), "\n"), "\n")
		lines = append(lines, "", "Enter: AI Explain", "Esc: Close")
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
	case "diff":
		title = "Artifact Diff"
		lines = strings.Split(strings.TrimSuffix(r.diffText, "\n"), "\n")
	case "info":
		title = firstNonEmptyStr(r.infoTitle, "Info")
		lines = strings.Split(strings.TrimSuffix(r.infoText, "\n"), "\n")
	}
	if len(lines) == 0 {
		lines = []string{"(empty)"}
	}
	needH := len(lines) + 2
	maxH := max(8, r.rows-4)
	if needH > h {
		h = min(needH, maxH)
	}
	return r.drawPanel(title, lines, w, h)
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
	txt := fmt.Sprintf("CLI Dojo | %s | %s/%s | %s | Engine: %s", firstNonEmptyStr(r.state.ModeLabel, "Free Play"), r.state.PackID, r.state.LevelID, elapsed, firstNonEmptyStr(r.state.Engine, "unknown"))
	if r.debug {
		txt = fmt.Sprintf("%s | %dx%d %v", txt, r.cols, r.rows, r.layout)
	}
	return r.theme.Header.Width(max(1, r.cols)).Render(txt)
}

func (r *Root) statusText() string {
	keys := r.help.View(r.keymap)
	if keys == "" {
		keys = "F1 Hints  F2 Goal  F4 Journal  F5 Check  F6 Reset  F9 Scrollback  F10 Menu"
	}
	if r.statusFlash != "" {
		keys += " | " + r.statusFlash
	}
	return r.theme.Status.Width(max(1, r.cols)).Render(keys)
}

func (r *Root) hudText() string {
	var b strings.Builder
	b.WriteString("Objective\n")
	for _, obj := range r.state.Objective {
		b.WriteString("- " + obj + "\n")
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
	var b strings.Builder
	for _, e := range r.journalEntries {
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
	if len(lv.ToolFocus) > 0 {
		b.WriteString("Tools: " + strings.Join(lv.ToolFocus, ", ") + "\n")
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
	switch item.Action {
	case "continue":
		r.menuOpen = false
	case "restart":
		r.menuOpen = false
		r.resetOpen = true
	case "level_select":
		r.menuOpen = false
		r.dispatchController(func(c Controller) { c.OnOpenLevelSelect() })
	case "main_menu":
		r.menuOpen = false
		r.dispatchController(func(c Controller) { c.OnOpenMainMenu() })
	case "settings":
		r.menuOpen = false
		r.dispatchController(func(c Controller) { c.OnOpenSettings() })
	case "stats":
		r.menuOpen = false
		r.dispatchController(func(c Controller) { c.OnOpenStats() })
	case "quit":
		r.menuOpen = false
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
	out = append(out, top)
	for row := 0; row < innerH; row++ {
		line := ""
		if row < len(lines) {
			line = lines[row]
		}
		line = padRune(line, innerW)
		out = append(out, v+line+v)
	}
	out = append(out, bl+strings.Repeat(h, innerW)+br)
	return strings.Join(out, "\n")
}

func (r *Root) animateIfNeeded() tea.Cmd {
	target := 0.0
	if r.overlayActive() {
		target = 1.0
	}
	if r.shouldAnimate(target) {
		return animateTickCmd()
	}
	return nil
}

func (r *Root) shouldAnimate(target float64) bool {
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
		for j := 0; j < len(src) && startCol+j < len(dst); j++ {
			dst[startCol+j] = src[j]
		}
		baseLines[row] = string(dst)
	}
	return strings.Join(baseLines[:rows], "\n")
}

var _ tea.Model = (*Root)(nil)
var _ View = (*Root)(nil)

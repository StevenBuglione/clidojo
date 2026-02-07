package ui

import (
	"fmt"
	"strings"
	"time"

	"clidojo/internal/term"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Root struct {
	app     *tview.Application
	theme   Theme
	ascii   bool
	debug   bool
	ctrl    Controller
	term    term.Pane
	layout  LayoutMode
	cols    int
	rows    int
	running bool

	state        PlayingState
	result       ResultState
	setupMsg     string
	setupDetails string
	statusFlash  string

	journalEntries []JournalEntry
	referenceText  string
	diffText       string
	infoTitle      string
	infoText       string

	header *tview.TextView
	status *tview.TextView
	hud    *tview.TextView
	body   *tview.Flex
	main   *tview.Flex
	pages  *tview.Pages

	drawer *tview.TextView

	menuModal      *tview.Modal
	hintsModal     *tview.Modal
	journalModal   *tview.Modal
	resultModal    *tview.Modal
	resetModal     *tview.Modal
	infoModal      *tview.Modal
	referenceModal *tview.Modal
	diffModal      *tview.Modal

	menuOpen      bool
	hintsOpen     bool
	goalOpen      bool
	journalOpen   bool
	resetOpen     bool
	infoOpen      bool
	referenceOpen bool
	diffOpen      bool

	lastEscAt     time.Time
	escQuitWindow time.Duration
}

type Options struct {
	ASCIIOnly bool
	Debug     bool
	TermPane  term.Pane
}

func New(opts Options) *Root {
	r := &Root{
		app:   tview.NewApplication(),
		theme: DefaultTheme(),
		ascii: opts.ASCIIOnly,
		debug: opts.Debug,
		term:  opts.TermPane,
		state: PlayingState{ModeLabel: "Free Play", StartedAt: time.Now()},
		// Double-Esc provides an emergency escape hatch in hosts where F-keys are unreliable.
		escQuitWindow: 800 * time.Millisecond,
	}

	r.header = tview.NewTextView().SetDynamicColors(true)
	r.status = tview.NewTextView().SetDynamicColors(true)
	r.hud = tview.NewTextView().SetDynamicColors(true)
	r.hud.SetBorder(true).SetTitle(" HUD ")
	r.hud.SetWrap(true).SetScrollable(true)

	r.drawer = tview.NewTextView().SetDynamicColors(true)
	r.drawer.SetBorder(true).SetTitle(" HUD Drawer ")

	r.body = tview.NewFlex().SetDirection(tview.FlexColumn)
	r.main = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(r.header, 1, 0, false).
		AddItem(r.body, 0, 1, true).
		AddItem(r.status, 1, 0, false)

	r.pages = tview.NewPages().AddPage("main", r.main, true, true)
	r.buildStaticModals()

	r.app.SetRoot(r.pages, true)
	r.app.SetInputCapture(r.captureInput)
	r.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		w, h := screen.Size()
		r.cols, r.rows = w, h
		r.applyLayout(DetermineLayoutMode(w, h), w, h)
		if r.ctrl != nil {
			r.ctrl.OnResize(w, h)
		}
		return false
	})

	r.refreshHeader()
	r.refreshStatus()
	r.refreshHUD()
	r.applyLayout(LayoutWide, 120, 30)
	return r
}

func (r *Root) buildStaticModals() {
	r.menuModal = tview.NewModal().
		SetText("Menu").
		AddButtons([]string{"Continue", "Restart level", "Change level", "Settings", "Stats", "Quit"}).
		SetDoneFunc(func(_ int, label string) {
			switch label {
			case "Continue":
				r.SetMenuOpen(false)
			case "Restart level":
				r.SetMenuOpen(false)
				r.SetResetConfirmOpen(true)
			case "Change level":
				r.SetMenuOpen(false)
				if r.ctrl != nil {
					r.ctrl.OnChangeLevel()
				}
			case "Settings":
				r.SetMenuOpen(false)
				if r.ctrl != nil {
					r.ctrl.OnOpenSettings()
				}
			case "Stats":
				r.SetMenuOpen(false)
				if r.ctrl != nil {
					r.ctrl.OnOpenStats()
				}
			case "Quit":
				r.SetMenuOpen(false)
				if r.ctrl != nil {
					r.ctrl.OnQuit()
				}
			}
		})

	r.hintsModal = tview.NewModal().
		SetText("Hints").
		AddButtons([]string{"Reveal", "Close"}).
		SetDoneFunc(func(_ int, label string) {
			switch label {
			case "Reveal":
				if r.ctrl != nil {
					r.ctrl.OnRevealHint()
				}
				r.updateHintsModalText()
			case "Close":
				r.SetHintsOpen(false)
			}
		})

	r.journalModal = tview.NewModal().
		SetText("Journal").
		AddButtons([]string{"AI Explain", "Close"}).
		SetDoneFunc(func(_ int, label string) {
			switch label {
			case "AI Explain":
				if r.ctrl != nil {
					r.ctrl.OnJournalExplainAI()
				}
			case "Close":
				r.SetJournalOpen(false)
			}
		})

	r.resetModal = tview.NewModal().
		SetText("Reset will destroy current /work state. Continue?").
		AddButtons([]string{"Cancel", "Reset"}).
		SetDoneFunc(func(_ int, label string) {
			r.SetResetConfirmOpen(false)
			if label == "Reset" && r.ctrl != nil {
				r.ctrl.OnReset()
			}
		})

	r.infoModal = tview.NewModal().
		SetText("Info").
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(_ int, _ string) { r.SetInfo("", "", false) })

	r.referenceModal = tview.NewModal().
		SetText("Reference solutions").
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(_ int, _ string) { r.SetReferenceText("", false) })

	r.diffModal = tview.NewModal().
		SetText("Diff").
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(_ int, _ string) { r.SetDiffText("", false) })
}

func (r *Root) SetController(c Controller) { r.ctrl = c }

func (r *Root) Run() error {
	r.running = true
	defer func() { r.running = false }()
	return r.app.Run()
}

func (r *Root) Stop() { r.app.Stop() }

func (r *Root) SetPlayingState(s PlayingState) {
	r.state = s
	if r.state.StartedAt.IsZero() {
		r.state.StartedAt = time.Now()
	}
	r.refreshHeader()
	r.refreshHUD()
	r.refreshStatus()
	if r.hintsOpen {
		r.updateHintsModalText()
	}
	r.requestDraw()
}

func (r *Root) SetTooSmall(cols, rows int) {
	r.applyLayout(LayoutTooSmall, cols, rows)
	r.requestDraw()
}

func (r *Root) SetSetupError(msg, details string) {
	r.setupMsg = msg
	r.setupDetails = details
	box := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf("[red]Setup error[-]\n\n%s\n\n%s", msg, details)).
		SetBorder(true).
		SetTitle(" Setup Wizard ")
	r.pages.AddAndSwitchToPage("setup_error", center(90, 20, box), true)
	r.requestDraw()
}

func (r *Root) SetMenuOpen(open bool) {
	r.menuOpen = open
	if open {
		r.pages.AddAndSwitchToPage("menu", center(60, 20, r.menuModal), true)
	} else {
		r.pages.RemovePage("menu")
	}
	r.requestDraw()
}

func (r *Root) SetHintsOpen(open bool) {
	r.hintsOpen = open
	if open {
		r.updateHintsModalText()
		r.pages.AddAndSwitchToPage("hints", center(84, 22, r.hintsModal), true)
	} else {
		r.pages.RemovePage("hints")
	}
	r.requestDraw()
}

func (r *Root) updateHintsModalText() {
	var b strings.Builder
	b.WriteString("Hints\n\n")
	for i, h := range r.state.Hints {
		status := "[green]available[-]"
		text := h.Text
		if h.Locked {
			status = "[yellow]locked[-]"
			if h.LockReason != "" {
				status = fmt.Sprintf("[yellow]locked[-] (%s)", h.LockReason)
			}
			text = "(hidden)"
		}
		if h.Revealed {
			status = "[cyan]revealed[-]"
		}
		b.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, status, text))
	}
	b.WriteString("\nReveal increments hint penalty immediately.")
	r.hintsModal.SetText(b.String())
}

func (r *Root) SetGoalOpen(open bool) {
	r.goalOpen = open
	r.applyLayout(r.layout, 0, 0)
	r.requestDraw()
}

func (r *Root) SetJournalOpen(open bool) {
	r.journalOpen = open
	if open {
		r.updateJournalModalText()
		r.pages.AddAndSwitchToPage("journal", center(100, 26, r.journalModal), true)
	} else {
		r.pages.RemovePage("journal")
	}
	r.requestDraw()
}

func (r *Root) SetResetConfirmOpen(open bool) {
	r.resetOpen = open
	if open {
		r.pages.AddAndSwitchToPage("reset", center(70, 12, r.resetModal), true)
	} else {
		r.pages.RemovePage("reset")
	}
	r.requestDraw()
}

func (r *Root) SetJournalEntries(entries []JournalEntry) {
	r.journalEntries = append([]JournalEntry(nil), entries...)
	if r.journalOpen {
		r.updateJournalModalText()
		r.requestDraw()
	}
}

func (r *Root) updateJournalModalText() {
	if len(r.journalEntries) == 0 {
		r.journalModal.SetText("Journal\n\nNo commands logged yet.")
		return
	}
	var b strings.Builder
	b.WriteString("Journal (/work/.dojo_cmdlog)\n\n")
	for _, e := range r.journalEntries {
		tagText := ""
		if len(e.Tags) > 0 {
			tagText = " [" + strings.Join(e.Tags, ",") + "]"
		}
		b.WriteString(fmt.Sprintf("%s  %s%s\n", e.Timestamp, e.Command, tagText))
	}
	r.journalModal.SetText(b.String())
}

func (r *Root) SetReferenceText(text string, open bool) {
	r.referenceText = text
	r.referenceOpen = open
	if open {
		r.referenceModal.SetText("Reference Solutions\n\n" + text)
		r.pages.AddAndSwitchToPage("reference", center(110, 28, r.referenceModal), true)
	} else {
		r.pages.RemovePage("reference")
	}
	r.requestDraw()
}

func (r *Root) SetDiffText(text string, open bool) {
	r.diffText = text
	r.diffOpen = open
	if open {
		r.diffModal.SetText("Artifact Diff\n\n" + text)
		r.pages.AddAndSwitchToPage("diff", center(110, 28, r.diffModal), true)
	} else {
		r.pages.RemovePage("diff")
	}
	r.requestDraw()
}

func (r *Root) SetInfo(title, text string, open bool) {
	r.infoTitle = title
	r.infoText = text
	r.infoOpen = open
	if open {
		r.infoModal.SetText(title + "\n\n" + text)
		r.pages.AddAndSwitchToPage("info", center(90, 22, r.infoModal), true)
	} else {
		r.pages.RemovePage("info")
	}
	r.requestDraw()
}

func (r *Root) SetResult(s ResultState) {
	r.result = s
	if !s.Visible {
		r.pages.RemovePage("result")
		r.requestDraw()
		return
	}

	banner := "[red::b]FAIL[-:-:-]"
	if s.Passed {
		banner = "[green::b]PASS[-:-:-]"
	}
	var b strings.Builder
	b.WriteString(banner)
	b.WriteString("\n\n")
	b.WriteString(s.Summary)
	b.WriteString("\n\n")
	for _, c := range s.Checks {
		mark := "✗"
		if c.Passed {
			mark = "✓"
		}
		b.WriteString(fmt.Sprintf("%s %s: %s\n", mark, c.ID, c.Message))
	}
	if len(s.Breakdown) > 0 {
		b.WriteString("\nScoring\n")
		for _, row := range s.Breakdown {
			b.WriteString(fmt.Sprintf("- %s: %s\n", row.Label, row.Value))
		}
	}
	b.WriteString(fmt.Sprintf("\nFinal Score: %d", s.Score))

	buttons := []string{}
	if s.CanShowReference {
		buttons = append(buttons, "Show reference solutions")
	}
	if s.CanOpenDiff {
		buttons = append(buttons, "Open diff")
	}
	primary := s.PrimaryAction
	if primary == "" {
		if s.Passed {
			primary = "Continue"
		} else {
			primary = "Try again"
		}
	}
	buttons = append(buttons, primary, "Close")

	r.resultModal = tview.NewModal().
		SetText(b.String()).
		AddButtons(buttons).
		SetDoneFunc(func(_ int, label string) {
			switch label {
			case "Show reference solutions":
				if r.ctrl != nil {
					r.ctrl.OnShowReferenceSolutions()
				}
			case "Open diff":
				if r.ctrl != nil {
					r.ctrl.OnOpenDiff()
				}
			case primary:
				r.SetResult(ResultState{})
				if r.ctrl != nil {
					if s.Passed {
						r.ctrl.OnNextLevel()
					} else {
						r.ctrl.OnTryAgain()
					}
				}
			default:
				r.SetResult(ResultState{})
				if r.ctrl != nil {
					r.ctrl.OnTryAgain()
				}
			}
		})

	r.pages.AddAndSwitchToPage("result", center(110, 30, r.resultModal), true)
	r.requestDraw()
}

func (r *Root) FlashStatus(msg string) {
	r.statusFlash = msg
	r.refreshStatus()
	r.requestDraw()
}

func (r *Root) refreshHeader() {
	elapsed := time.Since(r.state.StartedAt).Truncate(time.Second)
	r.header.SetBackgroundColor(r.theme.HeaderBG)
	r.header.SetTextColor(r.theme.HeaderFG)
	r.header.SetText(fmt.Sprintf(" CLI Dojo | %s | %s/%s | %s | Engine: %s ", r.state.ModeLabel, r.state.PackID, r.state.LevelID, elapsed, r.state.Engine))
}

func (r *Root) refreshStatus() {
	r.status.SetBackgroundColor(r.theme.StatusBG)
	r.status.SetTextColor(r.theme.StatusFG)
	text := " [F1] Hints  [F2] Goal  [F4] Journal  [F5] Check  [F6] Reset  [F9] Scrollback  [F10] Menu "
	if r.statusFlash != "" {
		text += "| " + r.statusFlash
	}
	r.status.SetText(text)
}

func (r *Root) refreshHUD() {
	var b strings.Builder
	b.WriteString("[::b]Objective[-:-:-]\n")
	for _, obj := range r.state.Objective {
		b.WriteString("- " + obj + "\n")
	}
	b.WriteString("\n[::b]Checks[-:-:-]\n")
	for _, c := range r.state.Checks {
		icon := "○"
		if c.Status == "pass" {
			icon = "✓"
		}
		if c.Status == "fail" {
			icon = "✗"
		}
		b.WriteString(fmt.Sprintf("%s %s\n", icon, c.Description))
	}
	b.WriteString("\n[::b]Hints[-:-:-]\n")
	for i, h := range r.state.Hints {
		status := "available"
		text := h.Text
		if h.Locked {
			status = "locked"
			text = "(hidden)"
		}
		if h.Revealed {
			status = "revealed"
		}
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, status, text))
	}
	b.WriteString("\n[::b]Score[-:-:-]\n")
	b.WriteString(fmt.Sprintf("Current: %d\nHints: %d  Resets: %d\nStreak: %d\n", r.state.Score, r.state.HintsUsed, r.state.Resets, r.state.Streak))
	if len(r.state.Badges) > 0 {
		b.WriteString("\n[::b]Badges[-:-:-]\n")
		for _, badge := range r.state.Badges {
			b.WriteString("- " + badge + "\n")
		}
	}
	r.hud.SetText(b.String())
	r.drawer.SetText(b.String())
}

func (r *Root) applyLayout(mode LayoutMode, cols, rows int) {
	if cols == 0 || rows == 0 {
		cols, rows = r.cols, r.rows
	}
	r.layout = mode
	r.body.Clear()

	switch mode {
	case LayoutTooSmall:
		msg := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
		msg.SetText(fmt.Sprintf("[yellow]Terminal too small[-]\nCurrent: %dx%d\nMinimum: 90x24", cols, rows))
		msg.SetBorder(true).SetTitle(" Resize Required ")
		r.body.AddItem(msg, 0, 1, false)
		r.pages.RemovePage("drawer")
	case LayoutWide:
		r.pages.RemovePage("drawer")
		r.body.AddItem(r.hud, 40, 0, r.state.HudFocused)
		r.body.AddItem(r.term.Primitive(), 0, 1, !r.state.HudFocused)
	default:
		r.body.AddItem(r.term.Primitive(), 0, 1, true)
		if r.goalOpen {
			r.pages.AddPage("drawer", leftDrawer(38, r.drawer), true, true)
		} else {
			r.pages.RemovePage("drawer")
		}
	}
}

func (r *Root) overlayActive() bool {
	return r.menuOpen || r.hintsOpen || r.journalOpen || r.resetOpen || r.infoOpen || r.referenceOpen || r.diffOpen || r.result.Visible
}

func (r *Root) closeTopOverlay() {
	switch {
	case r.diffOpen:
		r.SetDiffText("", false)
	case r.referenceOpen:
		r.SetReferenceText("", false)
	case r.infoOpen:
		r.SetInfo("", "", false)
	case r.resetOpen:
		r.SetResetConfirmOpen(false)
	case r.result.Visible:
		r.SetResult(ResultState{})
	case r.journalOpen:
		r.SetJournalOpen(false)
	case r.hintsOpen:
		r.SetHintsOpen(false)
	case r.menuOpen:
		r.SetMenuOpen(false)
	case r.goalOpen:
		r.SetGoalOpen(false)
	}
}

func (r *Root) captureInput(ev *tcell.EventKey) *tcell.EventKey {
	if ev == nil {
		return ev
	}

	if r.overlayActive() {
		if ev.Key() == tcell.KeyEsc {
			r.lastEscAt = time.Time{}
			r.closeTopOverlay()
			return nil
		}
		return ev
	}

	if ev.Key() != tcell.KeyEsc {
		r.lastEscAt = time.Time{}
	}

	if ev.Key() == tcell.KeyTab {
		if r.layout == LayoutWide {
			r.state.HudFocused = !r.state.HudFocused
			if r.state.HudFocused {
				r.app.SetFocus(r.hud)
			} else {
				r.app.SetFocus(r.term.Primitive())
			}
			r.refreshHUD()
			r.applyLayout(r.layout, 0, 0)
			r.requestDraw()
			return nil
		}
		if r.layout == LayoutMedium && r.goalOpen {
			r.state.HudFocused = !r.state.HudFocused
			if r.state.HudFocused {
				r.app.SetFocus(r.drawer)
			} else {
				r.app.SetFocus(r.term.Primitive())
			}
			return nil
		}
	}

	switch ev.Key() {
	case tcell.KeyF1:
		if r.ctrl != nil {
			r.ctrl.OnHints()
		}
		return nil
	case tcell.KeyF2:
		if r.ctrl != nil {
			r.ctrl.OnGoal()
		}
		return nil
	case tcell.KeyF4:
		if r.ctrl != nil {
			r.ctrl.OnJournal()
		}
		return nil
	case tcell.KeyF5:
		if r.ctrl != nil {
			r.ctrl.OnCheck()
		}
		return nil
	case tcell.KeyF6:
		r.SetResetConfirmOpen(true)
		return nil
	case tcell.KeyF9:
		r.term.ToggleScrollback()
		return nil
	case tcell.KeyF10:
		if r.ctrl != nil {
			r.ctrl.OnMenu()
		}
		return nil
	case tcell.KeyEsc:
		if r.goalOpen {
			r.SetGoalOpen(false)
			return nil
		}
		if r.term.InScrollback() {
			r.term.ToggleScrollback()
			return nil
		}
		now := time.Now()
		if !r.lastEscAt.IsZero() && now.Sub(r.lastEscAt) <= r.escQuitWindow {
			r.lastEscAt = time.Time{}
			if r.ctrl != nil {
				r.ctrl.OnQuit()
			}
			return nil
		}
		r.lastEscAt = now
		r.FlashStatus("Press Esc again quickly to quit")
	}

	if ev.Key() == tcell.KeyPgUp && ev.Modifiers()&tcell.ModShift != 0 {
		if !r.term.InScrollback() {
			r.term.ToggleScrollback()
		}
		r.term.Scroll(-10)
		return nil
	}
	if ev.Key() == tcell.KeyPgDn && ev.Modifiers()&tcell.ModShift != 0 {
		if !r.term.InScrollback() {
			r.term.ToggleScrollback()
		}
		r.term.Scroll(10)
		return nil
	}
	if r.term.InScrollback() {
		switch ev.Key() {
		case tcell.KeyUp:
			r.term.Scroll(-1)
		case tcell.KeyDown:
			r.term.Scroll(1)
		case tcell.KeyPgUp:
			r.term.Scroll(-10)
		case tcell.KeyPgDn:
			r.term.Scroll(10)
		}
		return nil
	}

	if r.ctrl != nil {
		r.ctrl.OnTerminalInput(term.EncodeEventToBytes(ev))
	}
	return nil
}

func (r *Root) requestDraw() {
	if !r.running {
		return
	}
	r.app.QueueUpdateDraw(func() {})
}

func center(w, h int, p tview.Primitive) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, h, 1, true).
			AddItem(nil, 0, 1, false), w, 1, true).
		AddItem(nil, 0, 1, false)
}

func leftDrawer(width int, p tview.Primitive) tview.Primitive {
	return tview.NewFlex().
		AddItem(p, width, 0, false).
		AddItem(nil, 0, 1, false)
}

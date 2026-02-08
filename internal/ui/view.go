package ui

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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
	screen  Screen
	cols    int
	rows    int
	running bool

	lastCols    int
	lastRows    int
	layoutDirty bool

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

	header *tview.TextView
	status *tview.TextView
	hud    *tview.TextView
	body   *tview.Flex
	game   *tview.Flex
	pages  *tview.Pages

	drawer *tview.TextView

	mainMenuList   *tview.List
	mainMenuInfo   *tview.TextView
	mainMenuRoot   *tview.Flex
	levelPackList  *tview.List
	levelLevelList *tview.List
	levelDetail    *tview.TextView
	levelRoot      *tview.Flex

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

	drawPending atomic.Bool
	drawReady   atomic.Bool
	drawMu      sync.Mutex
	drawStopCh  chan struct{}
}

type Options struct {
	ASCIIOnly bool
	Debug     bool
	TermPane  term.Pane
}

func New(opts Options) *Root {
	r := &Root{
		app:         tview.NewApplication(),
		theme:       DefaultTheme(),
		ascii:       opts.ASCIIOnly,
		debug:       opts.Debug,
		term:        opts.TermPane,
		screen:      ScreenMainMenu,
		state:       PlayingState{ModeLabel: "Free Play", StartedAt: time.Now(), HudWidth: 42},
		lastCols:    -1,
		lastRows:    -1,
		layoutDirty: true,
	}

	r.header = tview.NewTextView().SetDynamicColors(true)
	r.status = tview.NewTextView().SetDynamicColors(true)
	r.hud = tview.NewTextView().SetDynamicColors(true)
	r.hud.SetBorder(true).SetTitle(" HUD ")
	r.hud.SetWrap(true).SetScrollable(true)

	r.drawer = tview.NewTextView().SetDynamicColors(true)
	r.drawer.SetBorder(true).SetTitle(" HUD Drawer ")

	r.body = tview.NewFlex().SetDirection(tview.FlexColumn)
	r.game = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(r.header, 1, 0, false).
		AddItem(r.body, 0, 1, true).
		AddItem(r.status, 1, 0, false)

	r.buildMainMenu()
	r.buildLevelSelect()

	r.pages = tview.NewPages().
		AddPage("main_menu", r.mainMenuRoot, true, true).
		AddPage("level_select", r.levelRoot, true, false).
		AddPage("game", r.game, true, false)
	r.buildStaticModals()

	r.app.SetRoot(r.pages, true)
	r.app.SetInputCapture(r.captureInput)
	r.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		w, h := screen.Size()
		sizeChanged := w != r.lastCols || h != r.lastRows
		if sizeChanged {
			r.cols, r.rows = w, h
			r.lastCols, r.lastRows = w, h
			if r.ctrl != nil && r.screen == ScreenPlaying {
				r.layoutDirty = true
				r.requestDraw()
				r.ctrl.OnResize(w, h)
			}
		}
		return false
	})
	r.app.SetAfterDrawFunc(func(_ tcell.Screen) {
		// Mark the application draw loop as ready only after tview has
		// completed at least one draw. This avoids pre-run QueueUpdateDraw calls
		// racing with initialization in some PTY/webterm environments.
		r.drawReady.Store(true)
	})

	r.refreshHeader()
	r.refreshStatus()
	r.refreshHUD()
	r.refreshMainMenu()
	r.refreshLevelSelect()
	r.applyLayout(LayoutWide, 120, 30)
	r.SetScreen(ScreenMainMenu)
	return r
}

func (r *Root) buildMainMenu() {
	r.mainMenuList = tview.NewList()
	r.mainMenuList.ShowSecondaryText(false)
	r.mainMenuList.SetBorder(true).SetTitle(" Main Menu ")
	r.mainMenuList.SetChangedFunc(func(index int, _ string, _ string, _ rune) {
		r.refreshMainMenuInfo(index)
	})
	r.mainMenuList.SetSelectedFunc(func(_ int, _, secondary string, _ rune) {
		if r.ctrl == nil {
			return
		}
		switch secondary {
		case "continue":
			r.ctrl.OnContinue()
		case "daily":
			r.ctrl.OnContinue()
		case "select":
			r.ctrl.OnOpenLevelSelect()
		case "settings":
			r.ctrl.OnOpenSettings()
		case "stats":
			r.ctrl.OnOpenStats()
		case "quit":
			r.ctrl.OnQuit()
		}
	})
	r.mainMenuInfo = tview.NewTextView().SetDynamicColors(true)
	r.mainMenuInfo.SetBorder(true).SetTitle(" Overview ")
	r.mainMenuInfo.SetWrap(true)

	r.mainMenuRoot = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(r.mainMenuList, 34, 0, true).
		AddItem(r.mainMenuInfo, 0, 1, false)
}

func (r *Root) buildLevelSelect() {
	r.levelPackList = tview.NewList()
	r.levelPackList.ShowSecondaryText(false)
	r.levelPackList.SetBorder(true).SetTitle(" Packs ")
	r.levelPackList.SetChangedFunc(func(index int, _, _ string, _ rune) {
		r.handlePackSelection(index)
	})

	r.levelLevelList = tview.NewList()
	r.levelLevelList.ShowSecondaryText(false)
	r.levelLevelList.SetBorder(true).SetTitle(" Levels ")
	r.levelLevelList.SetChangedFunc(func(index int, _, _ string, _ rune) {
		r.handleLevelSelection(index)
	})
	r.levelLevelList.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		r.startSelectedLevel(index)
	})

	r.levelDetail = tview.NewTextView().SetDynamicColors(true)
	r.levelDetail.SetBorder(true).SetTitle(" Details ")
	r.levelDetail.SetWrap(true)

	lists := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(r.levelPackList, 32, 0, true).
		AddItem(r.levelLevelList, 42, 0, false)

	r.levelRoot = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(lists, 74, 0, true).
		AddItem(r.levelDetail, 0, 1, false)
}

func (r *Root) refreshMainMenu() {
	if r.mainMenuList == nil {
		return
	}
	current := r.mainMenuList.GetCurrentItem()
	r.mainMenuList.Clear()
	r.mainMenuList.AddItem("Continue", "continue", 0, nil)
	r.mainMenuList.AddItem("Daily Drill", "daily", 0, nil)
	r.mainMenuList.AddItem("Level Select", "select", 0, nil)
	r.mainMenuList.AddItem("Settings", "settings", 0, nil)
	r.mainMenuList.AddItem("Stats", "stats", 0, nil)
	r.mainMenuList.AddItem("Quit", "quit", 0, nil)
	if current >= 0 && current < r.mainMenuList.GetItemCount() {
		r.mainMenuList.SetCurrentItem(current)
	}
	if r.mainMenuList.GetItemCount() > 0 {
		r.refreshMainMenuInfo(r.mainMenuList.GetCurrentItem())
	}
}

func (r *Root) refreshMainMenuInfo(selected int) {
	if r.mainMenuInfo == nil {
		return
	}
	action := "Use Enter to select an option."
	switch selected {
	case 0:
		action = "Continue your most recent level run."
	case 1:
		action = "Daily drill uses a deterministic seed in dev mode."
	case 2:
		action = "Browse packs and choose a level."
	case 3:
		action = "Inspect runtime configuration."
	case 4:
		action = "Review local progress summary."
	case 5:
		action = "Exit CLI Dojo."
	}
	var b strings.Builder
	b.WriteString("[::b]CLI Dojo[-:-:-]\n\n")
	b.WriteString(fmt.Sprintf("Engine: %s\nPacks: %d  Levels: %d\n", firstNonEmptyStr(r.mainMenu.EngineName, "unknown"), r.mainMenu.PackCount, r.mainMenu.LevelCount))
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
	b.WriteString("\nAction:\n")
	b.WriteString(action)
	r.mainMenuInfo.SetText(b.String())
}

func (r *Root) refreshLevelSelect() {
	if r.levelPackList == nil || r.levelLevelList == nil {
		return
	}
	packCurrent := r.levelPackList.GetCurrentItem()
	r.levelPackList.Clear()
	for _, p := range r.catalog {
		r.levelPackList.AddItem(fmt.Sprintf("%s (%d)", p.Name, len(p.Levels)), p.PackID, 0, nil)
	}
	if r.levelPackList.GetItemCount() == 0 {
		r.levelLevelList.Clear()
		r.levelDetail.SetText("No packs loaded.")
		return
	}

	idx := 0
	if r.selectedPack != "" {
		for i, p := range r.catalog {
			if p.PackID == r.selectedPack {
				idx = i
				break
			}
		}
	} else if packCurrent >= 0 && packCurrent < r.levelPackList.GetItemCount() {
		idx = packCurrent
	}
	r.levelPackList.SetCurrentItem(idx)
	r.handlePackSelection(idx)
}

func (r *Root) handlePackSelection(index int) {
	if index < 0 || index >= len(r.catalog) {
		return
	}
	pack := r.catalog[index]
	r.selectedPack = pack.PackID

	levelCurrent := r.levelLevelList.GetCurrentItem()
	r.levelLevelList.Clear()
	for _, lv := range pack.Levels {
		label := fmt.Sprintf("%s  [d:%d  ~%dm]", lv.Title, lv.Difficulty, lv.EstimatedMinutes)
		r.levelLevelList.AddItem(label, lv.LevelID, 0, nil)
	}
	if len(pack.Levels) == 0 {
		r.levelDetail.SetText("No levels in this pack.")
		return
	}
	levelIdx := 0
	if r.selectedLevel != "" {
		for i, lv := range pack.Levels {
			if lv.LevelID == r.selectedLevel {
				levelIdx = i
				break
			}
		}
	} else if levelCurrent >= 0 && levelCurrent < r.levelLevelList.GetItemCount() {
		levelIdx = levelCurrent
	}
	r.levelLevelList.SetCurrentItem(levelIdx)
	r.handleLevelSelection(levelIdx)
}

func (r *Root) handleLevelSelection(index int) {
	pack := r.selectedPackSummary()
	if pack == nil || index < 0 || index >= len(pack.Levels) {
		return
	}
	lv := pack.Levels[index]
	r.selectedLevel = lv.LevelID

	var b strings.Builder
	b.WriteString(fmt.Sprintf("[::b]%s[-:-:-]\n", lv.Title))
	b.WriteString(fmt.Sprintf("ID: %s\nDifficulty: %d\nEstimated: %d min\n", lv.LevelID, lv.Difficulty, lv.EstimatedMinutes))
	if len(lv.ToolFocus) > 0 {
		b.WriteString("Tools: " + strings.Join(lv.ToolFocus, ", ") + "\n")
	}
	if strings.TrimSpace(lv.SummaryMD) != "" {
		b.WriteString("\n")
		b.WriteString(lv.SummaryMD)
		b.WriteString("\n")
	}
	if len(lv.ObjectiveBullets) > 0 {
		b.WriteString("\nObjectives:\n")
		for _, obj := range lv.ObjectiveBullets {
			b.WriteString("- " + obj + "\n")
		}
	}
	b.WriteString("\nPress Enter to start this level. Esc to return to main menu.")
	r.levelDetail.SetText(b.String())
}

func (r *Root) startSelectedLevel(index int) {
	pack := r.selectedPackSummary()
	if pack == nil || index < 0 || index >= len(pack.Levels) {
		return
	}
	lv := pack.Levels[index]
	r.selectedLevel = lv.LevelID
	if r.ctrl != nil {
		r.ctrl.OnStartLevel(pack.PackID, lv.LevelID)
	}
}

func (r *Root) selectedPackSummary() *PackSummary {
	for i := range r.catalog {
		if r.catalog[i].PackID == r.selectedPack {
			return &r.catalog[i]
		}
	}
	return nil
}

func (r *Root) buildStaticModals() {
	r.menuModal = tview.NewModal().
		SetText("Menu").
		AddButtons([]string{"Continue", "Restart level", "Level select", "Main menu", "Settings", "Stats", "Quit"}).
		SetDoneFunc(func(_ int, label string) {
			switch label {
			case "Continue":
				r.SetMenuOpen(false)
			case "Restart level":
				r.SetMenuOpen(false)
				r.SetResetConfirmOpen(true)
			case "Level select":
				r.SetMenuOpen(false)
				if r.ctrl != nil {
					r.ctrl.OnOpenLevelSelect()
				}
			case "Main menu":
				r.SetMenuOpen(false)
				if r.ctrl != nil {
					r.ctrl.OnOpenMainMenu()
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

func (r *Root) SetScreen(screen Screen) {
	r.withUI(func() {
		r.screen = screen
		switch screen {
		case ScreenMainMenu:
			r.stopDrawLoop()
			r.pages.SwitchToPage("main_menu")
			if r.mainMenuList != nil {
				r.app.SetFocus(r.mainMenuList)
			}
		case ScreenLevelSelect:
			r.stopDrawLoop()
			r.pages.SwitchToPage("level_select")
			if r.levelPackList != nil {
				r.app.SetFocus(r.levelPackList)
			}
		case ScreenPlaying:
			r.layoutDirty = true
			r.pages.SwitchToPage("game")
			r.startDrawLoop()
			r.requestDraw()
			if r.term != nil {
				r.app.SetFocus(r.term.Primitive())
			}
		}
	})
}

func (r *Root) SetMainMenuState(state MainMenuState) {
	r.withUI(func() {
		r.mainMenu = state
		idx := 0
		if r.mainMenuList != nil {
			idx = r.mainMenuList.GetCurrentItem()
		}
		r.refreshMainMenuInfo(idx)
	})
}

func (r *Root) SetCatalog(packs []PackSummary) {
	r.withUI(func() {
		r.catalog = append([]PackSummary(nil), packs...)
		r.refreshLevelSelect()
	})
}

func (r *Root) SetLevelSelection(packID, levelID string) {
	r.withUI(func() {
		r.selectedPack = packID
		r.selectedLevel = levelID
		r.refreshLevelSelect()
	})
}

func (r *Root) Run() error {
	r.running = true
	r.drawReady.Store(false)
	defer func() {
		r.running = false
		r.drawReady.Store(false)
		r.stopDrawLoop()
	}()
	return r.app.Run()
}

func (r *Root) Stop() {
	r.stopDrawLoop()
	r.app.Stop()
}

func (r *Root) SetPlayingState(s PlayingState) {
	r.withUI(func() {
		if s.HudWidth <= 0 {
			s.HudWidth = 42
		}
		if s.HudWidth != r.state.HudWidth {
			r.layoutDirty = true
		}
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
	})
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
	r.withUI(func() {
		r.menuOpen = open
		if open {
			r.pages.AddAndSwitchToPage("menu", center(60, 20, r.menuModal), true)
		} else {
			r.pages.RemovePage("menu")
		}
	})
}

func (r *Root) SetHintsOpen(open bool) {
	r.withUI(func() {
		r.hintsOpen = open
		if open {
			r.updateHintsModalText()
			r.pages.AddAndSwitchToPage("hints", center(84, 22, r.hintsModal), true)
		} else {
			r.pages.RemovePage("hints")
		}
	})
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
	r.withUI(func() {
		r.goalOpen = open
		r.layoutDirty = true
		r.requestDraw()
	})
}

func (r *Root) SetJournalOpen(open bool) {
	r.withUI(func() {
		r.journalOpen = open
		if open {
			r.updateJournalModalText()
			r.pages.AddAndSwitchToPage("journal", center(100, 26, r.journalModal), true)
		} else {
			r.pages.RemovePage("journal")
		}
	})
}

func (r *Root) SetResetConfirmOpen(open bool) {
	r.withUI(func() {
		r.resetOpen = open
		if open {
			r.pages.AddAndSwitchToPage("reset", center(70, 12, r.resetModal), true)
		} else {
			r.pages.RemovePage("reset")
		}
	})
}

func (r *Root) SetJournalEntries(entries []JournalEntry) {
	r.withUI(func() {
		r.journalEntries = append([]JournalEntry(nil), entries...)
		if r.journalOpen {
			r.updateJournalModalText()
		}
	})
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
	r.withUI(func() {
		r.referenceText = text
		r.referenceOpen = open
		if open {
			r.referenceModal.SetText("Reference Solutions\n\n" + text)
			r.pages.AddAndSwitchToPage("reference", center(110, 28, r.referenceModal), true)
		} else {
			r.pages.RemovePage("reference")
		}
	})
}

func (r *Root) SetDiffText(text string, open bool) {
	r.withUI(func() {
		r.diffText = text
		r.diffOpen = open
		if open {
			r.diffModal.SetText("Artifact Diff\n\n" + text)
			r.pages.AddAndSwitchToPage("diff", center(110, 28, r.diffModal), true)
		} else {
			r.pages.RemovePage("diff")
		}
	})
}

func (r *Root) SetInfo(title, text string, open bool) {
	r.withUI(func() {
		r.infoTitle = title
		r.infoText = text
		r.infoOpen = open
		if open {
			r.infoModal.SetText(title + "\n\n" + text)
			r.pages.AddAndSwitchToPage("info", center(90, 22, r.infoModal), true)
		} else {
			r.pages.RemovePage("info")
		}
	})
}

func (r *Root) SetResult(s ResultState) {
	r.withUI(func() {
		r.result = s
		if !s.Visible {
			r.pages.RemovePage("result")
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
	})
}

func (r *Root) FlashStatus(msg string) {
	r.withUI(func() {
		r.statusFlash = msg
		r.refreshStatus()
	})
}

func (r *Root) refreshHeader() {
	if r.screen != ScreenPlaying {
		return
	}
	elapsed := time.Since(r.state.StartedAt).Truncate(time.Second)
	r.header.SetBackgroundColor(r.theme.HeaderBG)
	r.header.SetTextColor(r.theme.HeaderFG)
	r.header.SetText(fmt.Sprintf(" CLI Dojo | %s | %s/%s | %s | Engine: %s ", r.state.ModeLabel, r.state.PackID, r.state.LevelID, elapsed, r.state.Engine))
}

func (r *Root) refreshStatus() {
	if r.screen != ScreenPlaying {
		return
	}
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
		hudWidth := r.state.HudWidth
		if hudWidth <= 0 {
			hudWidth = 42
		}
		r.body.AddItem(r.hud, hudWidth, 0, false)
		r.body.AddItem(r.term.Primitive(), 0, 1, true)
	default:
		r.body.AddItem(r.term.Primitive(), 0, 1, true)
		if r.goalOpen {
			drawerWidth := r.state.HudWidth
			if drawerWidth <= 0 {
				drawerWidth = 38
			}
			r.pages.AddPage("drawer", leftDrawer(drawerWidth, r.drawer), true, true)
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
	if ev.Key() == tcell.KeyCtrlQ {
		if r.ctrl != nil {
			r.ctrl.OnQuit()
		}
		return nil
	}

	if r.overlayActive() {
		if ev.Key() == tcell.KeyEsc {
			r.closeTopOverlay()
			return nil
		}
		return ev
	}

	if r.screen == ScreenMainMenu {
		if ev.Key() == tcell.KeyEsc {
			if r.ctrl != nil {
				r.ctrl.OnQuit()
			}
			return nil
		}
		if ev.Key() == tcell.KeyEnter || ev.Key() == tcell.KeyCtrlM || (ev.Key() == tcell.KeyRune && (ev.Rune() == '\n' || ev.Rune() == '\r')) {
			r.activateMainMenuSelection()
			return nil
		}
		return ev
	}
	if r.screen == ScreenLevelSelect {
		if ev.Key() == tcell.KeyEsc {
			if r.ctrl != nil {
				r.ctrl.OnBackToMainMenu()
			}
			return nil
		}
		if ev.Key() == tcell.KeyRight || ev.Key() == tcell.KeyTab {
			r.app.SetFocus(r.levelLevelList)
			return nil
		}
		if ev.Key() == tcell.KeyLeft || ev.Key() == tcell.KeyBacktab {
			r.app.SetFocus(r.levelPackList)
			return nil
		}
		if ev.Key() == tcell.KeyEnter || ev.Key() == tcell.KeyCtrlM || (ev.Key() == tcell.KeyRune && (ev.Rune() == '\n' || ev.Rune() == '\r')) {
			if r.app.GetFocus() == r.levelPackList {
				r.app.SetFocus(r.levelLevelList)
				return nil
			}
			r.startSelectedLevel(r.levelLevelList.GetCurrentItem())
			return nil
		}
		return ev
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

func (r *Root) activateMainMenuSelection() {
	if r.mainMenuList == nil || r.ctrl == nil {
		return
	}
	index := r.mainMenuList.GetCurrentItem()
	if index < 0 || index >= r.mainMenuList.GetItemCount() {
		return
	}
	_, secondary := r.mainMenuList.GetItemText(index)
	switch secondary {
	case "continue", "daily":
		r.ctrl.OnContinue()
	case "select":
		r.ctrl.OnOpenLevelSelect()
	case "settings":
		r.ctrl.OnOpenSettings()
	case "stats":
		r.ctrl.OnOpenStats()
	case "quit":
		r.ctrl.OnQuit()
	}
}

func (r *Root) RequestDraw() {
	if !r.running {
		return
	}
	r.drawPending.Store(true)
}

func (r *Root) requestDraw() {
	r.RequestDraw()
}

func (r *Root) withUI(fn func()) {
	if fn == nil {
		return
	}
	if !r.running {
		fn()
		return
	}
	r.PostTask(fn)
}

// RunOnUI schedules fn to run on the next UI draw cycle.
func (r *Root) RunOnUI(fn func()) {
	if fn == nil {
		return
	}
	r.PostTask(fn)
}

// PostTask schedules fn to run on the UI thread on the next draw cycle.
func (r *Root) PostTask(fn func()) {
	if fn == nil {
		return
	}
	if !r.running {
		fn()
		return
	}
	r.app.QueueUpdateDraw(fn)
}

// WaitForUI waits until all currently queued UI tasks are processed.
func (r *Root) WaitForUI(timeout time.Duration) bool {
	if !r.running {
		return true
	}
	done := make(chan struct{}, 1)
	r.PostTask(func() {
		done <- struct{}{}
	})
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (r *Root) startDrawLoop() {
	r.drawMu.Lock()
	if r.drawStopCh != nil {
		r.drawMu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	r.drawStopCh = stopCh
	r.drawMu.Unlock()

	go func() {
		ticker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		lastHeaderTick := time.Now()
		for {
			select {
			case <-ticker.C:
				if !r.running {
					continue
				}
				if r.screen == ScreenPlaying && time.Since(lastHeaderTick) >= time.Second {
					lastHeaderTick = time.Now()
					r.drawPending.Store(true)
				}
				if r.drawPending.Swap(false) {
					r.app.QueueUpdateDraw(func() {
						if r.screen == ScreenPlaying {
							mode := DetermineLayoutMode(r.cols, r.rows)
							if r.layoutDirty || mode != r.layout {
								r.applyLayout(mode, r.cols, r.rows)
								r.layoutDirty = false
							}
						}
						r.refreshHeader()
					})
				}
			case <-stopCh:
				return
			}
		}
	}()
}

func (r *Root) stopDrawLoop() {
	r.drawMu.Lock()
	stopCh := r.drawStopCh
	r.drawStopCh = nil
	r.drawMu.Unlock()
	if stopCh != nil {
		close(stopCh)
	}
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

func firstNonEmptyStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

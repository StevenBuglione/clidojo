package ui

import (
	"context"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"clidojo/internal/term"
)

type mockController struct {
	mu            sync.Mutex
	continueCalls int
	quitCalls     int
	resetCalls    int
	menuCalls     int
	inputs        [][]byte
}

func (m *mockController) OnContinue() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.continueCalls++
}
func (m *mockController) OnOpenLevelSelect()          {}
func (m *mockController) OnStartLevel(string, string) {}
func (m *mockController) OnBackToMainMenu()           {}
func (m *mockController) OnOpenMainMenu()             {}
func (m *mockController) OnCheck()                    {}
func (m *mockController) OnReset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetCalls++
}
func (m *mockController) OnMenu() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.menuCalls++
}
func (m *mockController) OnHints()   {}
func (m *mockController) OnGoal()    {}
func (m *mockController) OnJournal() {}
func (m *mockController) OnQuit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quitCalls++
}
func (m *mockController) OnResize(int, int) {}
func (m *mockController) OnTerminalInput(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]byte(nil), data...)
	m.inputs = append(m.inputs, cp)
}
func (m *mockController) OnChangeLevel()            {}
func (m *mockController) OnOpenSettings()           {}
func (m *mockController) OnOpenStats()              {}
func (m *mockController) OnRevealHint()             {}
func (m *mockController) OnNextLevel()              {}
func (m *mockController) OnTryAgain()               {}
func (m *mockController) OnShowReferenceSolutions() {}
func (m *mockController) OnOpenDiff()               {}
func (m *mockController) OnJournalExplainAI()       {}

func (m *mockController) ContinueCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.continueCalls
}

func (m *mockController) QuitCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.quitCalls
}

func (m *mockController) ResetCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resetCalls
}

func (m *mockController) MenuCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.menuCalls
}

func (m *mockController) Inputs() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]byte, len(m.inputs))
	for i := range m.inputs {
		out[i] = append([]byte(nil), m.inputs[i]...)
	}
	return out
}

type bracketedPane struct {
	*term.TerminalPane
}

func (p bracketedPane) BracketedPasteEnabled() bool { return true }

type panicSnapshotPane struct {
	*term.TerminalPane
}

func (p panicSnapshotPane) Snapshot(width, height int) term.Snapshot {
	panic("snapshot boom")
}

func press(v *Root, code rune, mod tea.KeyMod, text string) {
	_, _ = v.Update(tea.KeyPressMsg{Code: code, Mod: mod, Text: text})
}

func TestF6OpensResetConfirmWithoutImmediateReset(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	press(v, tea.KeyF6, 0, "")

	if ctrl.ResetCalls() != 0 {
		t.Fatalf("expected no immediate reset call")
	}
	if !v.resetOpen {
		t.Fatalf("expected reset confirm modal to be open")
	}
}

func TestOverlayEscClosesTopModal(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetResult(ResultState{Visible: true, Passed: false, Summary: "x", PrimaryAction: "Try again"})

	press(v, tea.KeyEsc, 0, "")
	if v.result.Visible {
		t.Fatalf("expected result modal to close on escape")
	}
}

func TestOverlayQClosesReferenceModal(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetReferenceText("### ref\ncontent", true)

	press(v, 'q', 0, "q")
	if v.referenceOpen {
		t.Fatalf("expected reference modal to close on q")
	}
}

func TestF10FromResultOverlayOpensPauseMenu(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)
	v.SetResult(ResultState{Visible: true, Passed: false, Summary: "x", PrimaryAction: "Try again"})

	press(v, tea.KeyF10, 0, "")
	if v.result.Visible {
		t.Fatalf("expected result modal to close before opening menu")
	}
	if !v.menuOpen {
		t.Fatalf("expected pause menu to open")
	}
	deadline := time.Now().Add(300 * time.Millisecond)
	for ctrl.MenuCalls() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ctrl.MenuCalls() == 0 {
		t.Fatalf("expected controller menu toggle")
	}
}

func TestF10FromMenuOverlayClosesPauseMenu(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)
	v.SetMenuOpen(true)

	press(v, tea.KeyF10, 0, "")
	if v.menuOpen {
		t.Fatalf("expected pause menu to close")
	}
	deadline := time.Now().Add(300 * time.Millisecond)
	for ctrl.MenuCalls() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ctrl.MenuCalls() == 0 {
		t.Fatalf("expected controller menu toggle")
	}
}

func TestOverlayEnterHandlesModalAction(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetMenuOpen(true)

	press(v, tea.KeyEnter, 0, "")
	if v.menuOpen {
		t.Fatalf("expected menu action to close menu")
	}
}

func TestEscPassesThroughToTerminal(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	press(v, tea.KeyEsc, 0, "")

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(ctrl.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	inputs := ctrl.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "\x1b" {
		t.Fatalf("expected escape to be forwarded to terminal")
	}
}

func TestTabPassesThroughToTerminal(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	press(v, tea.KeyTab, 0, "")

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(ctrl.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	inputs := ctrl.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "\t" {
		t.Fatalf("expected tab to be forwarded to terminal")
	}
}

func TestPasteMsgPassesThroughToTerminal(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	_, _ = v.Update(tea.PasteMsg{Content: "echo hi\npwd\n"})

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(ctrl.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	inputs := ctrl.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "echo hi\npwd\n" {
		t.Fatalf("expected pasted content to be forwarded unchanged")
	}
}

func TestPasteMsgIgnoredWhenOverlayOpen(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)
	v.SetMenuOpen(true)

	_, _ = v.Update(tea.PasteMsg{Content: "echo should_not_send\n"})
	time.Sleep(50 * time.Millisecond)

	if len(ctrl.Inputs()) != 0 {
		t.Fatalf("expected paste to be ignored while overlay is open")
	}
}

func TestPasteMsgUsesBracketedPasteWhenEnabled(t *testing.T) {
	pane := bracketedPane{TerminalPane: term.NewTerminalPane(nil)}
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	_, _ = v.Update(tea.PasteMsg{Content: "echo hi\npwd\n"})

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(ctrl.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	inputs := ctrl.Inputs()
	if len(inputs) != 1 {
		t.Fatalf("expected bracketed paste content to be forwarded")
	}
	if string(inputs[0]) != "\x1b[200~echo hi\npwd\n\x1b[201~" {
		t.Fatalf("unexpected bracketed paste payload: %q", string(inputs[0]))
	}
}

func TestClipboardMsgPassesThroughToTerminal(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	_, _ = v.Update(tea.ClipboardMsg{Content: "echo from clipboard\n"})

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(ctrl.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	inputs := ctrl.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "echo from clipboard\n" {
		t.Fatalf("expected clipboard content to be forwarded unchanged")
	}
}

func TestOverlayCopyShortcutSetsStatusFlash(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetResult(ResultState{
		Visible: true,
		Passed:  true,
		Summary: "ok",
		Checks:  []CheckResultRow{{ID: "c1", Passed: true, Message: "pass"}},
		Score:   1000,
	})

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl, Text: "c"})
	if cmd == nil {
		t.Fatalf("expected clipboard command")
	}
	if v.statusFlash == "" {
		t.Fatalf("expected status flash after copy")
	}
}

func TestStatusShowsCheckingSpinner(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetPlayingState(PlayingState{
		ModeLabel: "Free Play",
		PackID:    "builtin-core",
		LevelID:   "level-001",
		StartedAt: time.Now(),
		HudWidth:  42,
	})
	v.SetChecking(true)

	_, _ = v.Update(spinnerTickCmd(v.checkSpin)())
	out := v.statusText()
	if !strings.Contains(out, "Checking") {
		t.Fatalf("expected status to include checking spinner text")
	}
}

func TestHUDShowsMasterySection(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetPlayingState(PlayingState{
		ModeLabel: "Free Play",
		PackID:    "builtin-core",
		LevelID:   "level-001",
		StartedAt: time.Now(),
		HudWidth:  42,
		Score:     880,
		Checks: []CheckRow{
			{ID: "a", Description: "first", Status: "pass"},
			{ID: "b", Description: "second", Status: "pending"},
		},
	})

	out := v.hudText()
	if !strings.Contains(out, "Mastery") {
		t.Fatalf("expected HUD to include mastery section")
	}
}

func TestJournalCopyCurrentUsesSelection(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetJournalEntries([]JournalEntry{
		{Timestamp: "t1", Command: "echo one"},
		{Timestamp: "t2", Command: "echo two"},
	})
	v.SetJournalOpen(true)
	v.journalIndex = 1

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatalf("expected clipboard command for journal copy")
	}
	if v.statusFlash == "" {
		t.Fatalf("expected status flash after journal copy")
	}
}

func TestViewImplementsInterfaceCompileTime(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	var _ View = New(Options{TermPane: pane})
	_ = context.Background()
}

func TestMainMenuEnterActivatesSelection(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenMainMenu)

	press(v, tea.KeyEnter, 0, "")

	deadline := time.Now().Add(300 * time.Millisecond)
	for ctrl.ContinueCalls() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ctrl.ContinueCalls() != 1 {
		t.Fatalf("expected continue action on Enter, got %d", ctrl.ContinueCalls())
	}
}

func TestCtrlQQuitsFromAnyScreen(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	press(v, 'q', tea.ModCtrl, "")

	deadline := time.Now().Add(300 * time.Millisecond)
	for ctrl.QuitCalls() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ctrl.QuitCalls() != 1 {
		t.Fatalf("expected Ctrl+Q to trigger quit")
	}
}

func TestMainMenuMouseClickActivatesSelection(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane, MouseScope: "scoped"})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenMainMenu)

	_, _ = v.Update(tea.MouseClickMsg{X: 2, Y: 2, Button: tea.MouseLeft})

	deadline := time.Now().Add(300 * time.Millisecond)
	for ctrl.ContinueCalls() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ctrl.ContinueCalls() != 1 {
		t.Fatalf("expected mouse click to activate first main menu item")
	}
}

func TestScopedMouseIgnoredWhilePlayingWithoutOverlay(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane, MouseScope: "scoped"})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	_, _ = v.Update(tea.MouseClickMsg{X: 40, Y: 12, Button: tea.MouseLeft})
	time.Sleep(50 * time.Millisecond)

	if ctrl.ContinueCalls() != 0 || ctrl.QuitCalls() != 0 || ctrl.ResetCalls() != 0 {
		t.Fatalf("unexpected controller calls from scoped playing mouse click")
	}
}

func TestRandomEventSequenceNoPanic(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)
	v.SetPlayingState(PlayingState{
		ModeLabel: "Free Play",
		PackID:    "builtin-core",
		LevelID:   "level-001",
		StartedAt: time.Now(),
		HudWidth:  42,
	})

	r := rand.New(rand.NewSource(42))
	msgs := []tea.Msg{
		tea.WindowSizeMsg{Width: 120, Height: 36},
		tea.WindowSizeMsg{Width: 95, Height: 28},
		tea.WindowSizeMsg{Width: 88, Height: 23},
		tea.KeyPressMsg{Code: tea.KeyF1},
		tea.KeyPressMsg{Code: tea.KeyF2},
		tea.KeyPressMsg{Code: tea.KeyF4},
		tea.KeyPressMsg{Code: tea.KeyF5},
		tea.KeyPressMsg{Code: tea.KeyF6},
		tea.KeyPressMsg{Code: tea.KeyF10},
		tea.KeyPressMsg{Code: tea.KeyEsc},
		tea.KeyPressMsg{Code: tea.KeyPgUp, Mod: tea.ModShift},
		tea.KeyPressMsg{Code: 'a', Text: "a"},
		tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl},
		tea.PasteMsg{Content: "echo fuzz\n"},
		tea.MouseClickMsg{X: 10, Y: 10, Button: tea.MouseLeft},
		tea.MouseWheelMsg{X: 10, Y: 10, Button: tea.MouseWheelDown},
	}

	for i := 0; i < 1000; i++ {
		msg := msgs[r.Intn(len(msgs))]
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("update panicked on iteration %d with msg %#v: %v", i, msg, rec)
				}
			}()
			_, _ = v.Update(msg)
			_ = v.View()
		}()
	}
}

func TestUpdateRecoversFromPanic(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})

	_, _ = v.Update(applyMsg{fn: func(*Root) {
		panic("forced update panic")
	}})

	if !strings.Contains(v.statusFlash, "Recovered UI panic") {
		t.Fatalf("expected panic recovery status flash, got %q", v.statusFlash)
	}
}

func TestViewRecoversFromSnapshotPanic(t *testing.T) {
	pane := panicSnapshotPane{TerminalPane: term.NewTerminalPane(nil)}
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetPlayingState(PlayingState{
		ModeLabel: "Free Play",
		PackID:    "builtin-core",
		LevelID:   "level-001",
		StartedAt: time.Now(),
		HudWidth:  42,
	})

	_ = v.View()

	if !strings.Contains(v.statusFlash, "Recovered UI panic") {
		t.Fatalf("expected panic recovery status flash, got %q", v.statusFlash)
	}
}

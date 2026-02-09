package ui

import (
	"context"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"clidojo/internal/term"
	tea "github.com/charmbracelet/bubbletea/v2"
)

type mockController struct {
	mu            sync.Mutex
	continueCalls int
	dailyCalls    int
	campaignCalls int
	practiceCalls int
	quitCalls     int
	resetCalls    int
	menuCalls     int
	goalCalls     int
	hintsCalls    int
	journalCalls  int
	statsCalls    int
	inputs        [][]byte
	settings      []SettingsState
}

func (m *mockController) OnContinue() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.continueCalls++
}
func (m *mockController) OnStartDailyDrill() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dailyCalls++
}
func (m *mockController) OnStartCampaign() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.campaignCalls++
}
func (m *mockController) OnStartPractice() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.practiceCalls++
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
func (m *mockController) OnHints() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hintsCalls++
}
func (m *mockController) OnGoal() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.goalCalls++
}
func (m *mockController) OnJournal() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.journalCalls++
}
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
func (m *mockController) OnChangeLevel()  {}
func (m *mockController) OnOpenSettings() {}
func (m *mockController) OnOpenStats() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statsCalls++
}
func (m *mockController) OnRevealHint()             {}
func (m *mockController) OnNextLevel()              {}
func (m *mockController) OnTryAgain()               {}
func (m *mockController) OnShowReferenceSolutions() {}
func (m *mockController) OnOpenDiff()               {}
func (m *mockController) OnJournalExplainAI()       {}
func (m *mockController) OnApplySettings(s SettingsState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = append(m.settings, s)
}

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

func (m *mockController) GoalCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.goalCalls
}

func (m *mockController) HintsCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hintsCalls
}

func (m *mockController) StatsCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statsCalls
}

func (m *mockController) JournalCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.journalCalls
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

func (m *mockController) SettingsUpdates() []SettingsState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SettingsState, len(m.settings))
	copy(out, m.settings)
	return out
}

type spyPane struct {
	*term.TerminalPane
	mu        sync.Mutex
	inputs    [][]byte
	bracketed bool
}

func newSpyPane() *spyPane {
	return &spyPane{TerminalPane: term.NewTerminalPane(nil)}
}

func (p *spyPane) SendInput(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := append([]byte(nil), data...)
	p.inputs = append(p.inputs, cp)
	return nil
}

func (p *spyPane) Inputs() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]byte, len(p.inputs))
	for i := range p.inputs {
		out[i] = append([]byte(nil), p.inputs[i]...)
	}
	return out
}

func (p *spyPane) BracketedPasteEnabled() bool {
	if p.bracketed {
		return true
	}
	if p.TerminalPane == nil {
		return false
	}
	return p.TerminalPane.BracketedPasteEnabled()
}

type panicSnapshotPane struct {
	*term.TerminalPane
}

func (p panicSnapshotPane) Snapshot(width, height int) term.Snapshot {
	panic("snapshot boom")
}

type fixedSnapshotPane struct {
	*term.TerminalPane
	snap term.Snapshot
}

func (p fixedSnapshotPane) Snapshot(width, height int) term.Snapshot {
	out := p.snap
	if len(out.Lines) == 0 {
		out.Lines = make([]string, height)
	}
	if len(out.StyledLines) == 0 {
		out.StyledLines = append([]string(nil), out.Lines...)
	}
	return out
}

func press(v *Root, code rune, mod tea.KeyMod, text string) {
	_, _ = v.Update(tea.KeyPressMsg{Code: code, Mod: mod, Text: text})
}

func TestNormalizeKeyPressMsgEscFragmentArrow(t *testing.T) {
	msg := normalizeKeyPressMsg(tea.KeyPressMsg{Text: "[B"})
	if msg.Code != tea.KeyDown {
		t.Fatalf("expected [B to normalize to KeyDown, got %v", msg.Code)
	}
}

func TestNormalizeKeyPressMsgEscPrefixedFragmentArrow(t *testing.T) {
	msg, escFragment := normalizeKeyPressMsgWithMeta(tea.KeyPressMsg{Text: "\x1b[B"})
	if !escFragment {
		t.Fatalf("expected esc-prefixed fragment to be marked as fragment")
	}
	if msg.Code != tea.KeyDown {
		t.Fatalf("expected \\x1b[B to normalize to KeyDown, got %v", msg.Code)
	}
}

func TestNormalizeKeyPressMsgControlText(t *testing.T) {
	enter := normalizeKeyPressMsg(tea.KeyPressMsg{Text: "\r"})
	if enter.Code != tea.KeyEnter {
		t.Fatalf("expected carriage return text to normalize to KeyEnter, got %v", enter.Code)
	}
	esc := normalizeKeyPressMsg(tea.KeyPressMsg{Text: "\x1b"})
	if esc.Code != tea.KeyEsc {
		t.Fatalf("expected escape text to normalize to KeyEsc, got %v", esc.Code)
	}
}

func TestResetOverlayHandlesTextEnter(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetResetConfirmOpen(true)
	v.resetIndex = 0

	_, _ = v.Update(tea.KeyPressMsg{Text: "\r"})
	if v.resetOpen {
		t.Fatalf("expected reset overlay to close on text-enter normalization")
	}
}

func TestReferenceOverlayHandlesTextEscape(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetReferenceText("ref", true)

	_, _ = v.Update(tea.KeyPressMsg{Text: "\x1b"})
	if v.referenceOpen {
		t.Fatalf("expected reference overlay to close on text-escape normalization")
	}
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

func TestEscClosesGoalDrawer(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)
	v.SetGoalOpen(true)

	press(v, tea.KeyEsc, 0, "")

	deadline := time.Now().Add(300 * time.Millisecond)
	for ctrl.GoalCalls() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ctrl.GoalCalls() == 0 {
		t.Fatalf("expected Esc to dispatch goal toggle for drawer close")
	}
}

func TestEscFromHintsClosesMediumDrawer(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)
	v.layout = LayoutMedium
	v.goalOpen = true
	v.hintsOpen = true

	press(v, tea.KeyEsc, 0, "")

	if v.hintsOpen {
		t.Fatalf("expected hints overlay to close on Esc")
	}
	if v.goalOpen {
		t.Fatalf("expected medium HUD drawer to close on Esc when dismissing hints")
	}
	deadline := time.Now().Add(300 * time.Millisecond)
	for (ctrl.HintsCalls() == 0 || ctrl.GoalCalls() == 0) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ctrl.HintsCalls() == 0 {
		t.Fatalf("expected Esc to dispatch hints toggle")
	}
	if ctrl.GoalCalls() == 0 {
		t.Fatalf("expected Esc to dispatch goal toggle for drawer close")
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
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)

	press(v, tea.KeyEsc, 0, "")

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(pane.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	inputs := pane.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "\x1b" {
		t.Fatalf("expected escape to be forwarded to terminal")
	}
}

func TestEscFragmentCoalescesWithoutDuplicateEsc(t *testing.T) {
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.running = true
	var prog tea.Program
	v.program = &prog

	_, _ = v.Update(tea.KeyPressMsg{Text: "\x1b"})
	_, _ = v.Update(tea.KeyPressMsg{Text: "[B"})

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(pane.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	// Allow esc flush timer to fire if it was queued incorrectly.
	time.Sleep(50 * time.Millisecond)

	inputs := pane.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "\x1b[B" {
		t.Fatalf("expected coalesced down-arrow bytes only, got %#v", inputs)
	}
}

func TestEscSplitPrefixThenFinalByteForwardsAsCSI(t *testing.T) {
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.running = true
	var prog tea.Program
	v.program = &prog

	// Simulate browser/websocket splitting arrow-down as ESC, then "[" then "B".
	_, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	_, _ = v.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	_, _ = v.Update(tea.KeyPressMsg{Code: 'B', Text: "B"})

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(pane.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	// Allow esc flush timer to fire if it was queued incorrectly.
	time.Sleep(50 * time.Millisecond)

	inputs := pane.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "\x1b[B" {
		t.Fatalf("expected coalesced CSI down-arrow bytes, got %#v", inputs)
	}
}

func TestEscSplitPrefixThenKeyCodeForwardsAsCSI(t *testing.T) {
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.running = true
	var prog tea.Program
	v.program = &prog

	// Simulate browser/websocket splitting arrow-down as ESC, then "[" then
	// a terminal key code event instead of literal "B" text.
	_, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	_, _ = v.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	_, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(pane.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	// Allow esc/csi flush timers to fire if they were queued incorrectly.
	time.Sleep(50 * time.Millisecond)

	inputs := pane.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "\x1b[B" {
		t.Fatalf("expected coalesced key-code CSI down-arrow bytes, got %#v", inputs)
	}
}

func TestTabPassesThroughToTerminal(t *testing.T) {
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)

	press(v, tea.KeyTab, 0, "")

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(pane.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	inputs := pane.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "\t" {
		t.Fatalf("expected tab to be forwarded to terminal")
	}
}

func TestCtrlRPassesThroughToTerminal(t *testing.T) {
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)

	press(v, 'r', tea.ModCtrl, "")

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(pane.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	inputs := pane.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "\x12" {
		t.Fatalf("expected ctrl+r to be forwarded to terminal reverse-search")
	}
	if v.resetOpen {
		t.Fatalf("expected ctrl+r to avoid opening reset confirm")
	}
}

func TestPasteMsgPassesThroughToTerminal(t *testing.T) {
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)

	_, _ = v.Update(tea.PasteMsg("echo hi\npwd\n"))

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(pane.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	inputs := pane.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "echo hi\npwd\n" {
		t.Fatalf("expected pasted content to be forwarded unchanged")
	}
}

func TestPasteMsgIgnoredWhenOverlayOpen(t *testing.T) {
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetMenuOpen(true)

	_, _ = v.Update(tea.PasteMsg("echo should_not_send\n"))
	time.Sleep(50 * time.Millisecond)

	if len(pane.Inputs()) != 0 {
		t.Fatalf("expected paste to be ignored while overlay is open")
	}
}

func TestPasteMsgUsesBracketedPasteWhenEnabled(t *testing.T) {
	pane := newSpyPane()
	pane.bracketed = true
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)

	_, _ = v.Update(tea.PasteMsg("echo hi\npwd\n"))

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(pane.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	inputs := pane.Inputs()
	if len(inputs) != 1 {
		t.Fatalf("expected bracketed paste content to be forwarded")
	}
	if string(inputs[0]) != "\x1b[200~echo hi\npwd\n\x1b[201~" {
		t.Fatalf("unexpected bracketed paste payload: %q", string(inputs[0]))
	}
}

func TestCtrlVRequestsClipboardPaste(t *testing.T) {
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl, Text: "v"})
	if cmd == nil {
		t.Fatalf("expected ctrl+v to request clipboard read")
	}
	if len(pane.Inputs()) != 0 {
		t.Fatalf("expected ctrl+v to avoid direct terminal write before clipboard read")
	}
}

func TestClipboardMsgPassesThroughToTerminal(t *testing.T) {
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)

	_, _ = v.Update(tea.ClipboardMsg("echo from clipboard\n"))

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(pane.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	inputs := pane.Inputs()
	if len(inputs) != 1 || string(inputs[0]) != "echo from clipboard\n" {
		t.Fatalf("expected clipboard content to be forwarded unchanged")
	}
}

func TestOverlayCopyShortcutSetsStatusFlash(t *testing.T) {
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

func TestPassResultRendersConfettiWhenMotionEnabled(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane, MotionLevel: "full"})
	v.SetScreen(ScreenPlaying)
	v.SetPlayingState(PlayingState{
		ModeLabel: "Free Play",
		PackID:    "builtin-core",
		LevelID:   "level-001-pipes-101",
		StartedAt: time.Now(),
		HudWidth:  42,
	})
	v.SetResult(ResultState{
		Visible: true,
		Passed:  true,
		Summary: "pass",
		Checks:  []CheckResultRow{{ID: "c1", Passed: true, Message: "ok"}},
		Score:   1000,
	})

	out := v.View()
	if !strings.Contains(out, "✦") && !strings.Contains(out, "✶") && !strings.Contains(out, "✷") && !strings.Contains(out, "❖") {
		t.Fatalf("expected pass result view to render confetti particles")
	}
}

func TestPassResultSkipsConfettiWhenMotionOff(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane, MotionLevel: "off"})
	v.SetScreen(ScreenPlaying)
	v.SetPlayingState(PlayingState{
		ModeLabel: "Free Play",
		PackID:    "builtin-core",
		LevelID:   "level-001-pipes-101",
		StartedAt: time.Now(),
		HudWidth:  42,
	})
	v.SetResult(ResultState{
		Visible: true,
		Passed:  true,
		Summary: "pass",
		Checks:  []CheckResultRow{{ID: "c1", Passed: true, Message: "ok"}},
		Score:   1000,
	})

	out := v.View()
	if strings.Contains(out, "✦") || strings.Contains(out, "✶") || strings.Contains(out, "✷") {
		t.Fatalf("expected no confetti particles when motion is off")
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

func TestSpinnerTickIgnoredWhenNotChecking(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)

	_, cmd := v.Update(spinnerTickCmd(v.checkSpin)())
	if cmd != nil {
		t.Fatalf("expected no spinner reschedule when checking is false")
	}
}

func TestSpinnerStartMsgSchedulesTickWhenChecking(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetChecking(true)

	_, cmd := v.Update(spinnerStartMsg{})
	if cmd == nil {
		t.Fatalf("expected spinner start message to schedule a tick while checking")
	}
}

func TestRenderPlayingClearsStaleTooSmallFlagWhenSizeIsValid(t *testing.T) {
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
	v.cols = 80
	v.rows = 24
	v.SetTooSmall(80, 24)

	out := v.renderPlaying()
	if v.forceTooSmall || v.layout == LayoutTooSmall {
		t.Fatalf("expected stale too-small mode to clear at 80x24; force=%v layout=%v", v.forceTooSmall, v.layout)
	}
	if strings.Contains(out, "Resize Required") {
		t.Fatalf("expected stale too-small panel to disappear at 80x24")
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

func TestCtrlQPassesThroughToTerminal(t *testing.T) {
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	press(v, 'q', tea.ModCtrl, "")

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(pane.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ctrl.QuitCalls() != 0 {
		t.Fatalf("expected Ctrl+Q not to trigger quit")
	}
	inputs := pane.Inputs()
	if len(inputs) == 0 || string(inputs[0]) != "\x11" {
		t.Fatalf("expected Ctrl+Q to pass through as terminal input, got %#v", inputs)
	}
}

func TestCtrlShortcutsEnabledInDevMode(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane, DevMode: true})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	press(v, 'h', tea.ModCtrl, "")
	press(v, 'g', tea.ModCtrl, "")
	press(v, 'j', tea.ModCtrl, "")
	press(v, 'm', tea.ModCtrl, "")
	press(v, 'y', tea.ModCtrl, "")
	press(v, 'r', tea.ModCtrl, "")

	deadline := time.Now().Add(300 * time.Millisecond)
	for (ctrl.HintsCalls() == 0 || ctrl.GoalCalls() == 0 || ctrl.JournalCalls() == 0 || ctrl.MenuCalls() == 0) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ctrl.HintsCalls() != 1 {
		t.Fatalf("expected Ctrl+H to open hints in dev mode")
	}
	if ctrl.GoalCalls() != 1 {
		t.Fatalf("expected Ctrl+G to toggle goal in dev mode")
	}
	if ctrl.JournalCalls() != 1 {
		t.Fatalf("expected Ctrl+J to toggle journal in dev mode")
	}
	if ctrl.MenuCalls() != 1 {
		t.Fatalf("expected Ctrl+M to open menu in dev mode")
	}
	if !pane.InScrollback() {
		t.Fatalf("expected Ctrl+Y to toggle scrollback in dev mode")
	}
	if !v.resetOpen {
		t.Fatalf("expected Ctrl+R to open reset confirmation in dev mode")
	}
}

func TestCtrlShortcutsDoNotHijackNormalMode(t *testing.T) {
	pane := newSpyPane()
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)

	press(v, 'r', tea.ModCtrl, "")

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(pane.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if v.resetOpen {
		t.Fatalf("expected Ctrl+R not to open reset modal outside dev mode")
	}
	inputs := pane.Inputs()
	if len(inputs) == 0 || string(inputs[0]) != "\x12" {
		t.Fatalf("expected Ctrl+R to pass through as terminal input, got %#v", inputs)
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
		tea.PasteMsg("echo fuzz\n"),
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

func TestLevelSelectSearchFiltersLevels(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenLevelSelect)
	v.SetCatalog([]PackSummary{
		{
			PackID: "builtin-core",
			Name:   "Core",
			Levels: []LevelSummary{
				{LevelID: "level-001-pipes-101", Title: "Pipes 101", Difficulty: 1},
				{LevelID: "level-002-find-safe", Title: "Find Safe", Difficulty: 2, ToolFocus: []string{"find"}},
			},
		},
	})

	for _, ch := range "find" {
		press(v, ch, 0, string(ch))
	}
	levels := v.selectedPackLevels()
	if len(levels) != 1 || levels[0].LevelID != "level-002-find-safe" {
		t.Fatalf("expected search to keep only find-safe level, got %#v", levels)
	}
}

func TestLevelSelectDifficultyFilterCycles(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenLevelSelect)
	v.SetCatalog([]PackSummary{
		{
			PackID: "builtin-core",
			Name:   "Core",
			Levels: []LevelSummary{
				{LevelID: "easy", Title: "Easy", Difficulty: 1},
				{LevelID: "mid", Title: "Mid", Difficulty: 3},
				{LevelID: "hard", Title: "Hard", Difficulty: 5},
			},
		},
	})

	if got := len(v.selectedPackLevels()); got != 3 {
		t.Fatalf("expected all levels visible, got %d", got)
	}
	press(v, 'f', tea.ModAlt, "f")
	if got := len(v.selectedPackLevels()); got != 1 || v.selectedPackLevels()[0].LevelID != "easy" {
		t.Fatalf("expected easy band to include only easy, got %#v", v.selectedPackLevels())
	}
	press(v, 'f', tea.ModAlt, "f")
	if got := len(v.selectedPackLevels()); got != 1 || v.selectedPackLevels()[0].LevelID != "mid" {
		t.Fatalf("expected mid band to include only mid, got %#v", v.selectedPackLevels())
	}
	press(v, 'f', tea.ModAlt, "f")
	if got := len(v.selectedPackLevels()); got != 1 || v.selectedPackLevels()[0].LevelID != "hard" {
		t.Fatalf("expected hard band to include only hard, got %#v", v.selectedPackLevels())
	}
	press(v, 'f', tea.ModAlt, "f")
	if got := len(v.selectedPackLevels()); got != 3 {
		t.Fatalf("expected wrap back to all levels, got %d", got)
	}
}

func TestRenderTerminalPanelRendersInlineCursor(t *testing.T) {
	pane := fixedSnapshotPane{
		TerminalPane: term.NewTerminalPane(nil),
		snap: term.Snapshot{
			Lines:       []string{"player@dojo:/work$ "},
			StyledLines: []string{"player@dojo:/work$ "},
			CursorX:     5,
			CursorY:     0,
			CursorShow:  true,
		},
	}
	v := New(Options{TermPane: pane})
	panel := v.renderTerminalPanel(50, 6, 0, 0)
	if !strings.Contains(panel, "\x1b[7m") {
		t.Fatalf("expected reverse-video inline cursor marker in panel output")
	}
}

func TestRenderTermFrameRowsCursorVisibleOnDefaultCell(t *testing.T) {
	frame := term.Frame{
		W:          8,
		H:          1,
		CursorX:    3,
		CursorY:    0,
		CursorShow: true,
		Cells:      make([]term.FrameCell, 8),
	}
	for i := range frame.Cells {
		frame.Cells[i] = term.FrameCell{
			Ch: ' ',
			Style: term.CellStyle{
				FGDefault: true,
				BGDefault: true,
			},
		}
	}
	rows := renderTermFrameRows(frame, 8, 1, false)
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if !strings.Contains(rows[0], "30;47m") {
		t.Fatalf("expected fallback cursor style for default cell, got %q", rows[0])
	}
}

func TestComposeOverlayPreservesANSIStyles(t *testing.T) {
	base := "\x1b[31mHELLO\x1b[0m     \nplain line          "
	overlay := "\x1b[34mOK\x1b[0m"
	out := composeOverlayAt(base, overlay, 20, 2, 0, 6)
	if !strings.Contains(out, "\x1b[31m") {
		t.Fatalf("expected base ANSI style to be preserved, got %q", out)
	}
	if !strings.Contains(out, "\x1b[34m") {
		t.Fatalf("expected overlay ANSI style to be preserved, got %q", out)
	}
}

func TestSettingsOverlayAppliesUpdate(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)
	v.SetSettings(SettingsState{
		AutoCheckMode:       "off",
		AutoCheckDebounceMS: 800,
		StyleVariant:        "modern_arcade",
		MotionLevel:         "full",
		MouseScope:          "scoped",
	}, true)

	// Toggle auto-check mode once (off -> manual), then jump to Apply.
	press(v, tea.KeyRight, 0, "")
	for i := 0; i < 5; i++ {
		press(v, tea.KeyDown, 0, "")
	}
	press(v, tea.KeyEnter, 0, "")
	waitForCondition(t, 300*time.Millisecond, func() bool {
		return len(ctrl.SettingsUpdates()) == 1
	})
	updates := ctrl.SettingsUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected exactly one settings update dispatch, got %d", len(updates))
	}
	if updates[0].AutoCheckMode != "manual" {
		t.Fatalf("expected auto-check mode to be updated to manual, got %q", updates[0].AutoCheckMode)
	}
	if v.settingsOpen {
		t.Fatalf("expected settings overlay to close after apply")
	}
}

func TestStatsInfoOverlayRefreshHotkey(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)
	v.SetInfo("Stats", "demo", true)

	press(v, 'r', 0, "r")
	waitForCondition(t, 300*time.Millisecond, func() bool {
		return ctrl.StatsCalls() == 1
	})
	if got := ctrl.StatsCalls(); got != 1 {
		t.Fatalf("expected stats refresh to dispatch once, got %d", got)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

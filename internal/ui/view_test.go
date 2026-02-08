package ui

import (
	"context"
	"testing"

	"clidojo/internal/term"

	"github.com/gdamore/tcell/v2"
)

type mockController struct {
	continueCalls int
	quitCalls     int
	resetCalls    int
	inputs        [][]byte
}

func (m *mockController) OnContinue()                 { m.continueCalls++ }
func (m *mockController) OnOpenLevelSelect()          {}
func (m *mockController) OnStartLevel(string, string) {}
func (m *mockController) OnBackToMainMenu()           {}
func (m *mockController) OnOpenMainMenu()             {}
func (m *mockController) OnCheck()                    {}
func (m *mockController) OnReset()                    { m.resetCalls++ }
func (m *mockController) OnMenu()                     {}
func (m *mockController) OnHints()                    {}
func (m *mockController) OnGoal()                     {}
func (m *mockController) OnJournal()                  {}
func (m *mockController) OnQuit()                     { m.quitCalls++ }
func (m *mockController) OnResize(int, int)           {}
func (m *mockController) OnTerminalInput(data []byte) {
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

func TestF6OpensResetConfirmWithoutImmediateReset(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	v.captureInput(tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModNone))

	if ctrl.resetCalls != 0 {
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

	v.captureInput(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))
	if v.result.Visible {
		t.Fatalf("expected result modal to close on escape")
	}
}

func TestOverlayAllowsModalKeyHandling(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetScreen(ScreenPlaying)
	v.SetMenuOpen(true)

	ev := v.captureInput(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if ev == nil {
		t.Fatalf("expected non-escape keys to reach the active modal")
	}
}

func TestEscPassesThroughToTerminal(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	v.captureInput(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))

	if len(ctrl.inputs) != 1 || string(ctrl.inputs[0]) != "\x1b" {
		t.Fatalf("expected escape to be forwarded to terminal")
	}
}

func TestTabPassesThroughToTerminal(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	v.captureInput(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))

	if len(ctrl.inputs) != 1 || string(ctrl.inputs[0]) != "\t" {
		t.Fatalf("expected tab to be forwarded to terminal")
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

	v.captureInput(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	if ctrl.continueCalls != 1 {
		t.Fatalf("expected continue action on Enter, got %d", ctrl.continueCalls)
	}
}

func TestCtrlQQuitsFromAnyScreen(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	ctrl := &mockController{}
	v.SetController(ctrl)
	v.SetScreen(ScreenPlaying)

	v.captureInput(tcell.NewEventKey(tcell.KeyCtrlQ, 0, tcell.ModCtrl))

	if ctrl.quitCalls != 1 {
		t.Fatalf("expected Ctrl+Q to trigger quit")
	}
}

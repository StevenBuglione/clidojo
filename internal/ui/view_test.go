package ui

import (
	"context"
	"testing"

	"clidojo/internal/term"

	"github.com/gdamore/tcell/v2"
)

type mockController struct{ resetCalls int }

func (m *mockController) OnCheck()                  {}
func (m *mockController) OnReset()                  { m.resetCalls++ }
func (m *mockController) OnMenu()                   {}
func (m *mockController) OnHints()                  {}
func (m *mockController) OnGoal()                   {}
func (m *mockController) OnJournal()                {}
func (m *mockController) OnQuit()                   {}
func (m *mockController) OnResize(int, int)         {}
func (m *mockController) OnTerminalInput([]byte)    {}
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
	v.SetResult(ResultState{Visible: true, Passed: false, Summary: "x", PrimaryAction: "Try again"})

	v.captureInput(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))
	if v.result.Visible {
		t.Fatalf("expected result modal to close on escape")
	}
}

func TestOverlayAllowsModalKeyHandling(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	v := New(Options{TermPane: pane})
	v.SetMenuOpen(true)

	ev := v.captureInput(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if ev == nil {
		t.Fatalf("expected non-escape keys to reach the active modal")
	}
}

func TestViewImplementsInterfaceCompileTime(t *testing.T) {
	pane := term.NewTerminalPane(nil)
	var _ View = New(Options{TermPane: pane})
	_ = context.Background()
}

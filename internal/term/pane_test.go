package term

import (
	"io"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
)

func TestDrawWithoutSession(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatalf("init sim screen: %v", err)
	}
	defer s.Fini()
	s.SetSize(80, 24)

	p := NewTerminalPane(nil)
	p.SetRect(0, 0, 80, 24)
	p.Draw(s)

	line := readLine(s, 1, 1, 40)
	if !strings.Contains(line, "No terminal session") {
		t.Fatalf("expected placeholder text, got %q", line)
	}
}

func TestDrawClampsToVTSize(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatalf("init sim screen: %v", err)
	}
	defer s.Fini()
	s.SetSize(120, 40)

	p := NewTerminalPane(nil)
	p.SetRect(0, 0, 120, 40)
	p.vt = vt10x.New(vt10x.WithWriter(io.Discard), vt10x.WithSize(80, 24))

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("draw panicked with oversized pane: %v", r)
		}
	}()
	p.Draw(s)
}

func TestSnapshotDoesNotPanicWhenVTBoundsShift(t *testing.T) {
	p := NewTerminalPane(nil)
	p.vt = vt10x.New(vt10x.WithWriter(io.Discard), vt10x.WithSize(80, 24))
	p.rows = 40
	p.cols = 120

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("snapshot panicked: %v", r)
		}
	}()
	snap := p.Snapshot(120, 40)
	if len(snap.Lines) != 40 {
		t.Fatalf("expected 40 snapshot lines, got %d", len(snap.Lines))
	}
}

func readLine(s tcell.SimulationScreen, x, y, w int) string {
	var b strings.Builder
	for i := 0; i < w; i++ {
		r, _, _, _ := s.GetContent(x+i, y)
		if r == 0 {
			r = ' '
		}
		b.WriteRune(r)
	}
	return b.String()
}

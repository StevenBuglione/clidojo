package term

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gdamore/tcell/v2"
)

func TestEncodeEventToBytes(t *testing.T) {
	tests := []struct {
		name string
		ev   *tcell.EventKey
		want string
	}{
		{name: "tab", ev: tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), want: "\t"},
		{name: "shift tab", ev: tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone), want: "\x1b[Z"},
		{name: "alt rune", ev: tcell.NewEventKey(tcell.KeyRune, 'b', tcell.ModAlt), want: "\x1bb"},
		{name: "ctrl left", ev: tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModCtrl), want: "\x1b[1;5D"},
		{name: "alt right", ev: tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModAlt), want: "\x1b[1;3C"},
		{name: "shift up", ev: tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModShift), want: "\x1b[1;2A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeEventToBytes(tt.ev)
			if string(got) != tt.want {
				t.Fatalf("got %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestEncodeKeyPressToBytes(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		want string
	}{
		{name: "tab", key: tea.KeyPressMsg{Code: tea.KeyTab}, want: "\t"},
		{name: "shift tab", key: tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, want: "\x1b[Z"},
		{name: "alt rune", key: tea.KeyPressMsg{Code: 'b', Text: "b", Mod: tea.ModAlt}, want: "\x1bb"},
		{name: "ctrl left", key: tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl}, want: "\x1b[1;5D"},
		{name: "alt right", key: tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt}, want: "\x1b[1;3C"},
		{name: "shift up", key: tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift}, want: "\x1b[1;2A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeKeyPressToBytes(tt.key)
			if string(got) != tt.want {
				t.Fatalf("got %q, want %q", string(got), tt.want)
			}
		})
	}
}

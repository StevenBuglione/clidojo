package term

import (
	"testing"

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

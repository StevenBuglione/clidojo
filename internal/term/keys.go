package term

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
)

// EncodeEventToBytes converts key events to terminal byte sequences.
func EncodeEventToBytes(ev *tcell.EventKey) []byte {
	if ev == nil {
		return nil
	}

	mods := ev.Modifiers()

	switch ev.Key() {
	case tcell.KeyRune:
		out := []byte(string(ev.Rune()))
		if mods&tcell.ModAlt != 0 {
			return append([]byte{0x1b}, out...)
		}
		return out
	case tcell.KeyEnter:
		if mods&tcell.ModAlt != 0 {
			return []byte("\x1b\r")
		}
		return []byte("\r")
	case tcell.KeyTab:
		if mods&tcell.ModAlt != 0 {
			return []byte("\x1b\t")
		}
		return []byte("\t")
	case tcell.KeyBacktab:
		return []byte("\x1b[Z")
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if mods&tcell.ModAlt != 0 {
			return []byte{0x1b, 0x7f}
		}
		return []byte{0x7f}
	case tcell.KeyEsc:
		return []byte{0x1b}
	case tcell.KeyUp:
		return csiWithModifier("A", mods)
	case tcell.KeyDown:
		return csiWithModifier("B", mods)
	case tcell.KeyRight:
		return csiWithModifier("C", mods)
	case tcell.KeyLeft:
		return csiWithModifier("D", mods)
	case tcell.KeyHome:
		return csiWithModifier("H", mods)
	case tcell.KeyEnd:
		return csiWithModifier("F", mods)
	case tcell.KeyPgUp:
		return tildeWithModifier(5, mods)
	case tcell.KeyPgDn:
		return tildeWithModifier(6, mods)
	case tcell.KeyDelete:
		return tildeWithModifier(3, mods)
	case tcell.KeyInsert:
		return tildeWithModifier(2, mods)
	}

	if ctrl := ctrlCode(ev.Key()); ctrl != 0 {
		if mods&tcell.ModAlt != 0 {
			return []byte{0x1b, ctrl}
		}
		return []byte{ctrl}
	}

	if f := functionKey(ev.Key()); f != "" {
		return []byte(f)
	}

	return nil
}

func csiWithModifier(final string, mods tcell.ModMask) []byte {
	mod := xtermModifier(mods)
	if mod == 1 {
		return []byte("\x1b[" + final)
	}
	return []byte(fmt.Sprintf("\x1b[1;%d%s", mod, final))
}

func tildeWithModifier(n int, mods tcell.ModMask) []byte {
	mod := xtermModifier(mods)
	if mod == 1 {
		return []byte(fmt.Sprintf("\x1b[%d~", n))
	}
	return []byte(fmt.Sprintf("\x1b[%d;%d~", n, mod))
}

func xtermModifier(mods tcell.ModMask) int {
	mod := 1
	if mods&tcell.ModShift != 0 {
		mod += 1
	}
	if mods&tcell.ModAlt != 0 {
		mod += 2
	}
	if mods&tcell.ModCtrl != 0 {
		mod += 4
	}
	return mod
}

func ctrlCode(k tcell.Key) byte {
	switch k {
	case tcell.KeyCtrlA:
		return 0x01
	case tcell.KeyCtrlB:
		return 0x02
	case tcell.KeyCtrlC:
		return 0x03
	case tcell.KeyCtrlD:
		return 0x04
	case tcell.KeyCtrlE:
		return 0x05
	case tcell.KeyCtrlF:
		return 0x06
	case tcell.KeyCtrlG:
		return 0x07
	case tcell.KeyCtrlH:
		return 0x08
	case tcell.KeyCtrlI:
		return 0x09
	case tcell.KeyCtrlJ:
		return 0x0a
	case tcell.KeyCtrlK:
		return 0x0b
	case tcell.KeyCtrlL:
		return 0x0c
	case tcell.KeyCtrlM:
		return 0x0d
	case tcell.KeyCtrlN:
		return 0x0e
	case tcell.KeyCtrlO:
		return 0x0f
	case tcell.KeyCtrlP:
		return 0x10
	case tcell.KeyCtrlQ:
		return 0x11
	case tcell.KeyCtrlR:
		return 0x12
	case tcell.KeyCtrlS:
		return 0x13
	case tcell.KeyCtrlT:
		return 0x14
	case tcell.KeyCtrlU:
		return 0x15
	case tcell.KeyCtrlV:
		return 0x16
	case tcell.KeyCtrlW:
		return 0x17
	case tcell.KeyCtrlX:
		return 0x18
	case tcell.KeyCtrlY:
		return 0x19
	case tcell.KeyCtrlZ:
		return 0x1a
	case tcell.KeyCtrlSpace:
		return 0x00
	case tcell.KeyCtrlBackslash:
		return 0x1c
	case tcell.KeyCtrlRightSq:
		return 0x1d
	case tcell.KeyCtrlCarat:
		return 0x1e
	case tcell.KeyCtrlUnderscore:
		return 0x1f
	default:
		return 0
	}
}

func functionKey(k tcell.Key) string {
	switch k {
	case tcell.KeyF1:
		return "\x1bOP"
	case tcell.KeyF2:
		return "\x1bOQ"
	case tcell.KeyF3:
		return "\x1bOR"
	case tcell.KeyF4:
		return "\x1bOS"
	case tcell.KeyF5:
		return "\x1b[15~"
	case tcell.KeyF6:
		return "\x1b[17~"
	case tcell.KeyF7:
		return "\x1b[18~"
	case tcell.KeyF8:
		return "\x1b[19~"
	case tcell.KeyF9:
		return "\x1b[20~"
	case tcell.KeyF10:
		return "\x1b[21~"
	case tcell.KeyF11:
		return "\x1b[23~"
	case tcell.KeyF12:
		return "\x1b[24~"
	default:
		return ""
	}
}

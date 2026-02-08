package term

import (
	"fmt"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// EncodeKeyPressToBytes converts Bubble Tea key events to terminal byte
// sequences using xterm-compatible conventions.
func EncodeKeyPressToBytes(ev tea.KeyPressMsg) []byte {
	key := ev.Key()

	// Printable characters.
	if key.Text != "" {
		out := []byte(key.Text)
		if key.Mod&tea.ModAlt != 0 {
			return append([]byte{0x1b}, out...)
		}
		return out
	}

	switch key.Code {
	case tea.KeyEnter:
		if key.Mod&tea.ModAlt != 0 {
			return []byte("\x1b\r")
		}
		return []byte("\r")
	case tea.KeyTab:
		if key.Mod&tea.ModShift != 0 {
			return []byte("\x1b[Z")
		}
		if key.Mod&tea.ModAlt != 0 {
			return []byte("\x1b\t")
		}
		return []byte("\t")
	case tea.KeyBackspace:
		if key.Mod&tea.ModAlt != 0 {
			return []byte{0x1b, 0x7f}
		}
		return []byte{0x7f}
	case tea.KeyEscape:
		return []byte{0x1b}
	case tea.KeyUp:
		return teaCSIWithModifier("A", key.Mod)
	case tea.KeyDown:
		return teaCSIWithModifier("B", key.Mod)
	case tea.KeyRight:
		return teaCSIWithModifier("C", key.Mod)
	case tea.KeyLeft:
		return teaCSIWithModifier("D", key.Mod)
	case tea.KeyHome:
		return teaCSIWithModifier("H", key.Mod)
	case tea.KeyEnd:
		return teaCSIWithModifier("F", key.Mod)
	case tea.KeyPgUp:
		return teaTildeWithModifier(5, key.Mod)
	case tea.KeyPgDown:
		return teaTildeWithModifier(6, key.Mod)
	case tea.KeyDelete:
		return teaTildeWithModifier(3, key.Mod)
	case tea.KeyInsert:
		return teaTildeWithModifier(2, key.Mod)
	}

	if key.Mod&tea.ModCtrl != 0 && key.Code != 0 && utf8.ValidRune(key.Code) {
		if c := ctrlRuneCode(key.Code); c != 0 {
			if key.Mod&tea.ModAlt != 0 {
				return []byte{0x1b, c}
			}
			return []byte{c}
		}
	}

	if f := teaFunctionKey(key.Code); f != "" {
		return []byte(f)
	}
	return nil
}

func teaCSIWithModifier(final string, mods tea.KeyMod) []byte {
	mod := teaXtermModifier(mods)
	if mod == 1 {
		return []byte("\x1b[" + final)
	}
	return []byte(fmt.Sprintf("\x1b[1;%d%s", mod, final))
}

func teaTildeWithModifier(n int, mods tea.KeyMod) []byte {
	mod := teaXtermModifier(mods)
	if mod == 1 {
		return []byte(fmt.Sprintf("\x1b[%d~", n))
	}
	return []byte(fmt.Sprintf("\x1b[%d;%d~", n, mod))
}

func teaXtermModifier(mods tea.KeyMod) int {
	mod := 1
	if mods&tea.ModShift != 0 {
		mod += 1
	}
	if mods&tea.ModAlt != 0 {
		mod += 2
	}
	if mods&tea.ModCtrl != 0 {
		mod += 4
	}
	return mod
}

func ctrlRuneCode(r rune) byte {
	if r >= 'a' && r <= 'z' {
		return byte(r-'a') + 1
	}
	if r >= 'A' && r <= 'Z' {
		return byte(r-'A') + 1
	}
	switch r {
	case ' ':
		return 0x00
	case '\\':
		return 0x1c
	case ']':
		return 0x1d
	case '^':
		return 0x1e
	case '_':
		return 0x1f
	default:
		return 0
	}
}

func teaFunctionKey(code rune) string {
	switch code {
	case tea.KeyF1:
		return "\x1bOP"
	case tea.KeyF2:
		return "\x1bOQ"
	case tea.KeyF3:
		return "\x1bOR"
	case tea.KeyF4:
		return "\x1bOS"
	case tea.KeyF5:
		return "\x1b[15~"
	case tea.KeyF6:
		return "\x1b[17~"
	case tea.KeyF7:
		return "\x1b[18~"
	case tea.KeyF8:
		return "\x1b[19~"
	case tea.KeyF9:
		return "\x1b[20~"
	case tea.KeyF10:
		return "\x1b[21~"
	case tea.KeyF11:
		return "\x1b[23~"
	case tea.KeyF12:
		return "\x1b[24~"
	default:
		return ""
	}
}

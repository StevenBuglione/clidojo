package ui

import "github.com/gdamore/tcell/v2"

type Theme struct {
	HeaderBG tcell.Color
	HeaderFG tcell.Color
	StatusBG tcell.Color
	StatusFG tcell.Color
	HUDTitle tcell.Color
	Accent   tcell.Color
	Fail     tcell.Color
	Pass     tcell.Color
}

func DefaultTheme() Theme {
	return Theme{
		HeaderBG: tcell.PaletteColor(24),
		HeaderFG: tcell.ColorWhite,
		StatusBG: tcell.PaletteColor(236),
		StatusFG: tcell.ColorWhite,
		HUDTitle: tcell.PaletteColor(110),
		Accent:   tcell.PaletteColor(81),
		Fail:     tcell.PaletteColor(203),
		Pass:     tcell.PaletteColor(78),
	}
}

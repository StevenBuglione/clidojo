package ui

import lipgloss "charm.land/lipgloss/v2"

type Theme struct {
	Header         lipgloss.Style
	Status         lipgloss.Style
	PanelTitle     lipgloss.Style
	PanelBorder    lipgloss.Style
	PanelBody      lipgloss.Style
	Overlay        lipgloss.Style
	OverlayTitle   lipgloss.Style
	Accent         lipgloss.Style
	Pass           lipgloss.Style
	Fail           lipgloss.Style
	Pending        lipgloss.Style
	Muted          lipgloss.Style
	Info           lipgloss.Style
	TerminalBorder lipgloss.Style
}

func DefaultTheme() Theme {
	amber := lipgloss.Color("#E9B44C")
	mint := lipgloss.Color("#7BC8A4")
	brick := lipgloss.Color("#D96C6C")
	ink := lipgloss.Color("#20242B")
	slate := lipgloss.Color("#2C323D")
	powder := lipgloss.Color("#E7EAF0")
	blue := lipgloss.Color("#77B5D9")

	return Theme{
		Header: lipgloss.NewStyle().
			Background(ink).
			Foreground(powder).
			Padding(0, 1),
		Status: lipgloss.NewStyle().
			Background(slate).
			Foreground(powder).
			Padding(0, 1),
		PanelTitle: lipgloss.NewStyle().
			Foreground(amber).
			Bold(true),
		PanelBorder: lipgloss.NewStyle().
			Foreground(slate),
		PanelBody: lipgloss.NewStyle().
			Foreground(powder),
		Overlay: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(amber).
			Background(ink).
			Foreground(powder).
			Padding(1, 2),
		OverlayTitle: lipgloss.NewStyle().
			Foreground(amber).
			Bold(true),
		Accent: lipgloss.NewStyle().
			Foreground(blue).
			Bold(true),
		Pass: lipgloss.NewStyle().
			Foreground(mint).
			Bold(true),
		Fail: lipgloss.NewStyle().
			Foreground(brick).
			Bold(true),
		Pending: lipgloss.NewStyle().
			Foreground(amber),
		Muted: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#98A2B3")),
		Info: lipgloss.NewStyle().
			Foreground(blue),
		TerminalBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#415062")),
	}
}

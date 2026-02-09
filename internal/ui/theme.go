package ui

import lipgloss "github.com/charmbracelet/lipgloss/v2"

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
	return ThemeForVariant("modern_arcade")
}

func ThemeForVariant(variant string) Theme {
	switch variant {
	case "cozy_clean":
		return cozyCleanTheme()
	case "retro_terminal":
		return retroTerminalTheme()
	default:
		return modernArcadeTheme()
	}
}

func modernArcadeTheme() Theme {
	amber := lipgloss.Color("#FFC857")
	mint := lipgloss.Color("#67F0A8")
	brick := lipgloss.Color("#FF6F91")
	ink := lipgloss.Color("#0E1420")
	slate := lipgloss.Color("#1B2740")
	powder := lipgloss.Color("#EAF2FF")
	blue := lipgloss.Color("#5EEBFF")
	border := lipgloss.Color("#4B5F8A")

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
			Foreground(blue).
			Bold(true),
		PanelBorder: lipgloss.NewStyle().
			Foreground(border),
		PanelBody: lipgloss.NewStyle().
			Foreground(powder),
		Overlay: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(blue).
			Background(ink).
			Foreground(powder).
			Padding(1, 2),
		OverlayTitle: lipgloss.NewStyle().
			Foreground(blue).
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
			Foreground(lipgloss.Color("#9CAAC6")),
		Info: lipgloss.NewStyle().
			Foreground(blue),
		TerminalBorder: lipgloss.NewStyle().
			Foreground(border),
	}
}

func cozyCleanTheme() Theme {
	honey := lipgloss.Color("#F2B872")
	sage := lipgloss.Color("#80C4A3")
	rose := lipgloss.Color("#D17A86")
	night := lipgloss.Color("#1E2430")
	slate := lipgloss.Color("#30394A")
	paper := lipgloss.Color("#F4F6FA")
	sky := lipgloss.Color("#86B6F6")

	return Theme{
		Header:      lipgloss.NewStyle().Background(night).Foreground(paper).Padding(0, 1),
		Status:      lipgloss.NewStyle().Background(slate).Foreground(paper).Padding(0, 1),
		PanelTitle:  lipgloss.NewStyle().Foreground(honey).Bold(true),
		PanelBorder: lipgloss.NewStyle().Foreground(slate),
		PanelBody:   lipgloss.NewStyle().Foreground(paper),
		Overlay: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(honey).
			Background(night).
			Foreground(paper).
			Padding(1, 2),
		OverlayTitle: lipgloss.NewStyle().Foreground(honey).Bold(true),
		Accent:       lipgloss.NewStyle().Foreground(sky).Bold(true),
		Pass:         lipgloss.NewStyle().Foreground(sage).Bold(true),
		Fail:         lipgloss.NewStyle().Foreground(rose).Bold(true),
		Pending:      lipgloss.NewStyle().Foreground(honey),
		Muted:        lipgloss.NewStyle().Foreground(lipgloss.Color("#A3ACC2")),
		Info:         lipgloss.NewStyle().Foreground(sky),
		TerminalBorder: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4A5972")),
	}
}

func retroTerminalTheme() Theme {
	lime := lipgloss.Color("#9CF5A2")
	amber := lipgloss.Color("#E5D47A")
	red := lipgloss.Color("#FF6B6B")
	deep := lipgloss.Color("#07150A")
	forest := lipgloss.Color("#12301A")
	glow := lipgloss.Color("#C5F7C4")

	return Theme{
		Header:      lipgloss.NewStyle().Background(deep).Foreground(glow).Padding(0, 1),
		Status:      lipgloss.NewStyle().Background(forest).Foreground(glow).Padding(0, 1),
		PanelTitle:  lipgloss.NewStyle().Foreground(amber).Bold(true),
		PanelBorder: lipgloss.NewStyle().Foreground(forest),
		PanelBody:   lipgloss.NewStyle().Foreground(glow),
		Overlay: lipgloss.NewStyle().
			BorderStyle(lipgloss.DoubleBorder()).
			BorderForeground(amber).
			Background(deep).
			Foreground(glow).
			Padding(1, 2),
		OverlayTitle: lipgloss.NewStyle().Foreground(amber).Bold(true),
		Accent:       lipgloss.NewStyle().Foreground(lime).Bold(true),
		Pass:         lipgloss.NewStyle().Foreground(lime).Bold(true),
		Fail:         lipgloss.NewStyle().Foreground(red).Bold(true),
		Pending:      lipgloss.NewStyle().Foreground(amber),
		Muted:        lipgloss.NewStyle().Foreground(lipgloss.Color("#73A17A")),
		Info:         lipgloss.NewStyle().Foreground(lime),
		TerminalBorder: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1F5C2F")),
	}
}

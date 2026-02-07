package ui

func DetermineLayoutMode(cols, rows int) LayoutMode {
	if cols < 90 || rows < 24 {
		return LayoutTooSmall
	}
	if cols >= 120 {
		return LayoutWide
	}
	return LayoutMedium
}

package ui

func DetermineLayoutMode(cols, rows int) LayoutMode {
	if cols < 80 || rows < 24 {
		return LayoutTooSmall
	}
	if cols >= 120 && rows >= 30 {
		return LayoutWide
	}
	return LayoutCompact
}

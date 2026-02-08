package main

import (
	"clidojo/internal/term"
	"clidojo/internal/ui"
)

func main() {
	pane := term.NewTerminalPane(nil)
	v := ui.New(ui.Options{TermPane: pane})
	_ = v.Run()
}

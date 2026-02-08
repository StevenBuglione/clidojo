# Charm Migration Snippets

These snippets are templates, not copy-paste final code.

## 1) Root model shell

```go
package charmui

import tea "charm.land/bubbletea/v2"

type RootModel struct {
	state AppScreen
	// submodels + shared game state
}

func (m RootModel) Init() tea.Cmd {
	return nil
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		_ = msg
		return m, nil
	case DevDemoRequestedMsg:
		m = m.applyDemo(msg.Demo)
		return m, nil
	}
	return m, nil
}

func (m RootModel) View() string {
	return m.render()
}
```

## 2) External dispatch reliability pattern

```go
// HTTP handler thread:
func (h *DevHandler) demo(w http.ResponseWriter, r *http.Request) {
	var req struct{ Demo string `json:"demo"` }
	_ = json.NewDecoder(r.Body).Decode(&req)
	h.program.Send(DevDemoRequestedMsg{Demo: req.Demo})
	w.WriteHeader(http.StatusAccepted)
}
```

## 3) Key routing policy

```go
func (m PlayingModel) routeKey(msg tea.KeyPressMsg) (PlayingModel, tea.Cmd) {
	if isGlobalFnKey(msg) {
		return m.handleGlobal(msg)
	}
	if m.overlayOpen() {
		return m.handleOverlayKey(msg)
	}
	// Pass-through by default.
	return m, sendTerminalInputCmd(encodeKey(msg))
}
```

## 4) Help + key map

```go
type KeyMap struct {
	Hints key.Binding
	Goal  key.Binding
	Check key.Binding
	Reset key.Binding
	Menu  key.Binding
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Hints, k.Goal, k.Check, k.Reset, k.Menu}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Hints, k.Goal, k.Check, k.Reset, k.Menu}}
}
```

## 5) Huh embedding pattern

```go
// Keep form as submodel.
form, cmd := m.settingsForm.Update(msg)
if f, ok := form.(*huh.Form); ok {
	m.settingsForm = f
}
return m, cmd
```

## 6) Teatest pattern

```go
func TestMainMenuToLevelSelect(t *testing.T) {
	tm := teatest.NewTestModel(t, NewRootModel(), teatest.WithInitialTermSize(128, 40))
	tm.Send(tea.KeyPressMsg{Code: tea.KeyRunes, Text: "\n"}) // select menu item
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Level Select"))
	})
}
```

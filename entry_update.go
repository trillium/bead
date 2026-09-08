package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m entryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if labels, ok := msg.(labelsMsg); ok {
		m.allLabels = []string(labels)
		m.stale = false
		m.refilterLabels()
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		inW := min(64, max(24, m.width-24))
		m.title.Width = inW
		m.labelsIn.Width = inW
		m.desc.SetWidth(max(24, min(76, m.width-8)))
		m.desc.SetHeight(min(8, max(3, m.height-22)))
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.result = entryBack
			return m, tea.Quit
		case "ctrl+s":
			if strings.TrimSpace(m.title.Value()) == "" {
				m.errMsg = "title is required"
				m.setFocus(entryTitle)
				return m, nil
			}
			m.errMsg = ""
			m.result = entrySubmit
			return m, tea.Quit
		case "tab":
			m.errMsg = ""
			m.setFocus(m.focus + 1)
			return m, nil
		case "shift+tab":
			m.errMsg = ""
			m.setFocus(m.focus - 1)
			return m, nil
		case "up":
			// In the labels field with suggestions open, up moves
			// through suggestions; everywhere else it moves fields.
			if m.focus == entryLabels && len(m.suggest) > 0 {
				if m.sugIdx > 0 {
					m.sugIdx--
				}
				return m, nil
			}
			m.errMsg = ""
			m.setFocus(m.focus - 1)
			return m, nil
		case "down":
			if m.focus == entryLabels && len(m.suggest) > 0 {
				if m.sugIdx < len(m.suggest)-1 {
					m.sugIdx++
				}
				return m, nil
			}
			m.errMsg = ""
			m.setFocus(m.focus + 1)
			return m, nil
		case "left", "right":
			if m.focus == entryPriority {
				if msg.String() == "left" && m.prio > 0 {
					m.prio--
				}
				if msg.String() == "right" && m.prio < 4 {
					m.prio++
				}
				return m, nil
			}
		case "enter":
			if m.focus == entryLabels {
				if len(m.suggest) > 0 {
					m.acceptLabel(m.suggest[m.sugIdx])
				} else {
					// No match: accept the typed text as a new label.
					m.acceptLabel(m.labelsIn.Value())
				}
				return m, nil
			}
		case "backspace", "ctrl+h":
			// Empty labels input + backspace pops the last chip.
			if m.focus == entryLabels && m.labelsIn.Value() == "" && len(m.selected) > 0 {
				m.selected = m.selected[:len(m.selected)-1]
				m.refilterLabels()
				return m, nil
			}
		}
		// Digits set priority directly when it has focus.
		if m.focus == entryPriority && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			if r := msg.Runes[0]; r >= '0' && r <= '4' {
				m.prio = int(r - '0')
				return m, nil
			}
		}
	}

	// Route everything else to the focused input.
	var cmd tea.Cmd
	switch m.focus {
	case entryTitle:
		m.title, cmd = m.title.Update(msg)
	case entryLabels:
		m.labelsIn, cmd = m.labelsIn.Update(msg)
		m.refilterLabels()
	case entryDesc:
		m.desc, cmd = m.desc.Update(msg)
	case entryPriority:
		// Nothing editable; swallow the key.
	}
	return m, cmd
}

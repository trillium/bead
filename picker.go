package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// maxPickerRows caps the picker's visible list height; the window scrolls
// with the cursor (arrows) or the filter (typing).
const maxPickerRows = 12

type pickerModel struct {
	stores  []store
	maxName int
	query   string
	items   []int // indices into stores after filtering
	cursor  int
	chosen  *store
	abort   bool
	width   int
	height  int
}

func newPickerModel(stores []store) pickerModel {
	maxName := 0
	for _, s := range stores {
		if len(s.Name) > maxName {
			maxName = len(s.Name)
		}
	}
	items := make([]int, len(stores))
	for i := range stores {
		items[i] = i
	}
	return pickerModel{stores: stores, maxName: maxName, items: items, width: 80, height: 24}
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m *pickerModel) refilter() {
	m.items = filterStores(m.stores, m.query)
	m.cursor = 0
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.abort = true
			return m, tea.Quit
		case "enter":
			if len(m.items) > 0 {
				s := m.stores[m.items[m.cursor]]
				m.chosen = &s
			} else {
				m.abort = true
			}
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "ctrl+n":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
			return m, nil
		case "pgup":
			m.cursor -= visibleRows(m.height, len(m.items))
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, nil
		case "pgdown":
			m.cursor += visibleRows(m.height, len(m.items))
			if last := len(m.items) - 1; m.cursor > last {
				m.cursor = last
			}
			return m, nil
		case "home":
			m.cursor = 0
			return m, nil
		case "end":
			m.cursor = len(m.items) - 1
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, nil
		case "backspace", "ctrl+h":
			if r := []rune(m.query); len(r) > 0 {
				m.query = string(r[:len(r)-1])
				m.refilter()
			}
			return m, nil
		case "ctrl+u":
			m.query = ""
			m.refilter()
			return m, nil
		case "ctrl+w":
			m.query = dropLastWord(m.query)
			m.refilter()
			return m, nil
		}
		// Anything else carrying runes (letters included) types into the
		// filter: the list narrows and the first match is highlighted.
		// Deliberately no vim j/k navigation — those are filter input.
		if msg.Type == tea.KeyRunes {
			m.query += string(msg.Runes)
			m.refilter()
			return m, nil
		}
		if s := msg.String(); s == " " || s == "space" {
			m.query += " "
			m.refilter()
			return m, nil
		}
	}
	return m, nil
}

func (m pickerModel) View() string {
	th := activeTheme()
	var b strings.Builder
	n := len(m.items)
	rows := visibleRows(m.height, n)

	start := 0
	if n > rows {
		start = m.cursor - rows/2
		if start < 0 {
			start = 0
		}
		if start > n-rows {
			start = n - rows
		}
	}
	end := start + rows
	if end > n {
		end = n
	}

	fmt.Fprintf(&b, "%s\n", th.Title.Render("Select a bead store"))
	fmt.Fprintf(&b, "  %s %s%s  %s\n",
		th.Accent.Render("❯"),
		m.query,
		th.Dim.Render("█"),
		th.Dim.Render(fmt.Sprintf("%d/%d", n, len(m.stores))),
	)

	if n == 0 {
		fmt.Fprintf(&b, "  %s\n", th.Dim.Render("(no matches)"))
	} else {
		if start > 0 {
			fmt.Fprintf(&b, "  %s\n", th.Dim.Render(fmt.Sprintf("▲ %d more", start)))
		}
		for _, idx := range m.items[start:end] {
			s := m.stores[idx]
			name := padRight(s.Name, m.maxName)
			about := shorten(s.About, 70)
			if idx == m.items[m.cursor] {
				fmt.Fprintf(&b, "%s\n", th.Reverse.Render(fmt.Sprintf("  %s  %s", name, about)))
			} else {
				fmt.Fprintf(&b, "  %s  %s\n",
					th.Title.Render(name),
					th.Dim.Render(about))
			}
		}
		if end < n {
			fmt.Fprintf(&b, "  %s\n", th.Dim.Render(fmt.Sprintf("▼ %d more", n-end)))
		}
	}

	fmt.Fprintf(&b, "  %s\n", th.Dim.Render("↑↓ move · type to filter · ⏎ select · esc quit"))
	return b.String()
}

// runPickerTUI runs the fullscreen store picker. ok=false means the user
// bailed (esc / ctrl-c) or chose nothing.
func runPickerTUI(stores []store) (store, bool) {
	p := tea.NewProgram(newPickerModel(stores), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		// The TUI is a convenience, not a requirement: fall back to the
		// line prompt rather than dying.
		fmt.Fprintf(os.Stderr, "bead: picker unavailable (%v), falling back.\n", err)
		return pickStoreLine(stores)
	}
	m, ok := final.(pickerModel)
	if !ok || m.abort || m.chosen == nil {
		return store{}, false
	}
	return *m.chosen, true
}

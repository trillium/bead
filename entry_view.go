package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m entryModel) View() string {
	th := activeTheme()
	var b strings.Builder
	title := fmt.Sprintf("New bead in %s", m.storeName)
	if m.dryRun {
		title += "  " + th.Dim.Render("[dry run]")
	}
	fmt.Fprintf(&b, "%s\n\n", th.Title.Render(title))
	if m.errMsg != "" {
		fmt.Fprintf(&b, "  %s\n\n", th.Danger.Render("⚠ "+m.errMsg))
	}

	marker := func(f int) string {
		if m.focus == f {
			return th.Accent.Render("❯")
		}
		return " "
	}
	// navHint tags the destinations: the field tab moves to gets
	// -- TAB --, the one shift+tab returns to gets -- SHIFT+TAB --.
	navHint := func(f int) string {
		if f == (m.focus+1)%entryFieldCount {
			return "  " + th.Hint.Render("-- TAB --")
		}
		if f == (m.focus+entryFieldCount-1)%entryFieldCount {
			return "  " + th.Hint.Render("-- SHIFT+TAB --")
		}
		return ""
	}

	// Title.
	fmt.Fprintf(&b, "  %s %s%s\n", marker(entryTitle), th.Title.Render("Title"), navHint(entryTitle))
	fmt.Fprintf(&b, "    %s\n\n", m.title.View())

	// Priority.
	fmt.Fprintf(&b, "  %s %s  %s %s %s  %s%s\n\n", marker(entryPriority),
		th.Title.Render("Priority"),
		th.Dim.Render("⟨"), th.Name.Render(fmt.Sprintf("%d", m.prio)), th.Dim.Render("⟩"),
		th.Dim.Render("←/→ or 0-4"), navHint(entryPriority),
	)

	// Labels.
	fmt.Fprintf(&b, "  %s %s%s", marker(entryLabels), th.Title.Render("Labels"), navHint(entryLabels))
	if m.stale && m.allLabels != nil {
		fmt.Fprintf(&b, "  %s", th.Dim.Render("updating…"))
	}
	for _, s := range m.selected {
		fmt.Fprintf(&b, "  %s", th.Reverse.Render(" "+s+" "))
	}
	fmt.Fprintf(&b, "\n    %s\n", m.labelsIn.View())
	if m.allLabels == nil {
		fmt.Fprintf(&b, "    %s\n", th.Dim.Render("loading labels…"))
	}
	shown := m.suggest
	if len(shown) > 6 {
		shown = shown[:6]
	}
	for i, s := range shown {
		if m.focus == entryLabels && i == m.sugIdx {
			fmt.Fprintf(&b, "    %s\n", th.Reverse.Render(" "+s+" "))
		} else {
			fmt.Fprintf(&b, "    %s\n", th.Dim.Render(" "+s))
		}
	}
	if len(m.suggest) > len(shown) {
		fmt.Fprintf(&b, "    %s\n", th.Dim.Render(fmt.Sprintf("… +%d more", len(m.suggest)-len(shown))))
	}
	fmt.Fprintf(&b, "\n")

	// Description.
	fmt.Fprintf(&b, "  %s %s%s\n", marker(entryDesc), th.Title.Render("Description"), navHint(entryDesc))
	fmt.Fprintf(&b, "    %s\n\n", strings.ReplaceAll(m.desc.View(), "\n", "\n    "))

	fmt.Fprintf(&b, "  %s\n", th.Dim.Render(
		"tab/↑↓ fields · ←/→ priority · ⏎ add label · ctrl+s create · esc back"))
	return b.String()
}

// runEntryTUI runs the bead entry form. res is submit or back (esc).
func runEntryTUI(m entryModel) (entryModel, entryResult) {
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bead: entry form unavailable (%v)\n", err)
		return m, entryBack
	}
	em, ok := final.(entryModel)
	if !ok || em.result == entryNone {
		return m, entryBack
	}
	return em, em.result
}

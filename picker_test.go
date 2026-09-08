package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// keyMsg builds synthetic key presses for model tests.
func keyMsg(s string) tea.Msg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "space":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func testStores() []store {
	return []store{
		{Name: "brain", About: "Knowledge, documents, notes"},
		{Name: "task", About: "Actionable work items"},
		{Name: "external_llm_tasks", About: "Handoff asks for prose"},
	}
}

func TestFilterStores(t *testing.T) {
	stores := testStores()

	if got := filterStores(stores, ""); len(got) != 3 {
		t.Errorf("empty query = %v, want all 3", got)
	}
	// "task" matches task (name) and external_llm_tasks (name), in order.
	got := filterStores(stores, "task")
	if len(got) != 2 || stores[got[0]].Name != "task" || stores[got[1]].Name != "external_llm_tasks" {
		t.Errorf("query task = %v, want [task external_llm_tasks]", got)
	}
	// About text matches too, case-insensitively.
	got = filterStores(stores, "KNOWLEDGE")
	if len(got) != 1 || stores[got[0]].Name != "brain" {
		t.Errorf("query KNOWLEDGE = %v, want [brain]", got)
	}
	if got := filterStores(stores, "zzz-nope"); len(got) != 0 {
		t.Errorf("query zzz-nope = %v, want []", got)
	}
}

func TestVisibleRows(t *testing.T) {
	if got := visibleRows(24, 38); got != maxPickerRows {
		t.Errorf("tall terminal = %d, want %d", got, maxPickerRows)
	}
	if got := visibleRows(10, 38); got != 4 {
		t.Errorf("short terminal = %d, want 4", got)
	}
	if got := visibleRows(24, 3); got != 3 {
		t.Errorf("few items = %d, want 3", got)
	}
	if got := visibleRows(24, 0); got != 1 {
		t.Errorf("no items = %d, want 1 (empty state still renders)", got)
	}
}

func TestPickerEnterEmptyAborts(t *testing.T) {
	m := newPickerModel(testStores())
	m.query = "zzz-nope"
	m.refilter()
	updated, _ := m.Update(keyMsg("enter"))
	pm := updated.(pickerModel)
	if !pm.abort || pm.chosen != nil {
		t.Errorf("enter on no matches should abort, got %+v", pm)
	}
}

func TestPickerTypingFilters(t *testing.T) {
	m := newPickerModel(testStores())
	updated, _ := m.Update(keyMsg("t"))
	updated, _ = updated.(pickerModel).Update(keyMsg("a"))
	updated, _ = updated.(pickerModel).Update(keyMsg("s"))
	pm := updated.(pickerModel)
	if pm.query != "tas" {
		t.Errorf("query = %q, want tas", pm.query)
	}
	if len(pm.items) != 2 || pm.cursor != 0 {
		t.Errorf("items = %v cursor = %d, want 2 items at cursor 0", pm.items, pm.cursor)
	}
	updated, _ = pm.Update(keyMsg("esc"))
	if !updated.(pickerModel).abort {
		t.Errorf("esc should abort")
	}
}

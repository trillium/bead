package main

import (
	"strings"
	"testing"
)

func testEntry() entryModel {
	m := newEntryModel("task", "", false)
	m.allLabels = []string{"bug", "frontend", "backend", "bug-fix"}
	m.refilterLabels()
	return m
}

func update(m entryModel, keys ...string) entryModel {
	for _, k := range keys {
		u, _ := m.Update(keyMsg(k))
		m = u.(entryModel)
	}
	return m
}

func typeText(m entryModel, s string) entryModel {
	for _, r := range s {
		u, _ := m.Update(keyMsg(string(r)))
		m = u.(entryModel)
	}
	return m
}

func TestEntryDefaults(t *testing.T) {
	m := testEntry()
	if m.focus != entryTitle {
		t.Errorf("focus = %d, want title", m.focus)
	}
	if m.prio != 2 {
		t.Errorf("prio = %d, want 2", m.prio)
	}
	if len(m.suggest) != 4 {
		t.Errorf("suggest = %v, want all 4 labels", m.suggest)
	}
}

func TestEntryNavHints(t *testing.T) {
	// Focus title: next=priority gets TAB, prev=desc gets SHIFT+TAB.
	v := testEntry().View()
	for _, want := range []string{"-- TAB --", "-- SHIFT+TAB --"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}
	lines := strings.Split(v, "\n")
	var prioLine, descLine string
	for _, l := range lines {
		// Headers carry ❯ or a -- hint; skips textarea placeholder text.
		if strings.Contains(l, "Priority") && (strings.Contains(l, "❯") || strings.Contains(l, "-- ")) {
			prioLine = l
		}
		if strings.Contains(l, "Description") && (strings.Contains(l, "❯") || strings.Contains(l, "-- ")) {
			descLine = l
		}
	}
	if !strings.Contains(prioLine, "-- TAB --") || strings.Contains(prioLine, "SHIFT+TAB") {
		t.Errorf("priority line should carry TAB hint only: %q", prioLine)
	}
	if !strings.Contains(descLine, "-- SHIFT+TAB --") || strings.Contains(descLine, "-- TAB --") {
		t.Errorf("desc line should carry SHIFT+TAB hint only: %q", descLine)
	}
	// After tab to priority, hints rotate: labels gets TAB, title gets SHIFT+TAB.
	v = update(testEntry(), "tab").View()
	if !strings.Contains(v, "Labels") || !strings.Contains(v, "-- TAB --") {
		t.Errorf("rotated view missing TAB hint")
	}
}

func TestEntryTabCycles(t *testing.T) {
	m := testEntry()
	m = update(m, "tab", "tab", "tab")
	if m.focus != entryDesc {
		t.Errorf("3x tab focus = %d, want desc", m.focus)
	}
	m = update(m, "tab")
	if m.focus != entryTitle {
		t.Errorf("4x tab focus = %d, want wrap to title", m.focus)
	}
	m = update(m, "shift+tab")
	if m.focus != entryDesc {
		t.Errorf("shift+tab focus = %d, want desc", m.focus)
	}
}

func TestEntryUpDownMoves(t *testing.T) {
	m := testEntry()
	m = update(m, "down")
	if m.focus != entryPriority {
		t.Errorf("down focus = %d, want priority", m.focus)
	}
	m = update(m, "up", "up")
	if m.focus != entryDesc {
		t.Errorf("up x2 focus = %d, want wrap to desc", m.focus)
	}
}

func TestEntryPriorityKeys(t *testing.T) {
	m := update(testEntry(), "tab") // focus priority
	m = update(m, "right", "right")
	if m.prio != 4 {
		t.Errorf("prio = %d, want 4", m.prio)
	}
	m = update(m, "right")
	if m.prio != 4 {
		t.Errorf("prio = %d, want clamp at 4", m.prio)
	}
	m = update(m, "left", "left", "left", "left", "left")
	if m.prio != 0 {
		t.Errorf("prio = %d, want clamp at 0", m.prio)
	}
	m = update(m, "3")
	if m.prio != 3 {
		t.Errorf("prio = %d, want digit 3", m.prio)
	}
	m = update(m, "9")
	if m.prio != 3 {
		t.Errorf("prio = %d, want 9 ignored", m.prio)
	}
}

func TestEntryLabelAccept(t *testing.T) {
	m := update(testEntry(), "tab", "tab") // focus labels
	m = typeText(m, "bug")
	if len(m.suggest) != 2 { // bug, bug-fix
		t.Errorf("suggest = %v, want [bug bug-fix]", m.suggest)
	}
	m = update(m, "enter")
	if len(m.selected) != 1 || m.selected[0] != "bug" {
		t.Errorf("selected = %v, want [bug]", m.selected)
	}
	if m.labelsIn.Value() != "" {
		t.Errorf("input not cleared: %q", m.labelsIn.Value())
	}
	for _, s := range m.suggest {
		if s == "bug" {
			t.Errorf("accepted label still suggested: %v", m.suggest)
		}
	}
}

func TestEntryLabelSuggestionsNav(t *testing.T) {
	m := update(testEntry(), "tab", "tab")
	m = update(m, "down", "down")
	if m.sugIdx != 2 {
		t.Errorf("sugIdx = %d, want 2", m.sugIdx)
	}
	if m.focus != entryLabels {
		t.Errorf("down in labels moved field to %d", m.focus)
	}
	m = update(m, "enter")
	if len(m.selected) != 1 || m.selected[0] != "backend" {
		t.Errorf("selected = %v, want [backend]", m.selected)
	}
}

func TestEntryLabelFreeTextAndPop(t *testing.T) {
	m := update(testEntry(), "tab", "tab")
	m = typeText(m, "zzz-new")
	m = update(m, "enter")
	if len(m.selected) != 1 || m.selected[0] != "zzz-new" {
		t.Errorf("selected = %v, want free-text [zzz-new]", m.selected)
	}
	m = update(m, "backspace") // input empty → pop chip
	if len(m.selected) != 0 {
		t.Errorf("selected = %v, want popped", m.selected)
	}
}

func TestEntryLabelNoDupes(t *testing.T) {
	m := update(testEntry(), "tab", "tab")
	m = typeText(m, "zzz")
	m = update(m, "enter")
	m = typeText(m, "ZZZ") // no suggestion (selected excluded) → free-text path
	m = update(m, "enter")
	if len(m.selected) != 1 || m.selected[0] != "zzz" {
		t.Errorf("selected = %v, want single [zzz]", m.selected)
	}
}

func TestEntrySubmitGuards(t *testing.T) {
	m := update(testEntry(), "ctrl+s")
	if m.result != entryNone || m.errMsg == "" {
		t.Errorf("empty title submit: result=%d err=%q, want blocked", m.result, m.errMsg)
	}
	m = typeText(m, "my title")
	m = update(m, "ctrl+s")
	if m.result != entrySubmit {
		t.Errorf("result = %d, want submit", m.result)
	}
	u, _ := testEntry().Update(keyMsg("esc"))
	if u.(entryModel).result != entryBack {
		t.Errorf("esc should give entryBack")
	}
}

func TestEntryCreateArgs(t *testing.T) {
	m := update(testEntry(), "tab", "tab")
	m = typeText(m, "frontend")
	m = update(m, "enter")
	m = update(m, "tab", "tab", "shift+tab", "shift+tab") // wander and back to labels
	// cycle back to title: focus is labels(2) -> tab desc(3) -> tab title(0)
	m = update(m, "tab", "tab")
	m = typeText(m, "Fix it")
	// leave typed remainder in labels input
	m = update(m, "tab", "tab") // priority, labels
	m = typeText(m, "extra")
	args := m.createArgs()
	joined := strings.Join(args, " ")
	for _, want := range []string{"create", "Fix it", "-t task", "-p 2", "frontend,extra"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %q missing %q", joined, want)
		}
	}

	plain := testEntry()
	plain.title.SetValue("T")
	args = plain.createArgs()
	for _, a := range args {
		if a == "-l" {
			t.Errorf("no labels but -l present: %q", args)
		}
	}
}

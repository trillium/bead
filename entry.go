package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Entry-form fields, in tab order. Title is focused first.
const (
	entryTitle = iota
	entryPriority
	entryLabels
	entryDesc
	entryFieldCount
)

type entryResult int

const (
	entryNone entryResult = iota
	entrySubmit
	entryBack
)

type entryModel struct {
	storeName string
	title     textinput.Model
	prio      int
	labelsIn  textinput.Model
	selected  []string
	allLabels []string // nil = still loading
	suggest   []string // filtered, excluding selected
	sugIdx    int
	desc      textarea.Model
	stale     bool // labels rendered from an expired cache; refresh in flight
	focus     int
	dryRun    bool
	errMsg    string
	result    entryResult
	width     int
	height    int
}

func newEntryModel(storeName, prefillTitle string, dryRun bool) entryModel {
	ti := textinput.New()
	ti.Placeholder = "Short one-line title"
	ti.SetValue(prefillTitle)
	ti.Focus()

	li := textinput.New()
	li.Placeholder = "type to search, ⏎ to add"

	ta := textarea.New()
	ta.Placeholder = "Description (markdown ok)…"
	ta.ShowLineNumbers = false
	ta.SetWidth(72)
	ta.SetHeight(6)

	m := entryModel{
		storeName: storeName,
		title:     ti,
		prio:      2,
		labelsIn:  li,
		desc:      ta,
		focus:     entryTitle,
		dryRun:    dryRun,
		width:     80,
		height:    24,
	}
	m.refilterLabels()
	return m
}

func (m entryModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, fetchLabelsCmd(m.storeName))
}

func (m *entryModel) refilterLabels() {
	m.suggest = matchLabels(m.allLabels, m.selected, m.labelsIn.Value())
	m.sugIdx = 0
}

func (m *entryModel) setFocus(f int) {
	m.focus = (f + entryFieldCount) % entryFieldCount
	if m.focus == entryTitle {
		m.title.Focus()
	} else {
		m.title.Blur()
	}
	if m.focus == entryLabels {
		m.labelsIn.Focus()
	} else {
		m.labelsIn.Blur()
	}
	if m.focus == entryDesc {
		m.desc.Focus()
	} else {
		m.desc.Blur()
	}
}

func (m *entryModel) acceptLabel(l string) {
	l = strings.TrimSpace(l)
	if l == "" {
		return
	}
	for _, s := range m.selected {
		if strings.EqualFold(s, l) {
			return
		}
	}
	m.selected = append(m.selected, l)
	m.labelsIn.SetValue("")
	m.refilterLabels()
}

// allLabelValues merges chips with any typed remainder (comma-separated).
func (m entryModel) allLabelValues() []string {
	var out []string
	seen := map[string]bool{}
	add := func(l string) {
		l = strings.TrimSpace(l)
		if l == "" || seen[strings.ToLower(l)] {
			return
		}
		seen[strings.ToLower(l)] = true
		out = append(out, l)
	}
	for _, s := range m.selected {
		add(s)
	}
	for _, part := range strings.Split(m.labelsIn.Value(), ",") {
		add(part)
	}
	return out
}

func (m entryModel) body() string { return m.desc.Value() }

// createArgs builds `<store> create …` (body goes via --body-file by caller).
// Type is always "task": the form has no type field by design.
func (m entryModel) createArgs() []string {
	args := []string{"create", strings.TrimSpace(m.title.Value()), "-t", "task", "-p", fmt.Sprint(m.prio)}
	if labels := m.allLabelValues(); len(labels) > 0 {
		args = append(args, "-l", strings.Join(labels, ","))
	}
	return args
}

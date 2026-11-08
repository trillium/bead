package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const sampleDesc = `QUESTION (plain)
Can you sign in tonight?

WHY IT MATTERS
Needs two new tables. See https://example.com/pr/61.
1. A numbered step that is not an option.

OPTIONS
- Yes, apply them tonight (recommended): I prepare the commands; you run them.
- Their side applies them: Slower.
- Not tonight: Goes live later.

IF YOU DO NOT ANSWER
Not tonight.`

func TestParseOptionsBullets(t *testing.T) {
	got := parseOptions(sampleDesc)
	if len(got) != 3 {
		t.Fatalf("got %d options: %+v", len(got), got)
	}
	want := decisionOption{Key: "1", Name: "Yes, apply them tonight", Detail: "I prepare the commands; you run them.", Recommended: true}
	if got[0] != want {
		t.Errorf("opt 0 = %+v, want %+v", got[0], want)
	}
	if got[2].Key != "3" || got[2].Name != "Not tonight" || got[2].Recommended {
		t.Errorf("opt 2 = %+v", got[2])
	}
	if a := optionAnswer(got[0]); a != "Yes, apply them tonight" {
		t.Errorf("answer = %q", a)
	}
}

func TestParseOptionsLetteredAndNumbered(t *testing.T) {
	lettered := parseOptions("OPTIONS:\nA) Ship it now (recommended): fast\nB) Wait a week\n\nNOTES\nx")
	if len(lettered) != 2 || lettered[0].Key != "a" || lettered[1].Key != "b" || !lettered[0].Recommended {
		t.Fatalf("lettered = %+v", lettered)
	}
	if a := optionAnswer(lettered[1]); a != "Option B: Wait a week" {
		t.Errorf("answer = %q", a)
	}
	numbered := parseOptions("Options\n1. Red\n2. Blue\n   still blue, more detail\n")
	if len(numbered) != 2 || numbered[1].Key != "2" || numbered[1].Detail != "still blue, more detail" {
		t.Fatalf("numbered = %+v", numbered)
	}
	if a := optionAnswer(numbered[0]); a != "Option 1: Red" {
		t.Errorf("answer = %q", a)
	}
}

func TestParseOptionsFallbackAndNone(t *testing.T) {
	got := parseOptions("Pick one.\nOption A: first\nOption B - second\n")
	if len(got) != 2 || got[0].Name != "first" || got[1].Key != "b" {
		t.Fatalf("fallback = %+v", got)
	}
	if got := parseOptions("WHY\n1. a step\n2. another step\n"); got != nil {
		t.Errorf("numbered steps outside OPTIONS must not be options: %+v", got)
	}
	if got := parseOptions(""); got != nil {
		t.Errorf("empty = %+v", got)
	}
}

func TestItemLinks(t *testing.T) {
	got := itemLinks("see https://a.example/x, and (https://b.example/y) then https://a.example/x.")
	want := []string{"https://a.example/x", "https://b.example/y"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("links = %v, want %v", got, want)
	}
}

func TestParseDecisionsAndComments(t *testing.T) {
	data := []byte("warning: noise\n[{\"id\":\"task-1\",\"title\":\"T\",\"description\":\"D\",\"priority\":1,\"created_at\":\"2026-01-02T00:00:00Z\",\"labels\":[\"captain-decision\"],\"comment_count\":2},{\"id\":\"\"}]")
	got, err := parseDecisions("task", data)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v %v", got, err)
	}
	if d := got[0]; d.Store != "task" || d.ID != "task-1" || d.Priority != 1 || d.CommentCount != 2 || d.Labels[0] != decisionLabel {
		t.Errorf("decision = %+v", d)
	}
	if _, err := parseDecisions("task", []byte("oops")); err == nil {
		t.Error("want error without array")
	}
	cs, err := parseComments([]byte(`[{"author":"A","text":"hello","created_at":"2026-01-02T03:04:05Z"}]`))
	if err != nil || len(cs) != 1 || cs[0].Text != "hello" || cs[0].Author != "A" {
		t.Errorf("comments = %v %v", cs, err)
	}
}

func TestWaitingDecisionsOrderAndFilter(t *testing.T) {
	ds := []decision{
		{Store: "task", ID: "t-old", Priority: 1, CreatedAt: "2026-01-01T00:00:00Z"},
		{Store: "task", ID: "t-p2", Priority: 2, CreatedAt: "2026-03-01T00:00:00Z"},
		{Store: "review", ID: "r-new", Priority: 1, CreatedAt: "2026-02-01T00:00:00Z"},
		{Store: "task", ID: "t-p0", Priority: 0, CreatedAt: "2025-12-01T00:00:00Z"},
		{Store: "task", ID: "t-done", Priority: 0, CreatedAt: "2026-04-01T00:00:00Z", Labels: []string{answeredLabel}},
	}
	ids := func(ds []decision) string {
		var out []string
		for _, d := range ds {
			out = append(out, d.ID)
		}
		return strings.Join(out, " ")
	}
	if got := ids(waitingDecisions(ds, false)); got != "t-p0 r-new t-old t-p2" {
		t.Errorf("waiting = %s", got)
	}
	if got := ids(waitingDecisions(ds, true)); got != "t-done t-p0 r-new t-old t-p2" {
		t.Errorf("all = %s", got)
	}
}

func TestAnswerCommentStamp(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	if got := answerComment("  Not tonight \n", now); got != "Answer (captain, 2026-11-05): Not tonight" {
		t.Errorf("comment = %q", got)
	}
}

func TestSaveStepsNeverCloseUnlessAsked(t *testing.T) {
	d := decision{Store: "task", ID: "task-1"}
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)

	steps := saveSteps(d, outcomeAnswered, "Yes", now)
	if len(steps) != 2 {
		t.Fatalf("steps = %+v", steps)
	}
	if !reflect.DeepEqual(steps[0].Argv, []string{"comments", "add", "task-1", "-f", "/dev/stdin"}) ||
		steps[0].Stdin != "Answer (captain, 2026-11-05): Yes\n" {
		t.Errorf("comment step = %+v", steps[0])
	}
	if !reflect.DeepEqual(steps[1].Argv, []string{"label", "add", "task-1", answeredLabel}) {
		t.Errorf("label step = %+v", steps[1])
	}

	if steps := saveSteps(d, outcomeComment, "Yes", now); len(steps) != 1 {
		t.Errorf("comment-only steps = %+v", steps)
	}
	for _, o := range []decideOutcome{outcomeAnswered, outcomeComment} {
		for _, s := range saveSteps(d, o, "Yes", now) {
			if s.Argv[0] == "close" {
				t.Errorf("outcome %v closed the item", o)
			}
		}
	}
	closing := saveSteps(d, outcomeClose, "Yes", now)
	if len(closing) != 2 || closing[0].Argv[0] != "comments" || closing[1].Argv[0] != "close" {
		t.Errorf("close steps = %+v (comment must come first)", closing)
	}
}

func TestFormatStep(t *testing.T) {
	got := formatStep("task", step{Argv: []string{"comments", "add", "task-1", "-f", "/dev/stdin"}, Stdin: "hi\n"})
	if got != "task comments add task-1 -f /dev/stdin   # stdin: hi" {
		t.Errorf("line = %q", got)
	}
}

func TestWrapTextHangingIndent(t *testing.T) {
	got := wrapText("- one two three four five six seven", 20)
	want := "- one two three four\n  five six seven"
	if got != want {
		t.Errorf("wrapped = %q, want %q", got, want)
	}
	if got := wrapText("short\n\nlines", 20); got != "short\n\nlines" {
		t.Errorf("kept lines = %q", got)
	}
}

func TestRenderDecisionBody(t *testing.T) {
	d := decision{Store: "task", ID: "task-1", Title: "DECIDE: A thing?", Description: sampleDesc, Priority: 0,
		Labels: []string{decisionLabel}}
	out := renderDecisionBody(d, []comment{{Author: "Firstmate", Text: "context note", CreatedAt: "2026-11-05T10:00:00Z"}}, "", 60, activeTheme())
	for _, want := range []string{"DECIDE: A thing?", "P0 · task", "QUESTION (plain)", "OPTIONS", "Not tonight",
		"LINKS", "https://example.com/pr/61", "COMMENTS (1)", "Firstmate", "context note"} {
		if !strings.Contains(out, want) {
			t.Errorf("body missing %q:\n%s", want, out)
		}
	}
	if out := renderDecisionBody(d, nil, "loading", 60, activeTheme()); !strings.Contains(out, "loading…") {
		t.Errorf("loading state missing:\n%s", out)
	}
	if out := renderDecisionBody(decision{ID: "x"}, nil, "", 60, activeTheme()); !strings.Contains(out, "(no description)") {
		t.Errorf("empty description missing:\n%s", out)
	}
}

// --- the decide TUI ------------------------------------------------------

func dkey(s string) tea.Msg {
	switch s {
	case "ctrl+n":
		return tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
	}
	return keyMsg(s)
}

func dupdate(m decideModel, keys ...string) (decideModel, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		var u tea.Model
		u, cmd = m.Update(dkey(k))
		m = u.(decideModel)
	}
	return m, cmd
}

func testQueue(dry bool) decideModel {
	return newDecideModel([]decision{
		{Store: "task", ID: "task-1", Title: "First", Description: sampleDesc, Priority: 0},
		{Store: "task", ID: "task-2", Title: "Second", Description: "No options here.", Priority: 1},
	}, dry)
}

func TestDecideListOpensItem(t *testing.T) {
	m, _ := dupdate(testQueue(false), "down", "up", "enter")
	if m.mode != modeItem || m.cur != 0 {
		t.Fatalf("mode=%v cur=%d", m.mode, m.cur)
	}
	if len(m.opts) != 3 || m.focus != dfPick || m.pickIdx != 0 {
		t.Errorf("opts=%d focus=%d pick=%d", len(m.opts), m.focus, m.pickIdx)
	}
	m, _ = dupdate(m, "esc")
	if m.mode != modeList {
		t.Errorf("esc should return to the list")
	}
	if _, cmd := dupdate(m, "esc"); cmd == nil {
		t.Error("esc in the list should quit")
	}
}

func TestDecideItemWithoutOptionsStartsOnAnswer(t *testing.T) {
	m, _ := dupdate(testQueue(false), "down", "enter")
	if m.cur != 1 || m.focus != dfAnswer || len(m.opts) != 0 {
		t.Errorf("cur=%d focus=%d opts=%d", m.cur, m.focus, len(m.opts))
	}
	// Tab skips the missing Pick field.
	m, _ = dupdate(m, "tab", "tab", "tab")
	if m.focus != dfAnswer {
		t.Errorf("focus after full tab cycle = %d, want answer", m.focus)
	}
}

func TestDecideQuickPickFillsAnswer(t *testing.T) {
	m, _ := dupdate(testQueue(false), "enter", "2")
	if got := m.answer.Value(); got != "Their side applies them" {
		t.Errorf("answer = %q", got)
	}
	if m.focus != dfAnswer || m.picked != 1 {
		t.Errorf("focus=%d picked=%d", m.focus, m.picked)
	}
	// Arrow + enter picks the highlighted option too.
	m, _ = dupdate(testQueue(false), "enter", "right", "right", "enter")
	if got := m.answer.Value(); got != "Not tonight" {
		t.Errorf("answer = %q", got)
	}
	// Unknown keys on Pick change nothing.
	m, _ = dupdate(testQueue(false), "enter", "9")
	if m.answer.Value() != "" || m.focus != dfPick {
		t.Errorf("answer=%q focus=%d", m.answer.Value(), m.focus)
	}
}

func TestDecideRequiresAnAnswer(t *testing.T) {
	for _, outcome := range []decideOutcome{outcomeAnswered, outcomeComment, outcomeClose} {
		m, _ := dupdate(testQueue(false), "enter")
		m.outcome = outcome
		m, cmd := dupdate(m, "ctrl+s")
		if cmd != nil || m.errMsg == "" || m.saving {
			t.Errorf("outcome %v: saved without an answer (cmd=%v err=%q)", outcome, cmd != nil, m.errMsg)
		}
	}
	// Saving from the Save button is guarded the same way.
	m, _ := dupdate(testQueue(false), "enter", "tab", "tab", "tab", "enter")
	if m.errMsg == "" || m.saving {
		t.Errorf("save button saved without an answer (err=%q)", m.errMsg)
	}
}

func TestDecideOutcomeDefaultsToAnsweredNotClose(t *testing.T) {
	m, _ := dupdate(testQueue(false), "enter", "1", "tab")
	if m.focus != dfOutcome || m.outcome != outcomeAnswered {
		t.Fatalf("focus=%d outcome=%v", m.focus, m.outcome)
	}
	m, _ = dupdate(m, "right")
	if m.outcome != outcomeComment {
		t.Errorf("outcome = %v", m.outcome)
	}
	m, _ = dupdate(m, "right", "right")
	if m.outcome != outcomeClose {
		t.Errorf("outcome = %v (clamped at close)", m.outcome)
	}
	m, _ = dupdate(m, "left", "left", "left")
	if m.outcome != outcomeAnswered {
		t.Errorf("outcome = %v", m.outcome)
	}
}

func TestDecideSaveMovesToNextItem(t *testing.T) {
	m, _ := dupdate(testQueue(false), "enter", "1")
	m, cmd := dupdate(m, "ctrl+s")
	if cmd == nil || !m.saving {
		t.Fatal("ctrl+s with an answer should start saving")
	}
	// Keys are ignored while the save runs.
	m, _ = dupdate(m, "esc")
	if m.mode != modeItem {
		t.Error("esc during save should be ignored")
	}
	u, next := m.Update(savedMsg{idx: 0, steps: saveSteps(m.items[0], outcomeAnswered, "x", time.Now()), done: 2})
	m = u.(decideModel)
	if !m.handled[0] || m.saved != 1 || m.cur != 1 || m.mode != modeItem {
		t.Fatalf("handled=%v saved=%d cur=%d mode=%v", m.handled, m.saved, m.cur, m.mode)
	}
	if next != nil {
		t.Error("item 2 has no comments to load")
	}
	if m.answer.Value() != "" || !strings.Contains(m.status, "task-1") {
		t.Errorf("answer=%q status=%q", m.answer.Value(), m.status)
	}
	// Answering the last one ends the session.
	u, quit := m.Update(savedMsg{idx: 1, done: 2})
	m = u.(decideModel)
	if quit == nil || m.saved != 2 {
		t.Errorf("last answer should quit (saved=%d)", m.saved)
	}
}

func TestDecideSaveFailureKeepsAnswer(t *testing.T) {
	m, _ := dupdate(testQueue(false), "enter", "1", "ctrl+s")
	u, _ := m.Update(savedMsg{idx: 0, err: errors.New("task comments failed: boom")})
	m = u.(decideModel)
	if m.handled[0] || m.saving || m.mode != modeItem || m.cur != 0 {
		t.Errorf("handled=%v saving=%v cur=%d", m.handled, m.saving, m.cur)
	}
	if !strings.Contains(m.errMsg, "boom") || m.answer.Value() == "" {
		t.Errorf("err=%q answer=%q", m.errMsg, m.answer.Value())
	}
}

func TestDecidePartialFailureStillAdvances(t *testing.T) {
	m, _ := dupdate(testQueue(false), "enter", "1", "ctrl+s")
	u, _ := m.Update(savedMsg{idx: 0, done: 1, err: errors.New("task label failed: boom")})
	m = u.(decideModel)
	if !m.handled[0] || len(m.notes) != 1 || m.cur != 1 {
		t.Errorf("handled=%v notes=%v cur=%d", m.handled, m.notes, m.cur)
	}
}

func TestDecideDryRunRunsNothing(t *testing.T) {
	m, _ := dupdate(testQueue(true), "enter", "1")
	m, cmd := dupdate(m, "ctrl+s")
	if m.saving || cmd == nil {
		t.Fatal("dry run should answer immediately through a message")
	}
	msg, ok := cmd().(savedMsg)
	if !ok || msg.done != len(msg.steps) || msg.err != nil {
		t.Fatalf("msg = %+v", msg)
	}
	u, _ := m.Update(msg)
	m = u.(decideModel)
	if len(m.planned) != 2 || !strings.HasPrefix(m.planned[0], "task comments add task-1") ||
		!strings.Contains(m.planned[1], "label add task-1 "+answeredLabel) {
		t.Errorf("planned = %v", m.planned)
	}
}

func TestDecideSkipNextAndPrevious(t *testing.T) {
	m, _ := dupdate(testQueue(false), "enter", "ctrl+n")
	if m.cur != 1 {
		t.Errorf("cur = %d after ctrl+n", m.cur)
	}
	m, _ = dupdate(m, "ctrl+n")
	if m.cur != 0 {
		t.Errorf("cur = %d after wrapping", m.cur)
	}
	m, _ = dupdate(m, "ctrl+p")
	if m.cur != 1 || m.handled[0] || m.saved != 0 {
		t.Errorf("cur=%d handled=%v", m.cur, m.handled)
	}
}

func TestDecideCommentsLoadForItemsWithThem(t *testing.T) {
	q := testQueue(false)
	q.items[0].CommentCount = 1
	m, cmd := dupdate(q, "enter")
	if cmd == nil || m.commentsS != "loading" {
		t.Fatalf("cmd=%v state=%q", cmd != nil, m.commentsS)
	}
	u, _ := m.Update(commentsMsg{id: "task-1", comments: []comment{{Author: "FM", Text: "earlier note"}}})
	m = u.(decideModel)
	if m.commentsS != "" || len(m.comments) != 1 {
		t.Errorf("state=%q comments=%v", m.commentsS, m.comments)
	}
	// A late reply for another item is dropped.
	u, _ = m.Update(commentsMsg{id: "task-2", comments: []comment{{Text: "wrong item"}}})
	if got := u.(decideModel).comments; len(got) != 1 {
		t.Errorf("comments = %v", got)
	}
}

func TestDecideViews(t *testing.T) {
	m := testQueue(true)
	list := m.View()
	for _, want := range []string{"Decisions waiting on you  2", "[dry run]", "task-1", "First", "⏎ open"} {
		if !strings.Contains(list, want) {
			t.Errorf("list view missing %q:\n%s", want, list)
		}
	}
	m, _ = dupdate(m, "enter")
	item := m.View()
	for _, want := range []string{"Decision 1/2", "Pick", "[1] Yes, apply them tonight", "★", "Answer",
		"After saving", "mark answered", "[ Save answer ]", "-- TAB --", "ctrl+s save"} {
		if !strings.Contains(item, want) {
			t.Errorf("item view missing %q:\n%s", want, item)
		}
	}
	m.outcome = outcomeClose
	if !strings.Contains(m.View(), "close the item") {
		t.Error("close outcome should be shown")
	}
}

func TestDecideNextOpenSkipsAnswered(t *testing.T) {
	m := testQueue(false)
	m.items[1].Labels = []string{answeredLabel}
	if got := m.nextOpen(0); got != 0 {
		t.Errorf("nextOpen = %d, want wrap to 0", got)
	}
	m.handled[0] = true
	if got := m.nextOpen(0); got != -1 {
		t.Errorf("nextOpen = %d, want -1", got)
	}
}
